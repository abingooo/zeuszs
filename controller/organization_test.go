package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOrganizationControllerTestDB(t *testing.T) *gorm.DB {
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
		&model.User{}, &model.UserSession{}, &model.Token{},
		&model.Organization{}, &model.OrganizationFundAccount{}, &model.OrganizationMemberFund{}, &model.OrganizationAuditEvent{},
		&model.OrganizationInvite{}, &model.OrganizationInviteUse{},
		&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{},
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

func organizationControllerUser(t *testing.T, db *gorm.DB, username string, role int) model.User {
	t.Helper()
	user := model.User{
		Username: username, Password: "password-hash", Role: role, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1, AffCode: username + "-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func invokeOrganizationController(t *testing.T, method, path, body string, handler gin.HandlerFunc, actorID int) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", actorID)
	context.Set("role", common.RoleAdminUser)
	handler(context)
	return recorder
}

func invokeOrganizationControllerWithParams(t *testing.T, method, path, body string, handler gin.HandlerFunc, actorID int, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = params
	context.Set("id", actorID)
	context.Set("role", common.RoleAdminUser)
	handler(context)
	return recorder
}

func TestCreatePlatformOrganizationControllerReturnsOrganization(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-admin", common.RoleAdminUser)
	owner := organizationControllerUser(t, db, "controller-owner", common.RoleCommonUser)
	recorder := invokeOrganizationController(t, http.MethodPost, "/api/organization/admin/", fmt.Sprintf(`{"name":"Controller Lab","owner_user_id":%d}`, owner.Id), CreatePlatformOrganization, actor.Id)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"name":"Controller Lab"`)
}

func TestCreatePlatformOrganizationControllerProvisionsNewOwnerAccount(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-provision-admin", common.RoleAdminUser)
	previousInitialQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 0
	t.Cleanup(func() { common.QuotaForNewUser = previousInitialQuota })

	recorder := invokeOrganizationController(t, http.MethodPost, "/api/organization/admin/",
		`{"name":"Provisioned Lab","owner_username":"provisioned-owner","owner_password":"password123","owner_display_name":"Provisioned Owner","owner_email":"owner@example.com","allow_member_topup":false}`,
		CreatePlatformOrganization, actor.Id)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"name":"Provisioned Lab"`)
	assert.Contains(t, recorder.Body.String(), `"username":"provisioned-owner"`)
	assert.NotContains(t, recorder.Body.String(), "password123")

	var owner model.User
	require.NoError(t, db.Where("username = ?", "provisioned-owner").First(&owner).Error)
	assert.Positive(t, owner.OrganizationId)
	assert.Equal(t, model.OrganizationRoleOwner, owner.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, owner.OrganizationStatus)
}

func TestCreditPlatformOrganizationFundControllerCreditsPoolOnly(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-fund-admin", common.RoleAdminUser)
	owner := organizationControllerUser(t, db, "controller-fund-owner", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Funded Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)

	recorder := invokeOrganizationControllerWithParams(t, http.MethodPost,
		fmt.Sprintf("/api/organization/admin/%d/fund-credit", organization.Id),
		`{"amount":750,"reference":"controller-receipt"}`,
		CreditPlatformOrganizationFund, actor.Id,
		gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}})
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"pool_quota_after":750`)

	var account model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&account).Error)
	assert.Equal(t, int64(750), account.Quota)
	var persistedOwner model.User
	require.NoError(t, db.First(&persistedOwner, owner.Id).Error)
	assert.Zero(t, persistedOwner.Quota)
}

func TestBuildSelfUserDataIncludesOrganizationIdentity(t *testing.T) {
	user := &model.User{
		Id:                 42,
		Username:           "organization-session-user",
		Role:               common.RoleCommonUser,
		OrganizationId:     91,
		OrganizationRole:   model.OrganizationRoleAdmin,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}

	data := buildSelfUserData(user)
	assert.Equal(t, 91, data["organization_id"])
	assert.Equal(t, model.OrganizationRoleAdmin, data["organization_role"])
	assert.Equal(t, model.OrganizationMemberStatusActive, data["organization_status"])
}

func TestProvisionPlatformOrganizationMemberControllerCreatesAdmin(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-member-provision-admin", common.RoleAdminUser)
	owner := organizationControllerUser(t, db, "controller-member-provision-owner", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Member Provision Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	previousInitialQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 0
	t.Cleanup(func() { common.QuotaForNewUser = previousInitialQuota })

	recorder := invokeOrganizationControllerWithParams(
		t, http.MethodPost, fmt.Sprintf("/api/organization/admin/%d/members", organization.Id),
		`{"username":"created-org-admin","password":"password123","display_name":"Created Admin","organization_role":"admin"}`,
		ProvisionPlatformOrganizationMember, actor.Id,
		gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}},
	)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"organization_role":"admin"`)
	assert.NotContains(t, recorder.Body.String(), "password123")

	var created model.User
	require.NoError(t, db.Where("username = ?", "created-org-admin").First(&created).Error)
	assert.Equal(t, organization.Id, created.OrganizationId)
	assert.Equal(t, model.OrganizationRoleAdmin, created.OrganizationRole)
}

func TestOrganizationManagementErrorCodeIsStableForProvisioningAndFunding(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{service.ErrOrganizationOwnerUsernameRequired, "ORGANIZATION_OWNER_USERNAME_REQUIRED"},
		{service.ErrOrganizationOwnerPasswordInvalid, "ORGANIZATION_OWNER_PASSWORD_INVALID"},
		{service.ErrOrganizationOwnerAccountInvalid, "ORGANIZATION_OWNER_ACCOUNT_INVALID"},
		{service.ErrOrganizationMemberUsernameRequired, "ORGANIZATION_MEMBER_USERNAME_REQUIRED"},
		{service.ErrOrganizationMemberPasswordInvalid, "ORGANIZATION_MEMBER_PASSWORD_INVALID"},
		{service.ErrOrganizationMemberAccountInvalid, "ORGANIZATION_MEMBER_ACCOUNT_INVALID"},
		{service.ErrOrganizationMemberRoleInvalid, "ORGANIZATION_MEMBER_ROLE_INVALID"},
		{model.ErrOrganizationNotActive, "ORGANIZATION_INACTIVE"},
		{model.ErrOrganizationAccountingForbidden, "ORGANIZATION_ACCOUNTING_FORBIDDEN"},
		{model.ErrOrganizationFundOverflow, "ORGANIZATION_FUND_OVERFLOW"},
		{model.ErrOrganizationAccountingIdempotency, "ORGANIZATION_ACCOUNTING_IDEMPOTENCY_CONFLICT"},
		{model.ErrOrganizationAccountingInvalid, "ORGANIZATION_ACCOUNTING_INVALID"},
	}

	for _, test := range tests {
		assert.Equal(t, test.code, service.OrganizationManagementErrorCode(test.err))
	}
}

func TestOrganizationControllerRejectsInvalidRoleAndCrossOrganizationMove(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-admin-2", common.RoleAdminUser)
	owner := organizationControllerUser(t, db, "controller-owner-2", common.RoleCommonUser)
	organization := model.Organization{Name: "Controller Org", Status: model.OrganizationStatusActive, OwnerUserId: owner.Id, PolicyVersion: 1}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	other := organizationControllerUser(t, db, "controller-other", common.RoleCommonUser)
	recorder := invokeOrganizationController(t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/members/%d/role", organization.Id, other.Id), `{"role":"bogus"}`, AssignPlatformOrganizationMemberRole, actor.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	recorder = invokeOrganizationController(t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/members/%d/role", organization.Id, owner.Id), `{"role":"admin"}`, AssignPlatformOrganizationMemberRole, actor.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestTransferPlatformOrganizationOwnershipController(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-transfer-admin", common.RoleAdminUser)
	previousOwner := organizationControllerUser(t, db, "controller-transfer-old", common.RoleCommonUser)
	newOwner := organizationControllerUser(t, db, "controller-transfer-new", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Controller Transfer", Status: model.OrganizationStatusActive,
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

	recorder := invokeOrganizationControllerWithParams(
		t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/ownership", organization.Id),
		fmt.Sprintf(`{"new_owner_user_id":%d}`, newOwner.Id),
		TransferPlatformOrganizationOwnership, actor.Id,
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", organization.Id)}},
	)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), fmt.Sprintf(`"owner_user_id":%d`, newOwner.Id))

	var persistedPrevious, persistedNew model.User
	require.NoError(t, db.First(&persistedPrevious, previousOwner.Id).Error)
	require.NoError(t, db.First(&persistedNew, newOwner.Id).Error)
	assert.Equal(t, model.OrganizationRoleAdmin, persistedPrevious.OrganizationRole)
	assert.Equal(t, model.OrganizationRoleOwner, persistedNew.OrganizationRole)
}

func TestPlatformOrganizationMemberPolicyAndInviteControllers(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	actor := organizationControllerUser(t, db, "controller-operations-admin", common.RoleAdminUser)
	owner := organizationControllerUser(t, db, "controller-operations-owner", common.RoleCommonUser)
	member := organizationControllerUser(t, db, "controller-operations-member", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Controller Operations", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	for userID, role := range map[int]model.OrganizationRole{
		owner.Id:  model.OrganizationRoleOwner,
		member.Id: model.OrganizationRoleMember,
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"organization_id": organization.Id, "organization_role": role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id, UserId: member.Id,
		RecoverableQuota: 25, ConsumedQuota: 3,
	}).Error)

	organizationParam := gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}}
	members := invokeOrganizationControllerWithParams(
		t, http.MethodGet,
		fmt.Sprintf("/api/organization/admin/%d/members?keyword=operations-member", organization.Id),
		"", ListPlatformOrganizationMembers, actor.Id, organizationParam,
	)
	assert.Contains(t, members.Body.String(), `"success":true`)
	assert.Contains(t, members.Body.String(), `"username":"controller-operations-member"`)
	assert.Contains(t, members.Body.String(), `"recoverable_quota":25`)

	policy := invokeOrganizationControllerWithParams(
		t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/topup-policy", organization.Id),
		`{"allow_member_topup":false}`, UpdatePlatformOrganizationTopupPolicy, actor.Id, organizationParam,
	)
	assert.Contains(t, policy.Body.String(), `"success":true`)
	assert.Contains(t, policy.Body.String(), `"allow_member_topup":false`)
	var persistedOrganization model.Organization
	require.NoError(t, db.First(&persistedOrganization, organization.Id).Error)
	assert.False(t, persistedOrganization.AllowMemberTopup)
	assert.EqualValues(t, 2, persistedOrganization.PolicyVersion)

	const inviteCode = "RESEARCH-INVITE-CONTROLLER"
	createInvite := invokeOrganizationControllerWithParams(
		t, http.MethodPost, fmt.Sprintf("/api/organization/admin/%d/invites", organization.Id),
		fmt.Sprintf(`{"code":%q,"max_uses":2}`, inviteCode),
		CreatePlatformOrganizationInvite, actor.Id, organizationParam,
	)
	assert.Contains(t, createInvite.Body.String(), `"success":true`)
	assert.Contains(t, createInvite.Body.String(), `"code":"`+inviteCode+`"`)
	var invite model.OrganizationInvite
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&invite).Error)

	listInvites := invokeOrganizationControllerWithParams(
		t, http.MethodGet, fmt.Sprintf("/api/organization/admin/%d/invites", organization.Id),
		"", ListPlatformOrganizationInvites, actor.Id, organizationParam,
	)
	assert.Contains(t, listInvites.Body.String(), `"success":true`)
	assert.NotContains(t, listInvites.Body.String(), inviteCode)
	assert.NotContains(t, listInvites.Body.String(), invite.CodeHash)

	disableInvite := invokeOrganizationControllerWithParams(
		t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/invites/%d/status", organization.Id, invite.Id),
		"", DisablePlatformOrganizationInvite, actor.Id,
		gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}, {Key: "invite_id", Value: strconv.Itoa(invite.Id)}},
	)
	assert.Contains(t, disableInvite.Body.String(), `"success":true`)
	assert.Contains(t, disableInvite.Body.String(), `"status":"disabled"`)
}

func TestOrganizationOwnerAndAdminCannotInvokePlatformProvisioningControllers(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	owner := organizationControllerUser(t, db, "controller-tenant-owner", common.RoleCommonUser)
	organizationAdmin := organizationControllerUser(t, db, "controller-tenant-admin", common.RoleCommonUser)
	member := organizationControllerUser(t, db, "controller-tenant-member", common.RoleCommonUser)
	newOwner := organizationControllerUser(t, db, "controller-tenant-new-owner", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Tenant Only", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	for userID, role := range map[int]model.OrganizationRole{
		owner.Id:             model.OrganizationRoleOwner,
		organizationAdmin.Id: model.OrganizationRoleAdmin,
		member.Id:            model.OrganizationRoleMember,
		newOwner.Id:          model.OrganizationRoleMember,
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"organization_id": organization.Id, "organization_role": role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}
	params := gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}, {Key: "user_id", Value: strconv.Itoa(member.Id)}}
	candidate := organizationControllerUser(t, db, "controller-unassigned-owner", common.RoleCommonUser)

	for _, actorID := range []int{owner.Id, organizationAdmin.Id} {
		assign := invokeOrganizationControllerWithParams(
			t, http.MethodPatch,
			fmt.Sprintf("/api/organization/admin/%d/members/%d/role", organization.Id, member.Id),
			`{"role":"admin"}`, AssignPlatformOrganizationMemberRole, actorID, params,
		)
		assert.Equal(t, http.StatusForbidden, assign.Code)
		assert.Contains(t, assign.Body.String(), `"code":"PLATFORM_ORGANIZATION_FORBIDDEN"`)

		transfer := invokeOrganizationControllerWithParams(
			t, http.MethodPatch, fmt.Sprintf("/api/organization/admin/%d/ownership", organization.Id),
			fmt.Sprintf(`{"new_owner_user_id":%d}`, newOwner.Id),
			TransferPlatformOrganizationOwnership, actorID,
			gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}},
		)
		assert.Equal(t, http.StatusForbidden, transfer.Code)
		assert.Contains(t, transfer.Body.String(), `"code":"PLATFORM_ORGANIZATION_FORBIDDEN"`)

		create := invokeOrganizationController(
			t, http.MethodPost, "/api/organization/admin/",
			fmt.Sprintf(`{"name":"Forbidden Organization","owner_user_id":%d}`, candidate.Id),
			CreatePlatformOrganization, actorID,
		)
		assert.Equal(t, http.StatusForbidden, create.Code)
		assert.Contains(t, create.Body.String(), `"code":"PLATFORM_ORGANIZATION_FORBIDDEN"`)

		provision := invokeOrganizationControllerWithParams(
			t, http.MethodPost, fmt.Sprintf("/api/organization/admin/%d/members", organization.Id),
			`{"username":"forbidden-new-admin","password":"password123","organization_role":"admin"}`,
			ProvisionPlatformOrganizationMember, actorID,
			gin.Params{{Key: "id", Value: strconv.Itoa(organization.Id)}},
		)
		assert.Equal(t, http.StatusForbidden, provision.Code)
		assert.Contains(t, provision.Body.String(), `"code":"PLATFORM_ORGANIZATION_FORBIDDEN"`)
	}

	var persistedMember, persistedOrganization model.User
	require.NoError(t, db.First(&persistedMember, member.Id).Error)
	require.NoError(t, db.First(&persistedOrganization, newOwner.Id).Error)
	assert.Equal(t, model.OrganizationRoleMember, persistedMember.OrganizationRole)
	assert.Equal(t, model.OrganizationRoleMember, persistedOrganization.OrganizationRole)
	var currentOrganization model.Organization
	require.NoError(t, db.First(&currentOrganization, organization.Id).Error)
	assert.Equal(t, owner.Id, currentOrganization.OwnerUserId)
	var forbiddenProvisionCount int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "forbidden-new-admin").Count(&forbiddenProvisionCount).Error)
	assert.Zero(t, forbiddenProvisionCount)
}
