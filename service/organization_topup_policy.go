package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var ErrOrganizationMemberTopupDisabled = errors.New("organization member top-up is disabled")

type OrganizationTopupPolicy struct {
	OrganizationID   int                            `json:"organization_id"`
	OrganizationRole model.OrganizationRole         `json:"organization_role"`
	MemberStatus     model.OrganizationMemberStatus `json:"organization_status"`
	AllowMemberTopup bool                           `json:"allow_member_topup"`
	PolicyVersion    int64                          `json:"policy_version"`
	Allowed          bool                           `json:"allowed"`
}

// GetOrganizationTopupPolicyForUser resolves the current user and
// organization rows on every payment attempt. Only ordinary organization
// Members are governed by AllowMemberTopup; Owner/Admin retain access so they
// can manage organization funding even when member self-funding is disabled.
func GetOrganizationTopupPolicyForUser(userID int) (*OrganizationTopupPolicy, error) {
	if userID <= 0 || model.DB == nil {
		return nil, ErrOrganizationIdentityInvalid
	}
	var user model.User
	if err := model.DB.Select("id", "status", "organization_id", "organization_role", "organization_status").Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationIdentityInvalid
		}
		return nil, err
	}
	if user.Status != common.UserStatusEnabled || user.OrganizationStatus != model.OrganizationMemberStatusActive {
		return nil, ErrOrganizationMembershipInactive
	}
	if user.OrganizationId <= 0 || !validOrganizationRole(user.OrganizationRole) {
		return nil, ErrOrganizationIdentityInvalid
	}
	var organization model.Organization
	if err := model.DB.Select("id", "status", "allow_member_topup", "policy_version").Where("id = ?", user.OrganizationId).First(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationIdentityInvalid
		}
		return nil, err
	}
	if organization.Status != model.OrganizationStatusActive {
		return nil, ErrOrganizationInactive
	}
	policy := &OrganizationTopupPolicy{
		OrganizationID:   organization.Id,
		OrganizationRole: user.OrganizationRole,
		MemberStatus:     user.OrganizationStatus,
		AllowMemberTopup: organization.AllowMemberTopup,
		PolicyVersion:    organization.PolicyVersion,
		Allowed:          user.OrganizationRole != model.OrganizationRoleMember || organization.AllowMemberTopup,
	}
	return policy, nil
}

func RequireOrganizationWalletTopup(userID int) (*OrganizationTopupPolicy, error) {
	policy, err := GetOrganizationTopupPolicyForUser(userID)
	if err != nil {
		return nil, err
	}
	if !policy.Allowed {
		return policy, ErrOrganizationMemberTopupDisabled
	}
	return policy, nil
}
