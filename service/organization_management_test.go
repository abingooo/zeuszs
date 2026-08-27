package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOrganizationManagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Token{}, &model.Log{},
		&model.Organization{}, &model.OrganizationInvite{}, &model.OrganizationInviteUse{},
		&model.OrganizationFundAccount{}, &model.OrganizationMemberFund{}, &model.OrganizationAuditEvent{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createOrganizationManagementUser(t *testing.T, db *gorm.DB, username string, role int, organizationID int, organizationRole model.OrganizationRole) model.User {
	t.Helper()
	user := model.User{
		Username:           username,
		Password:           "password-hash",
		Role:               role,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organizationID,
		OrganizationRole:   organizationRole,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		AuthVersion:        1,
		Group:              "default",
		AffCode:            username + "-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createOrganizationManagementOrganization(t *testing.T, db *gorm.DB, ownerID int, status model.OrganizationStatus) model.Organization {
	t.Helper()
	organization := model.Organization{
		Name:             "Management Organization",
		Status:           status,
		OwnerUserId:      ownerID,
		AllowMemberTopup: true,
		PolicyVersion:    1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	return organization
}

func TestCreateOrganizationForPlatformAssignsOwnerAtomically(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "platform-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "new-owner", common.RoleCommonUser, 0, "")

	organization, err := CreateOrganizationForPlatform(actor.Id, CreateOrganizationParams{
		Name:             "  Research Lab  ",
		OwnerUserID:      owner.Id,
		AllowMemberTopup: common.GetPointer(false),
		RequestID:        "management-create-1",
	})
	require.NoError(t, err)
	require.NotNil(t, organization)
	assert.Equal(t, "Research Lab", organization.Name)
	assert.Equal(t, model.OrganizationStatusActive, organization.Status)
	assert.Equal(t, owner.Id, organization.OwnerUserId)
	assert.False(t, organization.AllowMemberTopup)

	var persistedOwner model.User
	require.NoError(t, db.First(&persistedOwner, owner.Id).Error)
	assert.Equal(t, organization.Id, persistedOwner.OrganizationId)
	assert.Equal(t, model.OrganizationRoleOwner, persistedOwner.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, persistedOwner.OrganizationStatus)
	assert.EqualValues(t, 2, persistedOwner.AuthVersion)

	var fund model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&fund).Error)
	var memberFund model.OrganizationMemberFund
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, owner.Id).First(&memberFund).Error)
	var audit model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ? AND action = ?", organization.Id, "organization.create").First(&audit).Error)
	assert.Equal(t, actor.Id, audit.ActorUserId)
	assert.Equal(t, "management-create-1", audit.RequestId)
}

func TestOrganizationManagementRejectsTenantOwnerWithoutPlatformRole(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "tenant-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, actor.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", actor.Id).Updates(map[string]interface{}{
		"organization_id":   organization.Id,
		"organization_role": model.OrganizationRoleOwner,
	}).Error)
	ownerCandidate := createOrganizationManagementUser(t, db, "candidate", common.RoleCommonUser, 0, "")

	_, err := CreateOrganizationForPlatform(actor.Id, CreateOrganizationParams{Name: "Rejected", OwnerUserID: ownerCandidate.Id})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
	var count int64
	require.NoError(t, db.Model(&model.Organization{}).Where("name = ?", "Rejected").Count(&count).Error)
	assert.Zero(t, count)
}

func TestAssignOrganizationMemberRoleForPlatformPreservesSingleOrganizationInvariant(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "platform-root", common.RoleRootUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "organization-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":   organization.Id,
		"organization_role": model.OrganizationRoleOwner,
	}).Error)
	otherOwner := createOrganizationManagementUser(t, db, "other-owner", common.RoleCommonUser, 0, "")
	member := createOrganizationManagementUser(t, db, "member", common.RoleCommonUser, 0, "")

	assigned, err := AssignOrganizationMemberRoleForPlatform(actor.Id, AssignOrganizationRoleParams{
		OrganizationID: organization.Id,
		UserID:         member.Id,
		Role:           model.OrganizationRoleMember,
		RequestID:      "management-member-1",
	})
	require.NoError(t, err)
	assert.Equal(t, organization.Id, assigned.OrganizationId)
	assert.Equal(t, model.OrganizationRoleMember, assigned.OrganizationRole)
	assert.EqualValues(t, 2, assigned.AuthVersion)

	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id":   otherOrganization.Id,
		"organization_role": model.OrganizationRoleOwner,
	}).Error)
	_, err = AssignOrganizationMemberRoleForPlatform(actor.Id, AssignOrganizationRoleParams{
		OrganizationID: otherOrganization.Id,
		UserID:         member.Id,
		Role:           model.OrganizationRoleMember,
	})
	assert.ErrorIs(t, err, ErrOrganizationUserAlreadyAssigned)

	_, err = AssignOrganizationMemberRoleForPlatform(actor.Id, AssignOrganizationRoleParams{
		OrganizationID: organization.Id,
		UserID:         otherOwner.Id,
		Role:           model.OrganizationRoleOwner,
	})
	assert.ErrorIs(t, err, ErrOrganizationUserAlreadyAssigned)

	_, err = AssignOrganizationMemberRoleForPlatform(actor.Id, AssignOrganizationRoleParams{
		OrganizationID: organization.Id,
		UserID:         owner.Id,
		Role:           model.OrganizationRoleAdmin,
	})
	assert.ErrorIs(t, err, ErrOrganizationOwnerDemotionForbidden)
}

func TestUpdateOrganizationStatusForPlatformInvalidatesMembersAndAudits(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "status-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "status-owner", common.RoleCommonUser, 0, "")
	member := createOrganizationManagementUser(t, db, "status-member", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":   organization.Id,
		"organization_role": model.OrganizationRoleOwner,
	}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
		"organization_id":   organization.Id,
		"organization_role": model.OrganizationRoleMember,
	}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: "organization-status-session", UserID: member.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	updated, err := UpdateOrganizationStatusForPlatform(actor.Id, UpdateOrganizationStatusParams{
		OrganizationID: organization.Id,
		Status:         model.OrganizationStatusDisabled,
		RequestID:      "management-status-1",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationStatusDisabled, updated.Status)
	var users []model.User
	require.NoError(t, db.Where("organization_id = ?", organization.Id).Order("id asc").Find(&users).Error)
	require.Len(t, users, 2)
	for _, user := range users {
		assert.EqualValues(t, 2, user.AuthVersion)
	}
	var session model.UserSession
	require.NoError(t, db.Where("sid = ?", "organization-status-session").First(&session).Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
	var audit model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ? AND action = ?", organization.Id, "organization.status.update").First(&audit).Error)
	assert.Equal(t, actor.Id, audit.ActorUserId)
}

func TestUpdateOrganizationStatusForPlatformEnforcesStateMachine(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "status-machine-admin", common.RoleAdminUser, 0, "")
	testCases := []struct {
		name    string
		from    model.OrganizationStatus
		to      model.OrganizationStatus
		allowed bool
	}{
		{name: "active to disabled", from: model.OrganizationStatusActive, to: model.OrganizationStatusDisabled, allowed: true},
		{name: "active to dissolving", from: model.OrganizationStatusActive, to: model.OrganizationStatusDissolving, allowed: true},
		{name: "active to dissolved", from: model.OrganizationStatusActive, to: model.OrganizationStatusDissolved},
		{name: "disabled to active", from: model.OrganizationStatusDisabled, to: model.OrganizationStatusActive, allowed: true},
		{name: "disabled to dissolving", from: model.OrganizationStatusDisabled, to: model.OrganizationStatusDissolving, allowed: true},
		{name: "disabled to dissolved", from: model.OrganizationStatusDisabled, to: model.OrganizationStatusDissolved},
		{name: "dissolving to active", from: model.OrganizationStatusDissolving, to: model.OrganizationStatusActive, allowed: true},
		{name: "dissolving to disabled", from: model.OrganizationStatusDissolving, to: model.OrganizationStatusDisabled},
		{name: "dissolving to dissolved", from: model.OrganizationStatusDissolving, to: model.OrganizationStatusDissolved, allowed: true},
		{name: "dissolved to active", from: model.OrganizationStatusDissolved, to: model.OrganizationStatusActive},
		{name: "dissolved to disabled", from: model.OrganizationStatusDissolved, to: model.OrganizationStatusDisabled},
		{name: "dissolved to dissolving", from: model.OrganizationStatusDissolved, to: model.OrganizationStatusDissolving},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			owner := createOrganizationManagementUser(t, db, fmt.Sprintf("status-machine-owner-%d", index), common.RoleCommonUser, 0, "")
			organization := createOrganizationManagementOrganization(t, db, owner.Id, testCase.from)
			updated, err := UpdateOrganizationStatusForPlatform(actor.Id, UpdateOrganizationStatusParams{
				OrganizationID: organization.Id,
				Status:         testCase.to,
				RequestID:      fmt.Sprintf("status-machine-%d", index),
			})

			var persisted model.Organization
			require.NoError(t, db.First(&persisted, organization.Id).Error)
			if testCase.allowed {
				require.NoError(t, err)
				require.NotNil(t, updated)
				assert.Equal(t, testCase.to, updated.Status)
				assert.Equal(t, testCase.to, persisted.Status)
				return
			}
			assert.ErrorIs(t, err, ErrOrganizationStatusTransition)
			assert.Nil(t, updated)
			assert.Equal(t, testCase.from, persisted.Status)
		})
	}
}

func TestUpdateDefaultOrganizationStatusForPlatformIsRejectedForRoot(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "default-status-root", common.RoleRootUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "default-status-owner", common.RoleCommonUser, 0, "")
	systemKey := model.DefaultOrganizationSystemKey
	organization := model.Organization{
		Name: "Default", SystemKey: &systemKey, Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)

	for index, status := range []model.OrganizationStatus{
		model.OrganizationStatusDisabled,
		model.OrganizationStatusDissolving,
		model.OrganizationStatusDissolved,
	} {
		t.Run(string(status), func(t *testing.T) {
			updated, err := UpdateOrganizationStatusForPlatform(actor.Id, UpdateOrganizationStatusParams{
				OrganizationID: organization.Id,
				Status:         status,
				RequestID:      fmt.Sprintf("default-status-%d", index),
			})
			assert.ErrorIs(t, err, model.ErrDefaultOrganizationConflict)
			assert.Nil(t, updated)

			var persisted model.Organization
			require.NoError(t, db.First(&persisted, organization.Id).Error)
			assert.Equal(t, model.OrganizationStatusActive, persisted.Status)
		})
	}
}

func TestListOrganizationsForPlatformRequiresPlatformRoleAndIncludesCounts(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "list-admin", common.RoleAdminUser, 0, "")
	owner := createOrganizationManagementUser(t, db, "list-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	member := createOrganizationManagementUser(t, db, "list-member", common.RoleCommonUser, organization.Id, model.OrganizationRoleMember)
	_ = member
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":   organization.Id,
		"organization_role": model.OrganizationRoleOwner,
	}).Error)
	require.NoError(t, db.Model(&model.OrganizationFundAccount{}).
		Where("organization_id = ?", organization.Id).Update("quota", 4321).Error)

	result, err := ListOrganizationsForPlatform(actor.Id, ListOrganizationsParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, int64(2), result.Items[0].MemberCount)
	assert.Equal(t, "list-owner", result.Items[0].OwnerUsername)
	assert.Equal(t, int64(4321), result.Items[0].FundQuota)

	_, err = ListOrganizationsForPlatform(owner.Id, ListOrganizationsParams{Limit: 10})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
	assert.False(t, errors.Is(err, ErrOrganizationActionForbidden))
}

func TestTransferOrganizationOwnershipForPlatformUpdatesBothPrincipalsAndAudit(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "transfer-platform-admin", common.RoleAdminUser, 0, "")
	previousOwner := createOrganizationManagementUser(t, db, "transfer-previous-owner", common.RoleCommonUser, 0, "")
	newOwner := createOrganizationManagementUser(t, db, "transfer-new-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, previousOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", previousOwner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", newOwner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleMember,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	now := time.Now().Unix()
	for _, session := range []model.UserSession{
		{SID: "transfer-previous-session", UserID: previousOwner.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: "transfer-previous-refresh", LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600},
		{SID: "transfer-new-session", UserID: newOwner.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: "transfer-new-refresh", LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600},
	} {
		session := session
		require.NoError(t, db.Create(&session).Error)
	}

	updated, err := TransferOrganizationOwnershipForPlatform(actor.Id, TransferOrganizationOwnershipParams{
		OrganizationID: organization.Id,
		NewOwnerUserID: newOwner.Id,
		RequestID:      "ownership-transfer-1",
	})
	require.NoError(t, err)
	assert.Equal(t, newOwner.Id, updated.OwnerUserId)

	var persistedPrevious, persistedNew model.User
	require.NoError(t, db.First(&persistedPrevious, previousOwner.Id).Error)
	require.NoError(t, db.First(&persistedNew, newOwner.Id).Error)
	assert.Equal(t, model.OrganizationRoleAdmin, persistedPrevious.OrganizationRole)
	assert.Equal(t, model.OrganizationRoleOwner, persistedNew.OrganizationRole)
	assert.EqualValues(t, 2, persistedPrevious.AuthVersion)
	assert.EqualValues(t, 2, persistedNew.AuthVersion)
	var sessions []model.UserSession
	require.NoError(t, db.Where("user_id IN ?", []int{previousOwner.Id, newOwner.Id}).Order("user_id asc").Find(&sessions).Error)
	require.Len(t, sessions, 2)
	for _, session := range sessions {
		assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
		assert.Equal(t, "organization_ownership_transferred", session.RevokedReason)
	}
	var audit model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ? AND action = ?", organization.Id, "organization.ownership.transfer").First(&audit).Error)
	assert.Equal(t, actor.Id, audit.ActorUserId)
	assert.Equal(t, "ownership-transfer-1", audit.RequestId)

	assert.ErrorIs(t, persistedNew.Delete(), model.ErrOrganizationOwnerDeletionForbidden)
	require.NoError(t, persistedPrevious.Delete())
}

func TestTransferOrganizationOwnershipRequiresPlatformRoleAndSameOrganization(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "transfer-root", common.RoleRootUser, 0, "")
	previousOwner := createOrganizationManagementUser(t, db, "transfer-owner", common.RoleCommonUser, 0, "")
	newOwner := createOrganizationManagementUser(t, db, "transfer-member", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, previousOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", previousOwner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", newOwner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleMember,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	_, err := TransferOrganizationOwnershipForPlatform(previousOwner.Id, TransferOrganizationOwnershipParams{
		OrganizationID: organization.Id, NewOwnerUserID: newOwner.Id,
	})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)

	otherOwner := createOrganizationManagementUser(t, db, "transfer-other-owner", common.RoleCommonUser, 0, "")
	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id": otherOrganization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	_, err = TransferOrganizationOwnershipForPlatform(actor.Id, TransferOrganizationOwnershipParams{
		OrganizationID: organization.Id, NewOwnerUserID: otherOwner.Id,
	})
	assert.ErrorIs(t, err, ErrOrganizationOwnerInvalid)
}

func TestTransferDefaultOrganizationOwnershipIsRejected(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	actor := createOrganizationManagementUser(t, db, "default-transfer-root", common.RoleRootUser, 0, "")
	previousOwner := createOrganizationManagementUser(t, db, "default-transfer-owner", common.RoleCommonUser, 0, "")
	newOwner := createOrganizationManagementUser(t, db, "default-transfer-member", common.RoleCommonUser, 0, "")
	systemKey := model.DefaultOrganizationSystemKey
	organization := model.Organization{
		Name: "Default", SystemKey: &systemKey, Status: model.OrganizationStatusActive,
		OwnerUserId: previousOwner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	for userID, role := range map[int]model.OrganizationRole{
		previousOwner.Id: model.OrganizationRoleOwner,
		newOwner.Id:      model.OrganizationRoleMember,
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"organization_id": organization.Id, "organization_role": role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}

	_, err := TransferOrganizationOwnershipForPlatform(actor.Id, TransferOrganizationOwnershipParams{
		OrganizationID: organization.Id, NewOwnerUserID: newOwner.Id,
	})
	assert.ErrorIs(t, err, model.ErrDefaultOrganizationConflict)
}
