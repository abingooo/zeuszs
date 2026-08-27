package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManageUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{},
		&model.Organization{}, &model.OrganizationFundAccount{}, &model.OrganizationMemberFund{},
		&model.OrganizationWalletReservation{}, &model.OrganizationQuotaOperation{},
		&model.OrganizationQuotaLedger{}, &model.OrganizationAuditEvent{},
	))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func performManageUserRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	ManageUser(c)
	return recorder
}

func performDeleteUserRequest(t *testing.T, userID int) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/user/%d", userID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	DeleteUser(c)
	return recorder
}

func TestUpdateUserIgnoresForgedOrganizationIdentity(t *testing.T) {
	db := setupManageUserTestDB(t)
	organization := model.Organization{
		Name: "Current Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 7001, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	otherOrganization := model.Organization{
		Name: "Forged Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 7002, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&otherOrganization).Error)
	platformAdmin := model.User{
		Id: 9999, Username: "platform-admin", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "update-platform-admin-aff",
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(&platformAdmin).Error)
	member := model.User{
		Username: "org-member-before", Password: "password", DisplayName: "Before",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		AffCode:        "update-organization-member-aff",
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive, AuthVersion: 1,
	}
	require.NoError(t, db.Create(&member).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := fmt.Sprintf(
		`{"id":%d,"username":"org-member-updated","display_name":"After","role":%d,"group":"default","organization_id":%d,"organization_role":"admin","organization_status":"disabled"}`,
		member.Id, common.RoleCommonUser, otherOrganization.Id,
	)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleAdminUser)
	c.Set("username", "platform-admin")
	UpdateUser(c)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var persisted model.User
	require.NoError(t, db.First(&persisted, member.Id).Error)
	assert.Equal(t, "org-member-updated", persisted.Username)
	assert.Equal(t, "After", persisted.DisplayName)
	assert.Equal(t, organization.Id, persisted.OrganizationId)
	assert.Equal(t, model.OrganizationRoleMember, persisted.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, persisted.OrganizationStatus)
}

func TestCreateUserCannotProvisionOrganizationAdminThroughGenericUserAPI(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 0
	t.Cleanup(func() { common.QuotaForNewUser = previousQuota })

	defaultKey := model.DefaultOrganizationSystemKey
	defaultOrganization := model.Organization{
		Name: "Default Organization", SystemKey: &defaultKey, Status: model.OrganizationStatusActive,
		OwnerUserId: 8001, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&defaultOrganization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: defaultOrganization.Id}).Error)
	platformAdmin := model.User{
		Id: 9999, Username: "platform-admin", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "create-platform-admin-aff",
		OrganizationId: defaultOrganization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(&platformAdmin).Error)
	otherOrganization := model.Organization{
		Name: "Forged Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 8002, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&otherOrganization).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := fmt.Sprintf(
		`{"username":"generic-created-user","password":"password123","role":%d,"organization_id":%d,"organization_role":"admin","organization_status":"disabled"}`,
		common.RoleCommonUser, otherOrganization.Id,
	)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleAdminUser)
	c.Set("username", "platform-admin")
	CreateUser(c)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var persisted model.User
	require.NoError(t, db.Where("username = ?", "generic-created-user").First(&persisted).Error)
	assert.Equal(t, defaultOrganization.Id, persisted.OrganizationId)
	assert.Equal(t, model.OrganizationRoleMember, persisted.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, persisted.OrganizationStatus)
}

func TestManageUserDisableAdvancesAuthVersionOnceAndRevokesSession(t *testing.T) {
	db := setupManageUserTestDB(t)
	now := time.Now().Unix()
	user := model.User{
		Username: "managed-disable-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-disable-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"disable"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, updated.Status)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "managed-disable-session").Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
}

func TestManageUserDemoteAdvancesAuthVersionAndRevokesSessionsOnce(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, authz.Init(db))

	now := time.Now().Unix()
	user := model.User{
		Username: "managed-demote-user", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	for _, sid := range []string{"managed-demote-session-one", "managed-demote-session-two"} {
		require.NoError(t, db.Create(&model.UserSession{
			SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + sid, LoginMethod: "password",
			LastActiveAt: now, ExpiresAt: now + 3600,
		}).Error)
	}

	sessionUpdateCount := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:count_demote_session_updates", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" {
			sessionUpdateCount++
		}
	}))

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"demote"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, updated.Role)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var sessions []model.UserSession
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("sid asc").Find(&sessions).Error)
	require.Len(t, sessions, 2)
	for _, session := range sessions {
		assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
		assert.Equal(t, "admin_demote", session.RevokedReason)
	}
	assert.Equal(t, 1, sessionUpdateCount)
}

func TestManageUserDeleteReturnsImmediatelyAndUnknownActionFails(t *testing.T) {
	db := setupManageUserTestDB(t)
	deleted := model.User{
		Username: "managed-delete-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "delete-aff",
	}
	require.NoError(t, db.Create(&deleted).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"delete"}`, deleted.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var deletedCount int64
	require.NoError(t, db.Unscoped().Model(&model.User{}).Where("id = ? AND deleted_at IS NOT NULL", deleted.Id).Count(&deletedCount).Error)
	assert.EqualValues(t, 1, deletedCount)

	unchanged := model.User{
		Username: "managed-unknown-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "unknown-aff",
	}
	require.NoError(t, db.Create(&unchanged).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"unknown"}`, unchanged.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	require.NoError(t, db.First(&unchanged, unchanged.Id).Error)
	assert.EqualValues(t, 1, unchanged.AuthVersion)
	assert.Equal(t, common.UserStatusEnabled, unchanged.Status)
}

func TestManageUserRejectsOrganizationOwnerDisableAndDelete(t *testing.T) {
	db := setupManageUserTestDB(t)
	owner := model.User{
		Username: "managed-owner", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "managed-owner-aff",
	}
	require.NoError(t, db.Create(&owner).Error)
	organization := model.Organization{
		Name: "Managed Owner Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&owner).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	disable := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"disable"}`, owner.Id))
	assert.Contains(t, disable.Body.String(), `"success":false`)
	assert.Contains(t, disable.Body.String(), model.ErrOrganizationOwnerDisableForbidden.Error())
	softDelete := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"delete"}`, owner.Id))
	assert.Contains(t, softDelete.Body.String(), `"success":false`)
	assert.Contains(t, softDelete.Body.String(), model.ErrOrganizationOwnerDeletionForbidden.Error())
	hardDelete := performDeleteUserRequest(t, owner.Id)
	assert.Contains(t, hardDelete.Body.String(), `"success":false`)
	assert.Contains(t, hardDelete.Body.String(), model.ErrOrganizationOwnerDeletionForbidden.Error())

	var persisted model.User
	require.NoError(t, db.First(&persisted, owner.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, persisted.Status)
	assert.False(t, persisted.DeletedAt.Valid)
	assert.EqualValues(t, 1, persisted.AuthVersion)
}

func TestManageUserPlatformDemotionPreservesOrganizationOwnerRole(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, authz.Init(db))

	owner := model.User{
		Username: "demoted-platform-owner", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "demoted-platform-owner-aff",
	}
	require.NoError(t, db.Create(&owner).Error)
	organization := model.Organization{
		Name: "Demotion Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&owner).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"demote"}`, owner.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var persisted model.User
	require.NoError(t, db.First(&persisted, owner.Id).Error)
	assert.Equal(t, common.RoleCommonUser, persisted.Role)
	assert.Equal(t, model.OrganizationRoleOwner, persisted.OrganizationRole)
	assert.Equal(t, organization.Id, persisted.OrganizationId)
	assert.EqualValues(t, 2, persisted.AuthVersion)
}

func TestManageUserOrganizationQuotaActionsUseLedgerAndStayNonnegative(t *testing.T) {
	db := setupManageUserTestDB(t)
	root := model.User{
		Id: 9999, Username: "root-operator", Password: "password", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "root-operator-aff",
	}
	require.NoError(t, db.Create(&root).Error)
	member := model.User{
		Username: "managed-quota-member", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 50, AffCode: "managed-quota-member-aff",
		OrganizationRole: model.OrganizationRoleMember, OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(&member).Error)
	organization := model.Organization{
		Name: "Managed Quota Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: root.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&member).Update("organization_id", organization.Id).Error)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id, UserId: member.Id, RecoverableQuota: 20,
	}).Error)

	add := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":10}`, member.Id))
	assert.Contains(t, add.Body.String(), `"success":true`)
	overdraw := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":100}`, member.Id))
	assert.Contains(t, overdraw.Body.String(), `"success":false`)
	subtract := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":30}`, member.Id))
	assert.Contains(t, subtract.Body.String(), `"success":true`)
	override := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"override","value":0}`, member.Id))
	assert.Contains(t, override.Body.String(), `"success":true`)

	var persisted model.User
	require.NoError(t, db.First(&persisted, member.Id).Error)
	assert.Zero(t, persisted.Quota)
	var memberFund model.OrganizationMemberFund
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, member.Id).First(&memberFund).Error)
	assert.Zero(t, memberFund.RecoverableQuota)
	assert.EqualValues(t, 60, memberFund.ConsumedQuota)
	var ledgers []model.OrganizationQuotaLedger
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, member.Id).Order("id asc").Find(&ledgers).Error)
	require.Len(t, ledgers, 3)
	assert.Equal(t, model.OrganizationLedgerWalletCredit, ledgers[0].Operation)
	assert.Equal(t, model.OrganizationLedgerWalletDebit, ledgers[1].Operation)
	assert.Equal(t, model.OrganizationLedgerWalletDebit, ledgers[2].Operation)
}
