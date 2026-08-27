package model

import (
	"errors"
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrOrganizationMigrationNoRoot       = errors.New("organization migration requires exactly one root user")
	ErrOrganizationMigrationMultipleRoot = errors.New("organization migration found multiple root users")
	ErrDefaultOrganizationConflict       = errors.New("default organization ownership conflicts with the platform root")
	ErrOrganizationMembershipMissing     = errors.New("organization migration found users without an organization")
	ErrOrganizationMembershipInvalid     = errors.New("organization migration found invalid organization membership")
	ErrOrganizationOwnerInvalid          = errors.New("organization migration found an invalid organization owner")
	ErrOrganizationSnapshotMissing       = errors.New("organization migration found resources without an organization snapshot")
	ErrOrganizationLogBackfillIncomplete = errors.New("organization log migration found logs without a valid user organization")
)

const taskBillingSourceSubscription = "subscription"

func GetDefaultOrganization() (*Organization, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return getDefaultOrganizationWithTx(DB)
}

func getDefaultOrganizationWithTx(tx *gorm.DB) (*Organization, error) {
	var organization Organization
	if err := tx.Where("system_key = ?", DefaultOrganizationSystemKey).First(&organization).Error; err != nil {
		return nil, err
	}
	return &organization, nil
}

// EnsureDefaultOrganizationForRootTx provisions the default organization in
// the caller's root-creation transaction. Owner assignment is intentionally a
// platform-side operation; organization roles never invoke this function.
func EnsureDefaultOrganizationForRootTx(tx *gorm.DB, rootUserId int) (*Organization, error) {
	if tx == nil || rootUserId <= 0 {
		return nil, ErrOrganizationMigrationNoRoot
	}

	var root User
	if err := lockForUpdate(tx).Unscoped().Where("id = ?", rootUserId).First(&root).Error; err != nil {
		return nil, err
	}
	if root.Role != common.RoleRootUser || root.DeletedAt.Valid {
		return nil, ErrOrganizationMigrationNoRoot
	}

	organization, err := getDefaultOrganizationWithTx(lockForUpdate(tx))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if root.OrganizationId != 0 {
			return nil, ErrDefaultOrganizationConflict
		}
		systemKey := DefaultOrganizationSystemKey
		organization = &Organization{
			Name:             DefaultOrganizationName,
			SystemKey:        &systemKey,
			Status:           OrganizationStatusActive,
			OwnerUserId:      root.Id,
			AllowMemberTopup: true,
			PolicyVersion:    1,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(organization).Error; err != nil {
			return nil, err
		}
		// Another master may have provisioned the row concurrently. Re-read it
		// under the transaction lock and validate ownership before proceeding.
		organization, err = getDefaultOrganizationWithTx(lockForUpdate(tx))
		if err != nil {
			return nil, err
		}
		if organization.OwnerUserId != root.Id {
			return nil, ErrDefaultOrganizationConflict
		}
	} else if err != nil {
		return nil, err
	} else if organization.OwnerUserId != root.Id {
		return nil, ErrDefaultOrganizationConflict
	}

	if root.OrganizationId != 0 && root.OrganizationId != organization.Id {
		return nil, ErrDefaultOrganizationConflict
	}
	if organization.LegacyMainBackfillAt != 0 &&
		(root.OrganizationId != organization.Id ||
			root.OrganizationRole != OrganizationRoleOwner ||
			root.OrganizationStatus != OrganizationMemberStatusActive) {
		return nil, ErrDefaultOrganizationConflict
	}
	if organization.LegacyMainBackfillAt == 0 {
		if err := tx.Unscoped().Model(&User{}).Where("id = ?", root.Id).Updates(map[string]interface{}{
			"organization_id":     organization.Id,
			"organization_role":   OrganizationRoleOwner,
			"organization_status": OrganizationMemberStatusActive,
		}).Error; err != nil {
			return nil, err
		}
	}

	var account OrganizationFundAccount
	err = lockForUpdate(tx).Where("organization_id = ?", organization.Id).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		account = OrganizationFundAccount{OrganizationId: organization.Id}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
			return nil, err
		}
		if err := lockForUpdate(tx).Where("organization_id = ?", organization.Id).First(&account).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return organization, nil
}

// EnsureDefaultOrganizationAndBackfill migrates a legacy single-tenant
// installation exactly once. Empty databases remain untouched until setup
// creates the root user; ambiguous root ownership fails closed.
func EnsureDefaultOrganizationAndBackfill() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var userCount int64
		if err := tx.Unscoped().Model(&User{}).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount == 0 {
			return nil
		}

		var roots []User
		if err := lockForUpdate(tx).Unscoped().Where("role = ?", common.RoleRootUser).Find(&roots).Error; err != nil {
			return err
		}
		if len(roots) > 1 {
			return ErrOrganizationMigrationMultipleRoot
		}
		if len(roots) == 0 || roots[0].DeletedAt.Valid {
			return ErrOrganizationMigrationNoRoot
		}

		organization, err := EnsureDefaultOrganizationForRootTx(tx, roots[0].Id)
		if err != nil {
			return err
		}
		if organization.LegacyMainBackfillAt != 0 {
			// The first organization migration shipped with a zero-only predicate.
			// Existing columns added by AutoMigrate are nullable on supported SQL
			// databases, so repair those missed legacy rows even after the marker
			// was written. A literal zero after the marker still fails closed: it
			// can represent a new invalid write whose intended tenant is unknown.
			var nullableUsers int64
			if err := tx.Unscoped().Model(&User{}).Where("organization_id IS NULL").Count(&nullableUsers).Error; err != nil {
				return err
			}
			if nullableUsers > 0 {
				if err := tx.Unscoped().Model(&User{}).Where("organization_id IS NULL").Updates(map[string]interface{}{
					"organization_id":     organization.Id,
					"organization_role":   OrganizationRoleMember,
					"organization_status": OrganizationMemberStatusActive,
				}).Error; err != nil {
					return err
				}
			}
			if err := validateOrganizationMembershipIntegrityTx(tx, organization.Id, roots[0].Id); err != nil {
				return err
			}
			if err := backfillLegacyOrganizationSnapshotsTx(tx, organization.Id, false); err != nil {
				return err
			}
			if err := validateLegacyOrganizationSnapshotsTx(tx); err != nil {
				return err
			}
			return nil
		}

		if err := tx.Unscoped().Model(&User{}).
			Where("organization_id IS NULL OR organization_id = ?", 0).
			Updates(map[string]interface{}{
				"organization_id":     organization.Id,
				"organization_role":   OrganizationRoleMember,
				"organization_status": OrganizationMemberStatusActive,
			}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Model(&User{}).Where("id = ?", roots[0].Id).Updates(map[string]interface{}{
			"organization_role":   OrganizationRoleOwner,
			"organization_status": OrganizationMemberStatusActive,
		}).Error; err != nil {
			return err
		}
		if err := validateOrganizationMembershipIntegrityTx(tx, organization.Id, roots[0].Id); err != nil {
			return err
		}
		if err := backfillLegacyOrganizationSnapshotsTx(tx, organization.Id, true); err != nil {
			return err
		}
		if err := validateLegacyOrganizationSnapshotsTx(tx); err != nil {
			return err
		}

		now := common.GetTimestamp()
		updates := map[string]interface{}{"legacy_main_backfill_at": now}
		if os.Getenv("LOG_SQL_DSN") == "" {
			updates["legacy_log_backfill_at"] = now
		}
		if err := tx.Model(&Organization{}).Where("id = ?", organization.Id).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
}

func validateOrganizationMembershipIntegrityTx(tx *gorm.DB, defaultOrganizationId int, rootUserId int) error {
	if tx == nil || defaultOrganizationId <= 0 || rootUserId <= 0 {
		return ErrOrganizationMembershipInvalid
	}

	var organizations []Organization
	if err := tx.Order("id").Find(&organizations).Error; err != nil {
		return err
	}
	organizationsById := make(map[int]Organization, len(organizations))
	for _, organization := range organizations {
		organizationsById[organization.Id] = organization
	}
	defaultOrganization, ok := organizationsById[defaultOrganizationId]
	if !ok || defaultOrganization.SystemKey == nil || *defaultOrganization.SystemKey != DefaultOrganizationSystemKey ||
		defaultOrganization.OwnerUserId != rootUserId {
		return ErrDefaultOrganizationConflict
	}

	ownerUsers := make(map[int]int, len(organizations))
	rootCount := 0
	users := make([]User, 0, 500)
	err := tx.Unscoped().Model(&User{}).
		Select("id", "role", "organization_id", "organization_role", "organization_status", "deleted_at").
		Order("id").
		FindInBatches(&users, 500, func(_ *gorm.DB, _ int) error {
			for _, user := range users {
				if user.OrganizationId <= 0 {
					return fmt.Errorf("%w: user %d", ErrOrganizationMembershipMissing, user.Id)
				}
				organization, exists := organizationsById[user.OrganizationId]
				if !exists {
					return fmt.Errorf("%w: user %d references organization %d", ErrOrganizationMembershipInvalid, user.Id, user.OrganizationId)
				}
				switch user.OrganizationRole {
				case OrganizationRoleOwner, OrganizationRoleAdmin, OrganizationRoleMember:
				default:
					return fmt.Errorf("%w: user %d has role %q", ErrOrganizationMembershipInvalid, user.Id, user.OrganizationRole)
				}
				switch user.OrganizationStatus {
				case OrganizationMemberStatusActive, OrganizationMemberStatusDisabled:
				default:
					return fmt.Errorf("%w: user %d has status %q", ErrOrganizationMembershipInvalid, user.Id, user.OrganizationStatus)
				}

				if user.OrganizationRole == OrganizationRoleOwner {
					if user.DeletedAt.Valid || user.OrganizationStatus != OrganizationMemberStatusActive || organization.OwnerUserId != user.Id {
						return fmt.Errorf("%w: organization %d owner user %d", ErrOrganizationOwnerInvalid, organization.Id, user.Id)
					}
					if existingOwner := ownerUsers[organization.Id]; existingOwner != 0 && existingOwner != user.Id {
						return fmt.Errorf("%w: organization %d has multiple owner memberships", ErrOrganizationOwnerInvalid, organization.Id)
					}
					ownerUsers[organization.Id] = user.Id
				}

				if user.Role == common.RoleRootUser {
					rootCount++
					if user.Id != rootUserId || user.DeletedAt.Valid || user.OrganizationId != defaultOrganizationId ||
						user.OrganizationRole != OrganizationRoleOwner || user.OrganizationStatus != OrganizationMemberStatusActive {
						return ErrDefaultOrganizationConflict
					}
				}
			}
			return nil
		}).Error
	if err != nil {
		return err
	}
	if rootCount != 1 {
		return ErrDefaultOrganizationConflict
	}
	for _, organization := range organizations {
		if organization.OwnerUserId <= 0 || ownerUsers[organization.Id] != organization.OwnerUserId {
			return fmt.Errorf("%w: organization %d owner user %d", ErrOrganizationOwnerInvalid, organization.Id, organization.OwnerUserId)
		}
	}
	return nil
}

func backfillLegacyOrganizationSnapshotsTx(tx *gorm.DB, organizationId int, includeZero bool) error {
	missingOrganization := "organization_id IS NULL"
	if includeZero {
		missingOrganization = "organization_id IS NULL OR organization_id = 0"
	}
	targetOrganizationIds := []int{organizationId}
	scopeByOwner := !includeZero
	if scopeByOwner {
		targetOrganizationIds = nil
		if err := tx.Unscoped().Model(&User{}).
			Where("organization_id IS NOT NULL AND organization_id <> ?", 0).
			Distinct("organization_id").
			Order("organization_id").
			Pluck("organization_id", &targetOrganizationIds).Error; err != nil {
			return err
		}
	}
	resources := []struct {
		model               interface{}
		legacyWalletBilling bool
	}{
		{model: &Token{}},
		{model: &TopUp{}},
		{model: &Midjourney{}, legacyWalletBilling: true},
		{model: &QuotaData{}},
	}
	for _, resource := range resources {
		if !tx.Migrator().HasTable(resource.model) {
			continue
		}
		for _, targetOrganizationId := range targetOrganizationIds {
			query := tx.Unscoped().Model(resource.model).Where(missingOrganization)
			if scopeByOwner {
				ownerIds := tx.Unscoped().Model(&User{}).
					Select("id").
					Where("organization_id = ?", targetOrganizationId)
				query = query.Where("user_id IN (?)", ownerIds)
			}
			updates := map[string]interface{}{"organization_id": targetOrganizationId}
			if resource.legacyWalletBilling {
				updates["legacy_organization_wallet"] = true
			}
			if err := query.Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	if tx.Migrator().HasTable(&Task{}) {
		for _, targetOrganizationId := range targetOrganizationIds {
			legacyTasks := make([]Task, 0, 500)
			query := tx.Unscoped().Model(&Task{}).
				Select("id", "private_data").
				Where(missingOrganization)
			if scopeByOwner {
				ownerIds := tx.Unscoped().Model(&User{}).
					Select("id").
					Where("organization_id = ?", targetOrganizationId)
				query = query.Where("user_id IN (?)", ownerIds)
			}
			err := query.FindInBatches(&legacyTasks, 500, func(_ *gorm.DB, _ int) error {
				walletTaskIds := make([]int64, 0, len(legacyTasks))
				for _, task := range legacyTasks {
					if task.PrivateData.BillingSource != taskBillingSourceSubscription {
						walletTaskIds = append(walletTaskIds, task.ID)
					}
				}
				if len(walletTaskIds) == 0 {
					return nil
				}
				return tx.Unscoped().Model(&Task{}).Where("id IN ?", walletTaskIds).
					Update("legacy_organization_wallet", true).Error
			}).Error
			if err != nil {
				return err
			}
			update := tx.Unscoped().Model(&Task{}).Where(missingOrganization)
			if scopeByOwner {
				ownerIds := tx.Unscoped().Model(&User{}).
					Select("id").
					Where("organization_id = ?", targetOrganizationId)
				update = update.Where("user_id IN (?)", ownerIds)
			}
			if err := update.Update("organization_id", targetOrganizationId).Error; err != nil {
				return err
			}
		}
	}
	if tx.Migrator().HasTable(&Log{}) {
		for _, targetOrganizationId := range targetOrganizationIds {
			query := tx.Unscoped().Model(&Log{}).
				Where("(" + missingOrganization + ") AND user_id > 0")
			if scopeByOwner {
				ownerIds := tx.Unscoped().Model(&User{}).
					Select("id").
					Where("organization_id = ?", targetOrganizationId)
				query = query.Where("user_id IN (?)", ownerIds)
			}
			if err := query.Update("organization_id", targetOrganizationId).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func validateLegacyOrganizationSnapshotsTx(tx *gorm.DB) error {
	resources := []struct {
		name  string
		model interface{}
	}{
		{name: "tokens", model: &Token{}},
		{name: "top_ups", model: &TopUp{}},
		{name: "midjourneys", model: &Midjourney{}},
		{name: "quota_data", model: &QuotaData{}},
		{name: "tasks", model: &Task{}},
	}
	for _, resource := range resources {
		if !tx.Migrator().HasTable(resource.model) {
			continue
		}
		var missing int64
		if err := tx.Unscoped().Model(resource.model).
			Where("organization_id IS NULL OR organization_id = ?", 0).
			Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("%w: %s has %d rows", ErrOrganizationSnapshotMissing, resource.name, missing)
		}
	}
	if tx.Migrator().HasTable(&Log{}) {
		var missing int64
		if err := tx.Unscoped().Model(&Log{}).
			Where("(organization_id IS NULL OR organization_id = ?) AND user_id > 0", 0).
			Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("%w: logs has %d rows", ErrOrganizationSnapshotMissing, missing)
		}
	}
	return nil
}

func backfillLegacyLogOrganization() error {
	organization, err := GetDefaultOrganization()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var userCount int64
		if countErr := DB.Unscoped().Model(&User{}).Count(&userCount).Error; countErr != nil {
			return countErr
		}
		if userCount == 0 {
			return nil
		}
		return ErrOrganizationMigrationNoRoot
	}
	if err != nil {
		return err
	}
	markerWritten := organization.LegacyLogBackfillAt != 0
	if LOG_DB == nil {
		return errors.New("log database is not initialized")
	}
	if err := validateOrganizationMembershipIntegrityTx(DB, organization.Id, organization.OwnerUserId); err != nil {
		return err
	}

	users := make([]User, 0, 500)
	err = DB.Unscoped().Model(&User{}).
		Select("id", "organization_id").
		Order("id").
		FindInBatches(&users, 500, func(_ *gorm.DB, _ int) error {
			userIdsByOrganization := make(map[int][]int)
			for _, user := range users {
				userIdsByOrganization[user.OrganizationId] = append(userIdsByOrganization[user.OrganizationId], user.Id)
			}
			for organizationId, userIds := range userIdsByOrganization {
				if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
					statement := "ALTER TABLE logs UPDATE organization_id = ? WHERE organization_id = 0 AND user_id IN ? SETTINGS mutations_sync = 1"
					if err := LOG_DB.Exec(statement, organizationId, userIds).Error; err != nil {
						return err
					}
					continue
				}
				if err := LOG_DB.Model(&Log{}).
					Where("(organization_id IS NULL OR organization_id = ?) AND user_id IN ?", 0, userIds).
					Update("organization_id", organizationId).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error
	if err != nil {
		return err
	}

	var missing int64
	missingQuery := LOG_DB.Model(&Log{}).Where("organization_id = ? AND user_id > 0", 0)
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		missingQuery = LOG_DB.Model(&Log{}).Where("(organization_id IS NULL OR organization_id = ?) AND user_id > 0", 0)
	}
	if err := missingQuery.Count(&missing).Error; err != nil {
		return err
	}
	if missing != 0 {
		return fmt.Errorf("%w: %d log rows", ErrOrganizationLogBackfillIncomplete, missing)
	}
	if markerWritten {
		return nil
	}

	return DB.Model(&Organization{}).Where("id = ?", organization.Id).
		Update("legacy_log_backfill_at", common.GetTimestamp()).Error
}

func clickHouseLogOrganizationBackfillSQL() string {
	return "ALTER TABLE logs UPDATE organization_id = ? WHERE organization_id = 0 AND user_id > 0"
}
