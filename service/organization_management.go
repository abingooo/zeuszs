package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The platform organization management API is deliberately separate from the
// tenant permission matrix. A platform administrator may provision tenants,
// but an organization Owner/Admin never gains this capability merely from
// their organization role.
var (
	ErrOrganizationManagementInvalid      = errors.New("invalid organization management request")
	ErrOrganizationNameRequired           = errors.New("organization name is required")
	ErrOrganizationOwnerRequired          = errors.New("organization owner is required")
	ErrOrganizationOwnerInvalid           = errors.New("organization owner is invalid")
	ErrOrganizationUserAlreadyAssigned    = errors.New("organization user already belongs to another organization")
	ErrOrganizationOwnerConflict          = errors.New("organization already has a different owner")
	ErrOrganizationOwnerDemotionForbidden = errors.New("organization owner cannot be demoted without a replacement")
	ErrOrganizationStatusInvalid          = errors.New("invalid organization status")
)

// CreateOrganizationParams contains only platform-controlled provisioning
// fields. SystemKey is intentionally not accepted: the reserved default key
// belongs to the platform-created default organization.
type CreateOrganizationParams struct {
	Name             string
	OwnerUserID      int
	AllowMemberTopup *bool
	RequestID        string
}

type ListOrganizationsParams struct {
	Offset int
	Limit  int
	Status *model.OrganizationStatus
}

type OrganizationListItem struct {
	model.Organization
	MemberCount   int64  `json:"member_count"`
	OwnerUsername string `json:"owner_username,omitempty"`
}

type OrganizationListResult struct {
	Items []OrganizationListItem `json:"items"`
	Total int64                  `json:"total"`
}

type UpdateOrganizationStatusParams struct {
	OrganizationID int
	Status         model.OrganizationStatus
	RequestID      string
}

type AssignOrganizationRoleParams struct {
	OrganizationID int
	UserID         int
	Role           model.OrganizationRole
	RequestID      string
}

type TransferOrganizationOwnershipParams struct {
	OrganizationID int
	NewOwnerUserID int
	RequestID      string
}

func normalizeOrganizationManagementRequestID(requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	if len(requestID) > 64 {
		return "", ErrOrganizationManagementInvalid
	}
	return requestID, nil
}

func normalizeOrganizationName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrOrganizationNameRequired
	}
	if len(name) > 128 {
		return "", ErrOrganizationManagementInvalid
	}
	return name, nil
}

func normalizeOrganizationStatus(status model.OrganizationStatus) (model.OrganizationStatus, error) {
	status = model.OrganizationStatus(strings.ToLower(strings.TrimSpace(string(status))))
	switch status {
	case model.OrganizationStatusActive,
		model.OrganizationStatusDisabled,
		model.OrganizationStatusDissolving,
		model.OrganizationStatusDissolved:
		return status, nil
	default:
		return "", ErrOrganizationStatusInvalid
	}
}

func normalizeOrganizationRole(role model.OrganizationRole) (model.OrganizationRole, error) {
	role = model.OrganizationRole(strings.ToLower(strings.TrimSpace(string(role))))
	switch role {
	case model.OrganizationRoleOwner, model.OrganizationRoleAdmin, model.OrganizationRoleMember:
		return role, nil
	default:
		return "", ErrOrganizationManagementInvalid
	}
}

func loadPlatformOrganizationActorTx(tx *gorm.DB, actorUserID int, lock bool) (*model.User, error) {
	if tx == nil || actorUserID <= 0 {
		return nil, ErrPlatformProvisioningForbidden
	}
	query := tx
	if lock {
		query = model.LockForUpdate(tx)
	}
	var actor model.User
	if err := query.Where("id = ?", actorUserID).First(&actor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformProvisioningForbidden
		}
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled || (actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser) {
		return nil, ErrPlatformProvisioningForbidden
	}
	return &actor, nil
}

// requirePlatformOrganizationActorTx re-reads the actor while holding a row
// lock. The request's organization principal and role are intentionally not
// consulted: only the platform role authorizes these provisioning operations.
func requirePlatformOrganizationActorTx(tx *gorm.DB, actorUserID int) (*model.User, error) {
	return loadPlatformOrganizationActorTx(tx, actorUserID, true)
}

// lockPlatformOrganizationScopeTx preserves the global organization-before-user
// lock order. The unlocked actor read prevents unauthorized callers from using
// organization existence as an oracle; the locked re-read handles revocation
// that races with the management request.
func lockPlatformOrganizationScopeTx(tx *gorm.DB, actorUserID, organizationID int) (*model.Organization, *model.User, error) {
	if organizationID <= 0 {
		return nil, nil, ErrOrganizationManagementInvalid
	}
	if _, err := loadPlatformOrganizationActorTx(tx, actorUserID, false); err != nil {
		return nil, nil, err
	}
	var organization model.Organization
	if err := model.LockForUpdate(tx).Where("id = ?", organizationID).First(&organization).Error; err != nil {
		return nil, nil, err
	}
	actor, err := requirePlatformOrganizationActorTx(tx, actorUserID)
	if err != nil {
		return nil, nil, err
	}
	return &organization, actor, nil
}

func organizationAuditTx(tx *gorm.DB, organizationID, actorUserID int, action, targetType, targetID, requestID string, metadata map[string]interface{}) error {
	if tx == nil || organizationID <= 0 || actorUserID <= 0 || strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" || strings.TrimSpace(targetID) == "" || strings.TrimSpace(requestID) == "" {
		return ErrOrganizationManagementInvalid
	}
	encoded, err := common.Marshal(metadata)
	if err != nil {
		return err
	}
	return tx.Create(&model.OrganizationAuditEvent{
		OrganizationId: organizationID,
		ActorUserId:    actorUserID,
		Action:         action,
		TargetType:     targetType,
		TargetId:       targetID,
		RequestId:      requestID,
		Metadata:       string(encoded),
	}).Error
}

// CreateOrganizationForPlatform creates an active organization, its fund
// account, and its owner in one transaction. A user with an existing tenant
// is rejected rather than silently moved, preserving the one-organization
// invariant.
func CreateOrganizationForPlatform(actorUserID int, params CreateOrganizationParams) (*model.Organization, error) {
	name, err := normalizeOrganizationName(params.Name)
	if err != nil {
		return nil, err
	}
	if params.OwnerUserID <= 0 {
		return nil, ErrOrganizationOwnerRequired
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	allowMemberTopup := true
	if params.AllowMemberTopup != nil {
		allowMemberTopup = *params.AllowMemberTopup
	}
	if model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}

	var organization model.Organization
	var changes []model.OrganizationAuthorizationChange
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := requirePlatformOrganizationActorTx(tx, actorUserID); err != nil {
			return err
		}

		var owner model.User
		if err := model.LockForUpdate(tx).Where("id = ?", params.OwnerUserID).First(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrganizationOwnerInvalid
			}
			return err
		}
		if owner.Status != common.UserStatusEnabled || owner.OrganizationId != 0 {
			return ErrOrganizationOwnerInvalid
		}

		organization = model.Organization{
			Name:             name,
			Status:           model.OrganizationStatusActive,
			OwnerUserId:      owner.Id,
			AllowMemberTopup: allowMemberTopup,
			PolicyVersion:    1,
		}
		if err := tx.Create(&organization).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error; err != nil {
			return err
		}

		result := tx.Model(&model.User{}).Where("id = ? AND organization_id = 0", owner.Id).Updates(map[string]interface{}{
			"organization_id":     organization.Id,
			"organization_role":   model.OrganizationRoleOwner,
			"organization_status": model.OrganizationMemberStatusActive,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationOwnerInvalid
		}
		change, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, owner.Id)
		if err != nil {
			return err
		}
		changes = append(changes, change)
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
			DoNothing: true,
		}).Create(&model.OrganizationMemberFund{
			OrganizationId: organization.Id,
			UserId:         owner.Id,
		}).Error; err != nil {
			return err
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.create", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
			"name":               organization.Name,
			"owner_user_id":      owner.Id,
			"allow_member_topup": organization.AllowMemberTopup,
		})
	})
	if err != nil {
		return nil, err
	}
	if err := model.FinalizeOrganizationAuthorizationChanges(changes, "organization_created"); err != nil {
		return &organization, err
	}
	return &organization, nil
}

// ListOrganizationsForPlatform returns a tenant-scoped administrative view.
// Counts and owner names are loaded with portable GORM queries rather than
// dialect-specific aggregate SQL.
func ListOrganizationsForPlatform(actorUserID int, params ListOrganizationsParams) (*OrganizationListResult, error) {
	if actorUserID <= 0 || model.DB == nil {
		return nil, ErrPlatformProvisioningForbidden
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Status != nil {
		status, err := normalizeOrganizationStatus(*params.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &status
	}

	// Re-read the actor before querying, including for read-only management
	// endpoints. This prevents an organization administrator from using a
	// stale platform-role value in a forged context.
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := requirePlatformOrganizationActorTx(tx, actorUserID)
		return err
	}); err != nil {
		return nil, err
	}

	query := model.DB.Model(&model.Organization{})
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var organizations []model.Organization
	if err := query.Order("id asc").Offset(params.Offset).Limit(params.Limit).Find(&organizations).Error; err != nil {
		return nil, err
	}
	items := make([]OrganizationListItem, 0, len(organizations))
	for _, organization := range organizations {
		item := OrganizationListItem{Organization: organization}
		if err := model.DB.Model(&model.User{}).Where("organization_id = ?", organization.Id).Count(&item.MemberCount).Error; err != nil {
			return nil, err
		}
		if organization.OwnerUserId > 0 {
			if err := model.DB.Model(&model.User{}).Where("id = ?", organization.OwnerUserId).Select("username").Scan(&item.OwnerUsername).Error; err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	return &OrganizationListResult{Items: items, Total: total}, nil
}

// UpdateOrganizationStatusForPlatform updates status and invalidates every
// member's authorization in the same transaction as the audit event.
func UpdateOrganizationStatusForPlatform(actorUserID int, params UpdateOrganizationStatusParams) (*model.Organization, error) {
	if params.OrganizationID <= 0 {
		return nil, ErrOrganizationManagementInvalid
	}
	status, err := normalizeOrganizationStatus(params.Status)
	if err != nil {
		return nil, err
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}

	var organization model.Organization
	var changes []model.OrganizationAuthorizationChange
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization = *current
		oldStatus := organization.Status
		if oldStatus == status {
			return nil
		}
		var userIDs []int
		if err := tx.Model(&model.User{}).Where("organization_id = ?", organization.Id).Order("id asc").Pluck("id", &userIDs).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Organization{}).Where("id = ?", organization.Id).Update("status", status).Error; err != nil {
			return err
		}
		organization.Status = status
		for _, userID := range userIDs {
			change, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, userID)
			if err != nil {
				return err
			}
			changes = append(changes, change)
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.status.update", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
			"from": string(oldStatus),
			"to":   string(status),
		})
	})
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		if err := model.FinalizeOrganizationAuthorizationChanges(changes, "organization_status_changed"); err != nil {
			return &organization, err
		}
	}
	return &organization, nil
}

// AssignOrganizationMemberRoleForPlatform assigns a user to an organization
// or changes an existing member's organization role. Cross-organization moves
// and owner replacement/demotion are rejected explicitly.
func AssignOrganizationMemberRoleForPlatform(actorUserID int, params AssignOrganizationRoleParams) (*model.User, error) {
	if params.OrganizationID <= 0 || params.UserID <= 0 {
		return nil, ErrOrganizationManagementInvalid
	}
	role, err := normalizeOrganizationRole(params.Role)
	if err != nil {
		return nil, err
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}

	var target model.User
	var change *model.OrganizationAuthorizationChange
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization := *current
		if organization.Status == model.OrganizationStatusDissolved {
			return model.ErrOrganizationNotActive
		}
		if organization.OwnerUserId == 0 && role != model.OrganizationRoleOwner {
			return ErrOrganizationOwnerRequired
		}
		if err := model.LockForUpdate(tx).Omit("password", "access_token").Where("id = ?", params.UserID).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrganizationOwnerInvalid
			}
			return err
		}
		if target.Status != common.UserStatusEnabled {
			return ErrOrganizationOwnerInvalid
		}
		if target.OrganizationId != 0 && target.OrganizationId != organization.Id {
			return ErrOrganizationUserAlreadyAssigned
		}
		if target.OrganizationId == organization.Id && target.OrganizationRole == model.OrganizationRoleOwner && role != model.OrganizationRoleOwner {
			return ErrOrganizationOwnerDemotionForbidden
		}

		if role == model.OrganizationRoleOwner {
			if organization.OwnerUserId != 0 && organization.OwnerUserId != target.Id {
				return ErrOrganizationOwnerConflict
			}
			var owners []model.User
			if err := model.LockForUpdate(tx).Where("organization_id = ? AND organization_role = ?", organization.Id, model.OrganizationRoleOwner).Order("id asc").Find(&owners).Error; err != nil {
				return err
			}
			for _, owner := range owners {
				if owner.Id != target.Id {
					return ErrOrganizationOwnerConflict
				}
			}
			if organization.OwnerUserId == 0 {
				if err := tx.Model(&model.Organization{}).Where("id = ? AND owner_user_id = 0", organization.Id).Update("owner_user_id", target.Id).Error; err != nil {
					return err
				}
				organization.OwnerUserId = target.Id
			}
		}
		oldOrganizationID := target.OrganizationId
		oldRole := target.OrganizationRole
		oldStatus := target.OrganizationStatus
		newStatus := oldStatus
		if oldOrganizationID == 0 {
			newStatus = model.OrganizationMemberStatusActive
		} else if oldStatus != model.OrganizationMemberStatusActive && oldStatus != model.OrganizationMemberStatusDisabled {
			// Legacy rows may have an empty snapshot. Repair it while assigning
			// a role, but preserve an explicit disabled membership.
			newStatus = model.OrganizationMemberStatusActive
		}
		if oldOrganizationID != organization.Id || oldRole != role || oldStatus != newStatus {
			if err := tx.Model(&model.User{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
				"organization_id":     organization.Id,
				"organization_role":   role,
				"organization_status": newStatus,
			}).Error; err != nil {
				return err
			}
			version, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, target.Id)
			if err != nil {
				return err
			}
			change = &version
			target.AuthVersion = version.AuthVersion
		}
		target.OrganizationId = organization.Id
		target.OrganizationRole = role
		target.OrganizationStatus = newStatus
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}},
			DoNothing: true,
		}).Create(&model.OrganizationMemberFund{OrganizationId: organization.Id, UserId: target.Id}).Error; err != nil {
			return err
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.member.role.update", "user", strconv.Itoa(target.Id), requestID, map[string]interface{}{
			"from_organization_id": oldOrganizationID,
			"to_organization_id":   organization.Id,
			"from_role":            string(oldRole),
			"to_role":              string(role),
			"from_status":          string(oldStatus),
			"to_status":            string(newStatus),
		})
	})
	if err != nil {
		return nil, err
	}
	if change != nil {
		if err := model.FinalizeOrganizationAuthorizationChanges([]model.OrganizationAuthorizationChange{*change}, "organization_role_changed"); err != nil {
			return &target, err
		}
	}
	return &target, nil
}

// TransferOrganizationOwnershipForPlatform atomically appoints a new Owner
// and demotes the previous Owner to organization Admin. This is deliberately a
// separate platform-only operation: generic role assignment cannot partially
// replace ownership, and organization Owner/Admin roles never authorize it.
func TransferOrganizationOwnershipForPlatform(actorUserID int, params TransferOrganizationOwnershipParams) (*model.Organization, error) {
	if params.OrganizationID <= 0 || params.NewOwnerUserID <= 0 {
		return nil, ErrOrganizationManagementInvalid
	}
	requestID, err := normalizeOrganizationManagementRequestID(params.RequestID)
	if err != nil {
		return nil, err
	}
	if model.DB == nil {
		return nil, ErrOrganizationManagementInvalid
	}

	var organization model.Organization
	changes := make([]model.OrganizationAuthorizationChange, 0, 2)
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		current, _, err := lockPlatformOrganizationScopeTx(tx, actorUserID, params.OrganizationID)
		if err != nil {
			return err
		}
		organization = *current
		if organization.Status == model.OrganizationStatusDissolved {
			return model.ErrOrganizationNotActive
		}
		if organization.OwnerUserId <= 0 {
			return ErrOrganizationOwnerRequired
		}
		if organization.SystemKey != nil && *organization.SystemKey == model.DefaultOrganizationSystemKey &&
			organization.OwnerUserId != params.NewOwnerUserID {
			return model.ErrDefaultOrganizationConflict
		}

		userIDs := []int{organization.OwnerUserId, params.NewOwnerUserID}
		sort.Ints(userIDs)
		if userIDs[0] == userIDs[1] {
			var currentOwner model.User
			if err := model.LockForUpdate(tx).Where("id = ?", organization.OwnerUserId).First(&currentOwner).Error; err != nil {
				return err
			}
			if currentOwner.OrganizationId != organization.Id || currentOwner.OrganizationRole != model.OrganizationRoleOwner {
				return ErrOrganizationOwnerInvalid
			}
			return nil
		}

		var users []model.User
		if err := model.LockForUpdate(tx).Where("id IN ?", userIDs).Order("id asc").Find(&users).Error; err != nil {
			return err
		}
		if len(users) != 2 {
			return ErrOrganizationOwnerInvalid
		}
		usersByID := make(map[int]*model.User, len(users))
		for i := range users {
			usersByID[users[i].Id] = &users[i]
		}
		previousOwner := usersByID[organization.OwnerUserId]
		newOwner := usersByID[params.NewOwnerUserID]
		if previousOwner == nil || newOwner == nil ||
			previousOwner.OrganizationId != organization.Id ||
			previousOwner.OrganizationRole != model.OrganizationRoleOwner {
			return ErrOrganizationOwnerInvalid
		}
		if newOwner.Status != common.UserStatusEnabled ||
			newOwner.OrganizationId != organization.Id ||
			newOwner.OrganizationStatus != model.OrganizationMemberStatusActive ||
			(newOwner.OrganizationRole != model.OrganizationRoleMember && newOwner.OrganizationRole != model.OrganizationRoleAdmin) {
			return ErrOrganizationOwnerInvalid
		}

		var owners []model.User
		if err := model.LockForUpdate(tx).
			Where("organization_id = ? AND organization_role = ?", organization.Id, model.OrganizationRoleOwner).
			Order("id asc").Find(&owners).Error; err != nil {
			return err
		}
		if len(owners) != 1 || owners[0].Id != previousOwner.Id {
			return ErrOrganizationOwnerConflict
		}

		organizationUpdate := tx.Model(&model.Organization{}).
			Where("id = ? AND owner_user_id = ?", organization.Id, previousOwner.Id).
			Update("owner_user_id", newOwner.Id)
		if organizationUpdate.Error != nil {
			return organizationUpdate.Error
		}
		if organizationUpdate.RowsAffected != 1 {
			return ErrOrganizationOwnerConflict
		}
		previousOwnerUpdate := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND organization_role = ?", previousOwner.Id, organization.Id, model.OrganizationRoleOwner).
			Update("organization_role", model.OrganizationRoleAdmin)
		if previousOwnerUpdate.Error != nil {
			return previousOwnerUpdate.Error
		}
		if previousOwnerUpdate.RowsAffected != 1 {
			return ErrOrganizationOwnerConflict
		}
		newOwnerUpdate := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND organization_role = ?", newOwner.Id, organization.Id, newOwner.OrganizationRole).
			Update("organization_role", model.OrganizationRoleOwner)
		if newOwnerUpdate.Error != nil {
			return newOwnerUpdate.Error
		}
		if newOwnerUpdate.RowsAffected != 1 {
			return ErrOrganizationOwnerConflict
		}
		organization.OwnerUserId = newOwner.Id

		for _, userID := range []int{previousOwner.Id, newOwner.Id} {
			change, err := model.AdvanceOrganizationUserAuthorizationWithTx(tx, userID)
			if err != nil {
				return err
			}
			changes = append(changes, change)
		}
		return organizationAuditTx(tx, organization.Id, actorUserID, "organization.ownership.transfer", "organization", strconv.Itoa(organization.Id), requestID, map[string]interface{}{
			"previous_owner_user_id": previousOwner.Id,
			"new_owner_user_id":      newOwner.Id,
			"previous_owner_role":    string(model.OrganizationRoleAdmin),
		})
	})
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		if err := model.FinalizeOrganizationAuthorizationChanges(changes, "organization_ownership_transferred"); err != nil {
			return &organization, err
		}
	}
	return &organization, nil
}

// String helpers are exported for controllers and clients that need stable
// error/action labels without duplicating literal values.
func OrganizationManagementErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPlatformProvisioningForbidden):
		return "PLATFORM_ORGANIZATION_FORBIDDEN"
	case errors.Is(err, ErrOrganizationOwnerConflict):
		return "ORGANIZATION_OWNER_CONFLICT"
	case errors.Is(err, ErrOrganizationUserAlreadyAssigned):
		return "ORGANIZATION_USER_ALREADY_ASSIGNED"
	case errors.Is(err, ErrOrganizationOwnerDemotionForbidden):
		return "ORGANIZATION_OWNER_DEMOTION_FORBIDDEN"
	case errors.Is(err, ErrOrganizationOwnerInvalid):
		return "ORGANIZATION_OWNER_INVALID"
	case errors.Is(err, ErrOrganizationStatusInvalid):
		return "ORGANIZATION_STATUS_INVALID"
	case errors.Is(err, ErrOrganizationOwnerUsernameRequired):
		return "ORGANIZATION_OWNER_USERNAME_REQUIRED"
	case errors.Is(err, ErrOrganizationOwnerPasswordInvalid):
		return "ORGANIZATION_OWNER_PASSWORD_INVALID"
	case errors.Is(err, ErrOrganizationOwnerAccountInvalid):
		return "ORGANIZATION_OWNER_ACCOUNT_INVALID"
	case errors.Is(err, ErrOrganizationMemberUsernameRequired):
		return "ORGANIZATION_MEMBER_USERNAME_REQUIRED"
	case errors.Is(err, ErrOrganizationMemberPasswordInvalid):
		return "ORGANIZATION_MEMBER_PASSWORD_INVALID"
	case errors.Is(err, ErrOrganizationMemberAccountInvalid):
		return "ORGANIZATION_MEMBER_ACCOUNT_INVALID"
	case errors.Is(err, ErrOrganizationMemberRoleInvalid):
		return "ORGANIZATION_MEMBER_ROLE_INVALID"
	case errors.Is(err, model.ErrOrganizationNotActive):
		return "ORGANIZATION_INACTIVE"
	case errors.Is(err, model.ErrOrganizationAccountingForbidden):
		return "ORGANIZATION_ACCOUNTING_FORBIDDEN"
	case errors.Is(err, model.ErrOrganizationFundOverflow):
		return "ORGANIZATION_FUND_OVERFLOW"
	case errors.Is(err, model.ErrOrganizationAccountingIdempotency):
		return "ORGANIZATION_ACCOUNTING_IDEMPOTENCY_CONFLICT"
	case errors.Is(err, model.ErrOrganizationAccountingInvalid):
		return "ORGANIZATION_ACCOUNTING_INVALID"
	default:
		return "ORGANIZATION_MANAGEMENT_INVALID"
	}
}
