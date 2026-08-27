package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOrganizationAuthServiceTest(t *testing.T) (*model.Organization, *model.User) {
	t.Helper()
	previousDB, previousRedis := model.DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
	})

	organization := &model.Organization{
		Name:          "Organization A",
		Status:        model.OrganizationStatusActive,
		OwnerUserId:   1001,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	user := &model.User{
		Username:           "organization-owner",
		Password:           "unused-password-hash",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationRole:   model.OrganizationRoleOwner,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		AuthVersion:        1,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Model(organization).Update("owner_user_id", user.Id).Error)
	organization.OwnerUserId = user.Id
	return organization, user
}

func TestResolveOrganizationPrincipalValidatesOrganizationAndMembership(t *testing.T) {
	organization, user := setupOrganizationAuthServiceTest(t)

	principal, err := ResolveOrganizationPrincipal(user.ToBaseUser())
	require.NoError(t, err)
	assert.Equal(t, user.Id, principal.UserID)
	assert.Equal(t, organization.Id, principal.OrganizationID)
	assert.Equal(t, model.OrganizationRoleOwner, principal.Role)

	user.OrganizationStatus = model.OrganizationMemberStatusDisabled
	_, err = ResolveOrganizationPrincipal(user.ToBaseUser())
	assert.ErrorIs(t, err, ErrOrganizationMembershipInactive)

	user.OrganizationStatus = model.OrganizationMemberStatusActive
	require.NoError(t, model.DB.Model(organization).Update("status", model.OrganizationStatusDisabled).Error)
	_, err = ResolveOrganizationPrincipal(user.ToBaseUser())
	assert.ErrorIs(t, err, ErrOrganizationInactive)

	principal, err = ResolveOrganizationPrincipalAllowInactive(user.ToBaseUser())
	require.NoError(t, err)
	assert.Equal(t, organization.Id, principal.OrganizationID)
}

func TestOrganizationActionMatrixDoesNotUsePlatformRoleAsTenantBypass(t *testing.T) {
	owner := OrganizationPrincipal{UserID: 1, OrganizationID: 10, Role: model.OrganizationRoleOwner}
	admin := OrganizationPrincipal{UserID: 2, OrganizationID: 10, Role: model.OrganizationRoleAdmin}
	memberPlatformAdmin := OrganizationPrincipal{
		UserID: 3, OrganizationID: 10, Role: model.OrganizationRoleMember, PlatformRole: common.RoleAdminUser,
	}

	assert.True(t, CanOrganizationAction(owner, OrganizationActionMemberAllocate))
	assert.True(t, CanOrganizationAction(admin, OrganizationActionMemberAllocate))
	assert.True(t, CanOrganizationAction(owner, OrganizationActionFundTopup))
	assert.True(t, CanOrganizationAction(admin, OrganizationActionFundTopup))
	assert.True(t, CanOrganizationAction(memberPlatformAdmin, OrganizationActionRead))
	assert.False(t, CanOrganizationAction(memberPlatformAdmin, OrganizationActionMemberRead))
	assert.False(t, CanOrganizationAction(memberPlatformAdmin, OrganizationActionBillingRead))
	assert.False(t, CanOrganizationAction(memberPlatformAdmin, OrganizationActionFundTopup))
}

func TestResolveOrganizationPrincipalRejectsForgedOwnerRole(t *testing.T) {
	organization, user := setupOrganizationAuthServiceTest(t)
	require.NoError(t, model.DB.Model(organization).Update("owner_user_id", user.Id+100).Error)

	_, err := ResolveOrganizationPrincipal(user.ToBaseUser())
	assert.ErrorIs(t, err, ErrOrganizationIdentityInvalid)
	_, err = ResolveOrganizationPrincipalAllowInactive(user.ToBaseUser())
	assert.ErrorIs(t, err, ErrOrganizationIdentityInvalid)
}

func TestAuthorizeOrganizationTargetHidesCrossOrganizationAndProtectsPrivilegedRoles(t *testing.T) {
	principal := OrganizationPrincipal{UserID: 1, OrganizationID: 10, Role: model.OrganizationRoleAdmin}

	err := AuthorizeOrganizationTarget(principal, 11, model.OrganizationRoleMember, OrganizationActionMemberRead)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	err = AuthorizeOrganizationTarget(principal, 10, model.OrganizationRoleOwner, OrganizationActionMemberDisable)
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)

	err = AuthorizeOrganizationTarget(principal, 10, model.OrganizationRoleAdmin, OrganizationActionMemberRead)
	require.NoError(t, err)
	err = AuthorizeOrganizationTarget(principal, 10, model.OrganizationRoleMember, OrganizationActionMemberDisable)
	require.NoError(t, err)
}

func TestRequirePlatformOrganizationProvisionerRereadsPlatformRole(t *testing.T) {
	_, user := setupOrganizationAuthServiceTest(t)

	_, err := RequirePlatformOrganizationProvisioner(user.Id)
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)

	require.NoError(t, model.DB.Model(user).Update("role", common.RoleAdminUser).Error)
	actor, err := RequirePlatformOrganizationProvisioner(user.Id)
	require.NoError(t, err)
	assert.Equal(t, common.RoleAdminUser, actor.Role)

	require.NoError(t, model.DB.Model(user).Update("status", common.UserStatusDisabled).Error)
	_, err = RequirePlatformOrganizationProvisioner(user.Id)
	assert.True(t, errors.Is(err, ErrPlatformProvisioningForbidden))
}
