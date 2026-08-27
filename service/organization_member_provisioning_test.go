package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionOrganizationMemberForPlatformCreatesAdminAtomically(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	actor := createOrganizationManagementUser(t, db, "member-provision-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "member-provision-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	previousInitialQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 275
	t.Cleanup(func() { common.QuotaForNewUser = previousInitialQuota })

	member, err := ProvisionOrganizationMemberForPlatform(actor.Id, ProvisionOrganizationMemberParams{
		OrganizationID: organization.Id,
		Username:       "provisioned-admin",
		Password:       "password123",
		DisplayName:    "Provisioned Admin",
		Email:          " ADMIN@EXAMPLE.COM ",
		Role:           model.OrganizationRoleAdmin,
		RequestID:      "member-provision-request",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationRoleAdmin, member.OrganizationRole)
	assert.Equal(t, organization.Id, member.OrganizationID)
	assert.Equal(t, 275, member.Quota)
	assert.Equal(t, common.RoleCommonUser, member.PlatformRole)

	var persisted model.User
	require.NoError(t, db.First(&persisted, member.UserID).Error)
	assert.Equal(t, "admin@example.com", persisted.Email)
	assert.Equal(t, model.OrganizationRoleAdmin, persisted.OrganizationRole)
	assert.NotEqual(t, "password123", persisted.Password)

	var fund model.OrganizationMemberFund
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, persisted.Id).First(&fund).Error)
	var ledger model.OrganizationQuotaLedger
	require.NoError(t, db.Where("organization_id = ? AND user_id = ? AND source_type = ?", organization.Id, persisted.Id, "platform_member_grant").First(&ledger).Error)
	assert.EqualValues(t, 275, ledger.UserQuotaDelta)
	assert.Equal(t, actor.Id, ledger.ActorUserId)
	var audit model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ? AND action = ? AND target_id = ?", organization.Id, "organization.member.provision", persisted.Id).First(&audit).Error)
	assert.Equal(t, "member-provision-request", audit.RequestId)
}

func TestProvisionOrganizationMemberForPlatformRejectsOwnerAndTenantActor(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	platformAdmin := createOrganizationManagementUser(t, db, "member-provision-platform", common.RoleAdminUser, 0, "")
	tenantOwner := createOrganizationManagementUser(t, db, "member-provision-tenant-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, tenantOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", tenantOwner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	_, err := ProvisionOrganizationMemberForPlatform(platformAdmin.Id, ProvisionOrganizationMemberParams{
		OrganizationID: organization.Id,
		Username:       "forbidden-owner",
		Password:       "password123",
		Role:           model.OrganizationRoleOwner,
	})
	assert.ErrorIs(t, err, ErrOrganizationMemberRoleInvalid)

	_, err = ProvisionOrganizationMemberForPlatform(tenantOwner.Id, ProvisionOrganizationMemberParams{
		OrganizationID: organization.Id,
		Username:       "forbidden-admin",
		Password:       "password123",
		Role:           model.OrganizationRoleAdmin,
	})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)

	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username IN ?", []string{"forbidden-owner", "forbidden-admin"}).Count(&count).Error)
	assert.Zero(t, count)
}
