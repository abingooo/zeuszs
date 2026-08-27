package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationInviteLifecycleStoresOnlyHashAndAudits(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "invite-platform-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "invite-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	created, err := CreateOrganizationInviteForPlatform(actor.Id, CreateOrganizationInviteParams{
		OrganizationID: organization.Id,
		Code:           "lab-access",
		MaxUses:        2,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
		RequestID:      "invite-create-1",
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "LAB-ACCESS", created.Code)
	assert.Equal(t, "LAB-AC", created.CodePrefix)
	assert.Equal(t, model.OrganizationRoleMember, created.DefaultRole)

	var persisted model.OrganizationInvite
	require.NoError(t, db.First(&persisted, created.ID).Error)
	assert.NotEqual(t, persisted.CodeHash, created.Code)
	assert.Equal(t, HashOrganizationInviteCode(created.Code), persisted.CodeHash)

	listed, err := ListOrganizationInvitesForPlatform(actor.Id, organization.Id, ListOrganizationInvitesParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, created.CodePrefix, listed.Items[0].CodePrefix)
	assert.NotContains(t, fmt.Sprintf("%+v", listed.Items[0]), "LAB-ACCESS")

	disabled, err := DisableOrganizationInviteForPlatform(actor.Id, DisableOrganizationInviteParams{
		OrganizationID: organization.Id,
		InviteID:       created.ID,
		RequestID:      "invite-disable-1",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationInviteStatusDisabled, disabled.Status)
	var audits []model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ?", organization.Id).Order("id asc").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "organization.invite.create", audits[0].Action)
	assert.Equal(t, "organization.invite.disable", audits[1].Action)
}

func TestOrganizationInviteManagementRejectsTenantOwnerAndExpiredOrDuplicateCodes(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	owner := createOrganizationManagementUser(t, db, "invite-tenant-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	_, err := CreateOrganizationInviteForPlatform(owner.Id, CreateOrganizationInviteParams{OrganizationID: organization.Id, Code: "TENANT-ONE"})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)

	platformAdmin := createOrganizationManagementUser(t, db, "invite-platform-admin-2", common.RoleAdminUser, 0, "")
	_, err = CreateOrganizationInviteForPlatform(platformAdmin.Id, CreateOrganizationInviteParams{
		OrganizationID: organization.Id,
		Code:           "expired-1",
		ExpiresAt:      time.Now().Add(-time.Minute).Unix(),
	})
	assert.ErrorIs(t, err, ErrOrganizationInviteExpiryInvalid)
	_, err = CreateOrganizationInviteForPlatform(platformAdmin.Id, CreateOrganizationInviteParams{OrganizationID: organization.Id, Code: "TENANT-ONE"})
	require.NoError(t, err)
	_, err = CreateOrganizationInviteForPlatform(platformAdmin.Id, CreateOrganizationInviteParams{OrganizationID: organization.Id, Code: "tenant-one"})
	assert.ErrorIs(t, err, ErrOrganizationInviteConflict)
}

func TestListOrganizationMembersForPlatformReturnsSafeFundingProjection(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "member-list-admin", common.RoleRootUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "member-list-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	member := createOrganizationManagementUser(t, db, "member-list-member", common.RoleCommonUser, organization.Id, model.OrganizationRoleMember)
	member.Quota = 900
	member.UsedQuota = 100
	member.Password = "must-not-leak"
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
		"quota":               900,
		"used_quota":          100,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	limit := int64(700)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId:   organization.Id,
		UserId:           member.Id,
		RecoverableQuota: 300,
		ConsumedQuota:    100,
		ConsumptionLimit: &limit,
	}).Error)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{OrganizationId: organization.Id, UserId: owner.Id}).Error)

	result, err := ListOrganizationMembersForPlatform(actor.Id, organization.Id, ListOrganizationMembersParams{Limit: 10, Keyword: "member-list-member"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	view := result.Items[0]
	assert.Equal(t, member.Id, view.UserID)
	assert.Equal(t, int64(300), view.RecoverableQuota)
	assert.Equal(t, int64(100), view.ConsumedQuota)
	assert.Equal(t, &limit, view.ConsumptionLimit)

	_, err = ListOrganizationMembersForPlatform(owner.Id, organization.Id, ListOrganizationMembersParams{Limit: 10})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
}

func TestUpdateOrganizationTopupPolicyForPlatformBumpsPolicyAndAuthVersions(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "policy-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "policy-owner", common.RoleCommonUser, 0, "")
	member := createOrganizationManagementUser(t, db, "policy-member", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	for _, user := range []model.User{owner, member} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
			"organization_id": organization.Id,
			"organization_role": func() model.OrganizationRole {
				if user.Id == owner.Id {
					return model.OrganizationRoleOwner
				}
				return model.OrganizationRoleMember
			}(),
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{SID: "policy-session", UserID: member.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600}).Error)

	updated, err := UpdateOrganizationTopupPolicyForPlatform(actor.Id, UpdateOrganizationTopupPolicyParams{
		OrganizationID:   organization.Id,
		AllowMemberTopup: false,
		RequestID:        "policy-update-1",
	})
	require.NoError(t, err)
	assert.False(t, updated.AllowMemberTopup)
	assert.EqualValues(t, 2, updated.PolicyVersion)
	var users []model.User
	require.NoError(t, db.Where("organization_id = ?", organization.Id).Order("id asc").Find(&users).Error)
	for _, user := range users {
		assert.EqualValues(t, 2, user.AuthVersion)
	}
	var session model.UserSession
	require.NoError(t, db.Where("sid = ?", "policy-session").First(&session).Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)

	unchanged, err := UpdateOrganizationTopupPolicyForPlatform(actor.Id, UpdateOrganizationTopupPolicyParams{OrganizationID: organization.Id, AllowMemberTopup: false})
	require.NoError(t, err)
	assert.EqualValues(t, 2, unchanged.PolicyVersion)
	var auditCount int64
	require.NoError(t, db.Model(&model.OrganizationAuditEvent{}).Where("action = ?", "organization.topup_policy.update").Count(&auditCount).Error)
	assert.EqualValues(t, 1, auditCount)
	assert.False(t, errors.Is(err, ErrOrganizationActionForbidden))
	assert.True(t, strings.Contains(unchanged.Name, "Management"))
}
