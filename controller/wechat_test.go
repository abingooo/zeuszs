package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWeChatRegistrationReadsOrganizationInviteOnlyFromHeader(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousWeChatEnabled := common.WeChatAuthEnabled
	previousRegisterEnabled := common.RegisterEnabled
	previousWeChatAddress := common.WeChatServerAddress
	previousWeChatToken := common.WeChatServerToken
	previousSessionSecret := common.SessionSecret
	previousNewUserQuota := common.QuotaForNewUser
	previousDefaultToken := constant.GenerateDefaultToken

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Organization{},
		&model.OrganizationInvite{},
		&model.OrganizationInviteUse{},
		&model.OrganizationFundAccount{},
		&model.OrganizationMemberFund{},
		&model.OrganizationQuotaLedger{},
		&model.OrganizationQuotaOperation{},
		&model.OrganizationWalletReservation{},
		&model.OrganizationAuditEvent{},
		&model.User{},
		&model.Token{},
		&model.UserSession{},
		&model.ExternalIdentityClaim{},
		&model.Log{},
	))
	defaultKey := model.DefaultOrganizationSystemKey
	defaultOrganization := model.Organization{
		Name: "Default", SystemKey: &defaultKey, Status: model.OrganizationStatusActive,
		OwnerUserId: 1001, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&defaultOrganization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: defaultOrganization.Id}).Error)
	invitedOrganization := model.Organization{
		Name: "Invited", Status: model.OrganizationStatusActive,
		OwnerUserId: 1002, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&invitedOrganization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: invitedOrganization.Id}).Error)
	const organizationInviteCode = "WECHAT-ORG-CODE"
	organizationInvite := model.OrganizationInvite{
		OrganizationId: invitedOrganization.Id,
		CodeHash:       service.HashOrganizationInviteCode(organizationInviteCode),
		CodePrefix:     "WECHAT",
		Status:         model.OrganizationInviteStatusActive,
		MaxUses:        1,
		DefaultRole:    model.OrganizationRoleMember,
		CreatedBy:      1002,
	}
	require.NoError(t, db.Create(&organizationInvite).Error)

	wechatServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		wechatID := "wechat-" + request.URL.Query().Get("code")
		payload, _ := common.Marshal(wechatLoginResponse{Success: true, Data: wechatID})
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(payload)
	}))
	t.Cleanup(wechatServer.Close)

	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.WeChatAuthEnabled = true
	common.RegisterEnabled = true
	common.WeChatServerAddress = wechatServer.URL
	common.WeChatServerToken = "wechat-registration-test-token"
	common.SessionSecret = "wechat-registration-session-secret"
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedisEnabled
		common.WeChatAuthEnabled = previousWeChatEnabled
		common.RegisterEnabled = previousRegisterEnabled
		common.WeChatServerAddress = previousWeChatAddress
		common.WeChatServerToken = previousWeChatToken
		common.SessionSecret = previousSessionSecret
		common.QuotaForNewUser = previousNewUserQuota
		constant.GenerateDefaultToken = previousDefaultToken
	})

	router := gin.New()
	router.GET("/api/oauth/wechat", WeChatAuth)
	queryOnlyRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/oauth/wechat?code=query-only&organization_invite_code="+organizationInviteCode,
		nil,
	)
	queryOnlyResponse := httptest.NewRecorder()
	router.ServeHTTP(queryOnlyResponse, queryOnlyRequest)
	require.Equal(t, http.StatusOK, queryOnlyResponse.Code)
	assert.Contains(t, queryOnlyResponse.Body.String(), `"success":true`)

	var queryOnlyUser model.User
	require.NoError(t, db.Where("wechat_id = ?", "wechat-query-only").First(&queryOnlyUser).Error)
	assert.Equal(t, defaultOrganization.Id, queryOnlyUser.OrganizationId)
	require.NoError(t, db.First(&organizationInvite, organizationInvite.Id).Error)
	assert.Zero(t, organizationInvite.UsedCount)

	headerRequest := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code=header", nil)
	headerRequest.Header.Set(wechatOrganizationInviteHeader, organizationInviteCode)
	headerResponse := httptest.NewRecorder()
	router.ServeHTTP(headerResponse, headerRequest)
	require.Equal(t, http.StatusOK, headerResponse.Code)
	assert.Contains(t, headerResponse.Body.String(), `"success":true`)

	var invitedUser model.User
	require.NoError(t, db.Where("wechat_id = ?", "wechat-header").First(&invitedUser).Error)
	assert.Equal(t, invitedOrganization.Id, invitedUser.OrganizationId)
	assert.Equal(t, model.OrganizationRoleMember, invitedUser.OrganizationRole)
	var inviteUse model.OrganizationInviteUse
	require.NoError(t, db.Where("user_id = ?", invitedUser.Id).First(&inviteUse).Error)
	assert.Equal(t, organizationInvite.Id, inviteUse.OrganizationInviteId)
	require.NoError(t, db.First(&organizationInvite, organizationInvite.Id).Error)
	assert.Equal(t, 1, organizationInvite.UsedCount)
}
