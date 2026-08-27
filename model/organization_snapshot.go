package model

import (
	"errors"

	"gorm.io/gorm"
)

var ErrOrganizationSnapshotInvalid = errors.New("organization resource snapshot has no valid organization")

// organizationIDForUser resolves tenant ownership from the primary user row.
// Resource payloads must never be trusted to provide this value. During a
// pre-migration/maintenance window an installation may not yet have an
// organizations table; that legacy state is tolerated until provisioning has
// completed. Once the tenant table exists, an existing user without a tenant
// is an identity error and must not create a cross-tenant resource.
func organizationIDForUser(tx *gorm.DB, userID int) (int, error) {
	if tx == nil || userID <= 0 {
		return 0, nil
	}
	// A few maintenance/legacy migrations create resource tables before the
	// users table. Preserve their ability to bootstrap; the normal application
	// migration always creates users before accepting tenant-bound writes.
	if !tx.Migrator().HasTable(&User{}) {
		return 0, nil
	}
	var snapshot struct {
		OrganizationId int
	}
	err := tx.Model(&User{}).
		Select("organization_id").
		Where("id = ?", userID).
		First(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		provisioned, provisionErr := organizationProvisioned(tx)
		if provisionErr != nil {
			return 0, provisionErr
		}
		if provisioned {
			return 0, ErrOrganizationSnapshotInvalid
		}
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if snapshot.OrganizationId <= 0 {
		provisioned, provisionErr := organizationProvisioned(tx)
		if provisionErr != nil {
			return 0, provisionErr
		}
		if provisioned {
			return 0, ErrOrganizationSnapshotInvalid
		}
	}
	return snapshot.OrganizationId, nil
}

func organizationTableReady(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	return tx.Migrator().HasTable(&Organization{})
}

func organizationProvisioned(tx *gorm.DB) (bool, error) {
	if !organizationTableReady(tx) {
		return false, nil
	}
	var count int64
	if err := tx.Model(&Organization{}).
		Where("system_key = ?", DefaultOrganizationSystemKey).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func overwriteOrganizationSnapshot(tx *gorm.DB, userID int, organizationID *int) error {
	if organizationID == nil {
		return nil
	}
	resolved, err := organizationIDForUser(tx, userID)
	if err != nil {
		return err
	}
	*organizationID = resolved
	return nil
}

// resolveOrganizationSnapshotForLog uses the primary DB even when logs are
// stored in a separate SQL or ClickHouse database.
func resolveOrganizationSnapshotForLog(userID int) (int, error) {
	return organizationIDForUser(DB, userID)
}
