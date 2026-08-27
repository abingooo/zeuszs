package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var (
	ErrOrganizationIdentityInvalid    = model.ErrOrganizationIdentityInvalid
	ErrOrganizationInactive           = errors.New("organization is inactive")
	ErrOrganizationMembershipInactive = errors.New("organization membership is inactive")
	ErrOrganizationActionForbidden    = errors.New("organization action is forbidden")
	ErrPlatformProvisioningForbidden  = errors.New("platform organization provisioning is forbidden")
)

// OrganizationPrincipal is derived from server-side user and organization
// state. Request payloads and headers must never be used to populate it.
type OrganizationPrincipal struct {
	UserID         int
	OrganizationID int
	Role           model.OrganizationRole
	PlatformRole   int
}

type OrganizationAction string

const (
	OrganizationActionRead               OrganizationAction = "organization.read"
	OrganizationActionInviteRead         OrganizationAction = "invite.read"
	OrganizationActionInviteCreate       OrganizationAction = "invite.create"
	OrganizationActionInviteDisable      OrganizationAction = "invite.disable"
	OrganizationActionMemberRead         OrganizationAction = "member.read"
	OrganizationActionMemberDisable      OrganizationAction = "member.disable"
	OrganizationActionMemberRemove       OrganizationAction = "member.remove"
	OrganizationActionMemberQuotaRead    OrganizationAction = "member.quota.read"
	OrganizationActionMemberAllocate     OrganizationAction = "member.quota.allocate"
	OrganizationActionMemberRecover      OrganizationAction = "member.quota.recover"
	OrganizationActionMemberLimitUpdate  OrganizationAction = "member.limit.update"
	OrganizationActionTopupPolicyUpdate  OrganizationAction = "member.topup_policy.update"
	OrganizationActionMemberTokenDisable OrganizationAction = "member.token.disable"
	OrganizationActionBillingRead        OrganizationAction = "billing.read"
	OrganizationActionLedgerRead         OrganizationAction = "ledger.read"
	OrganizationActionAuditRead          OrganizationAction = "audit.read"
)

var organizationAdminActions = map[OrganizationAction]struct{}{
	OrganizationActionRead:               {},
	OrganizationActionInviteRead:         {},
	OrganizationActionInviteCreate:       {},
	OrganizationActionInviteDisable:      {},
	OrganizationActionMemberRead:         {},
	OrganizationActionMemberDisable:      {},
	OrganizationActionMemberRemove:       {},
	OrganizationActionMemberQuotaRead:    {},
	OrganizationActionMemberAllocate:     {},
	OrganizationActionMemberRecover:      {},
	OrganizationActionMemberLimitUpdate:  {},
	OrganizationActionTopupPolicyUpdate:  {},
	OrganizationActionMemberTokenDisable: {},
	OrganizationActionBillingRead:        {},
	OrganizationActionLedgerRead:         {},
	OrganizationActionAuditRead:          {},
}

var organizationMemberMutationActions = map[OrganizationAction]struct{}{
	OrganizationActionMemberDisable:      {},
	OrganizationActionMemberRemove:       {},
	OrganizationActionMemberAllocate:     {},
	OrganizationActionMemberRecover:      {},
	OrganizationActionMemberLimitUpdate:  {},
	OrganizationActionMemberTokenDisable: {},
}

// ResolveOrganizationPrincipal validates both the user's active membership and
// the current organization row. Platform roles never grant tenant access.
func ResolveOrganizationPrincipal(user *model.UserBase) (OrganizationPrincipal, error) {
	principal, organizationStatus, membershipStatus, err := resolveOrganizationPrincipal(user)
	if err != nil {
		return OrganizationPrincipal{}, err
	}
	if membershipStatus != model.OrganizationMemberStatusActive {
		return OrganizationPrincipal{}, ErrOrganizationMembershipInactive
	}
	if organizationStatus != model.OrganizationStatusActive {
		return OrganizationPrincipal{}, ErrOrganizationInactive
	}
	return principal, nil
}

// ResolveOrganizationPrincipalAllowInactive is limited to self status views
// and platform recovery flows. It still rejects malformed or missing tenant
// identities, but exposes the principal when either status is inactive.
func ResolveOrganizationPrincipalAllowInactive(user *model.UserBase) (OrganizationPrincipal, error) {
	principal, _, _, err := resolveOrganizationPrincipal(user)
	return principal, err
}

func resolveOrganizationPrincipal(user *model.UserBase) (OrganizationPrincipal, model.OrganizationStatus, model.OrganizationMemberStatus, error) {
	if user == nil || user.Id <= 0 || user.OrganizationId <= 0 || !validOrganizationRole(user.OrganizationRole) || !validOrganizationMemberStatus(user.OrganizationStatus) {
		return OrganizationPrincipal{}, "", "", ErrOrganizationIdentityInvalid
	}
	var organization model.Organization
	if err := model.DB.Select("id", "status", "owner_user_id").Where("id = ?", user.OrganizationId).First(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return OrganizationPrincipal{}, "", "", ErrOrganizationIdentityInvalid
		}
		return OrganizationPrincipal{}, "", "", fmt.Errorf("resolve organization principal: %w", err)
	}
	if !validOrganizationStatus(organization.Status) {
		return OrganizationPrincipal{}, "", "", ErrOrganizationIdentityInvalid
	}
	if user.OrganizationRole == model.OrganizationRoleOwner && organization.OwnerUserId != user.Id {
		return OrganizationPrincipal{}, "", "", ErrOrganizationIdentityInvalid
	}
	return OrganizationPrincipal{
		UserID:         user.Id,
		OrganizationID: user.OrganizationId,
		Role:           user.OrganizationRole,
		PlatformRole:   user.Role,
	}, organization.Status, user.OrganizationStatus, nil
}

func validOrganizationRole(role model.OrganizationRole) bool {
	switch role {
	case model.OrganizationRoleOwner, model.OrganizationRoleAdmin, model.OrganizationRoleMember:
		return true
	default:
		return false
	}
}

func validOrganizationStatus(status model.OrganizationStatus) bool {
	switch status {
	case model.OrganizationStatusActive, model.OrganizationStatusDisabled,
		model.OrganizationStatusDissolving, model.OrganizationStatusDissolved:
		return true
	default:
		return false
	}
}

func validOrganizationMemberStatus(status model.OrganizationMemberStatus) bool {
	switch status {
	case model.OrganizationMemberStatusActive, model.OrganizationMemberStatusDisabled:
		return true
	default:
		return false
	}
}

// CanOrganizationAction applies the fixed Phase 1 organization permission
// matrix. It deliberately ignores PlatformRole so platform administrators do
// not gain ambient tenant access.
func CanOrganizationAction(principal OrganizationPrincipal, action OrganizationAction) bool {
	if principal.UserID <= 0 || principal.OrganizationID <= 0 {
		return false
	}
	switch principal.Role {
	case model.OrganizationRoleOwner:
		_, ok := organizationAdminActions[action]
		return ok
	case model.OrganizationRoleAdmin:
		_, ok := organizationAdminActions[action]
		return ok
	case model.OrganizationRoleMember:
		return action == OrganizationActionRead
	default:
		return false
	}
}

// AuthorizeOrganizationTarget checks tenant isolation before permissions so a
// cross-organization identifier is indistinguishable from a missing resource.
// Owner and Admin may mutate Member targets only.
func AuthorizeOrganizationTarget(principal OrganizationPrincipal, targetOrganizationID int, targetRole model.OrganizationRole, action OrganizationAction) error {
	if principal.OrganizationID <= 0 || targetOrganizationID <= 0 || principal.OrganizationID != targetOrganizationID {
		return gorm.ErrRecordNotFound
	}
	if !CanOrganizationAction(principal, action) {
		return ErrOrganizationActionForbidden
	}
	if _, mutation := organizationMemberMutationActions[action]; mutation && targetRole != model.OrganizationRoleMember {
		return ErrOrganizationActionForbidden
	}
	return nil
}

// AuthorizeOrganizationSelf is for organization.read endpoints exposed to a
// Member. Members may inspect their own organization membership summary only;
// Owner/Admin handlers should use AuthorizeOrganizationTarget for member data.
func AuthorizeOrganizationSelf(principal OrganizationPrincipal, targetUserID int, action OrganizationAction) error {
	if targetUserID <= 0 || principal.UserID != targetUserID {
		return gorm.ErrRecordNotFound
	}
	if !CanOrganizationAction(principal, action) {
		return ErrOrganizationActionForbidden
	}
	return nil
}

// LoadOrganizationTarget performs the target lookup on the server and applies
// tenant isolation and the fixed target-role restrictions.
func LoadOrganizationTarget(principal OrganizationPrincipal, targetUserID int, action OrganizationAction) (*model.User, error) {
	if targetUserID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var target model.User
	if err := model.DB.Omit("password", "access_token").Where("id = ?", targetUserID).First(&target).Error; err != nil {
		return nil, err
	}
	if principal.Role == model.OrganizationRoleMember && action == OrganizationActionRead {
		if err := AuthorizeOrganizationSelf(principal, targetUserID, action); err != nil {
			return nil, err
		}
	}
	if err := AuthorizeOrganizationTarget(principal, target.OrganizationId, target.OrganizationRole, action); err != nil {
		return nil, err
	}
	return &target, nil
}

// RequirePlatformOrganizationProvisioner re-reads the platform role from the
// database. It is only for explicit create/role-assignment provisioning paths.
func RequirePlatformOrganizationProvisioner(actorUserID int) (*model.User, error) {
	if actorUserID <= 0 || model.DB == nil {
		return nil, ErrPlatformProvisioningForbidden
	}
	var actor model.User
	if err := model.DB.Select("id", "role", "status").Where("id = ?", actorUserID).First(&actor).Error; err != nil {
		return nil, err
	}
	// Only the two built-in platform roles may provision organizations. Do not
	// use a numeric >= comparison here: an unknown/corrupt role value must not
	// acquire platform provisioning authority by accident.
	if actor.Status != common.UserStatusEnabled ||
		(actor.Role != common.RoleAdminUser && actor.Role != common.RoleRootUser) {
		return nil, ErrPlatformProvisioningForbidden
	}
	return &actor, nil
}
