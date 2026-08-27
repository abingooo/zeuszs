package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuthLogoutRejectsRefreshCookieSessionMismatch(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.SessionSecret = "auth-logout-mismatch-test-secret"
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})

	organization := &model.Organization{
		Name: "Logout Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 1001, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	user := &model.User{
		Username: "logout-mismatch-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(user).Error)
	sessionA, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent-a")
	require.NoError(t, err)
	sessionB, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "agent-b")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/auth/logout", nil)
	c.Request.Header.Set("Authorization", "Bearer "+sessionA.AccessToken)
	c.Request.Header.Set("X-Auth-Session", sessionA.Session.SID)
	c.Request.AddCookie(&http.Cookie{Name: service.RefreshCookieName, Value: sessionB.RefreshToken})

	AuthLogout(c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "AUTH_SESSION_MISMATCH", response.Code)
	for _, sid := range []string{sessionA.Session.SID, sessionB.Session.SID} {
		stored, err := model.GetUserSessionBySID(sid)
		require.NoError(t, err)
		assert.Equal(t, model.UserSessionStatusActive, stored.Status)
	}
}

func TestWriteAuthSessionErrorMapsSessionGrowthLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "active session limit",
			err:            model.ErrUserSessionLimit,
			expectedStatus: http.StatusConflict,
			expectedCode:   "AUTH_SESSION_LIMIT",
		},
		{
			name:           "issuance limit",
			err:            model.ErrUserSessionIssuanceLimit,
			expectedStatus: http.StatusTooManyRequests,
			expectedCode:   "AUTH_SESSION_ISSUANCE_LIMIT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			writeAuthSessionError(c, test.err)

			assert.Equal(t, test.expectedStatus, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.expectedCode, response.Code)
		})
	}
}

func TestSessionLimitDoesNotRecordRejectedLoginAsSuccessful(t *testing.T) {
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousActiveLimit := common.UserSessionActiveLimit
	previousIssuanceLimit := common.UserSessionIssuanceLimit
	previousIssuanceWindow := common.UserSessionIssuanceWindowSeconds
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 100
	common.UserSessionIssuanceWindowSeconds = int64(common.DefaultUserSessionIssuanceWindowSeconds)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.UserSessionActiveLimit = previousActiveLimit
		common.UserSessionIssuanceLimit = previousIssuanceLimit
		common.UserSessionIssuanceWindowSeconds = previousIssuanceWindow
	})

	organization := &model.Organization{
		Name: "Rejected Login Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 1001, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	const previousLastLoginAt = int64(123)
	user := &model.User{
		Username: "rejected-login-audit-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, LastLoginAt: previousLastLoginAt,
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: "existing-active-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
	setupLogin(user, c)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, previousLastLoginAt, stored.LastLoginAt)
}

func TestPasswordLoginRejectsInactiveOrganizationState(t *testing.T) {
	tests := []struct {
		name    string
		disable func(t *testing.T, db *gorm.DB, organization *model.Organization, user *model.User)
	}{
		{
			name: "membership disabled",
			disable: func(t *testing.T, db *gorm.DB, _ *model.Organization, user *model.User) {
				t.Helper()
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).
					Update("organization_status", model.OrganizationMemberStatusDisabled).Error)
			},
		},
		{
			name: "organization disabled",
			disable: func(t *testing.T, db *gorm.DB, organization *model.Organization, _ *model.User) {
				t.Helper()
				require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", organization.Id).
					Update("status", model.OrganizationStatusDisabled).Error)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousDB, previousRedis := model.DB, common.RedisEnabled
			previousPasswordLogin := common.PasswordLoginEnabled
			previousSecret := common.SessionSecret
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}, &model.UserSession{}, &model.TwoFA{}))
			model.DB = db
			common.RedisEnabled = false
			common.PasswordLoginEnabled = true
			common.SessionSecret = "password-organization-state-test-secret"
			t.Cleanup(func() {
				model.DB = previousDB
				common.RedisEnabled = previousRedis
				common.PasswordLoginEnabled = previousPasswordLogin
				common.SessionSecret = previousSecret
			})

			organization := &model.Organization{
				Name: "Password Login Organization", Status: model.OrganizationStatusActive,
				OwnerUserId: 1001, PolicyVersion: 1,
			}
			require.NoError(t, db.Create(organization).Error)
			hashedPassword, err := common.Password2Hash("password123")
			require.NoError(t, err)
			user := &model.User{
				Username: "inactive-organization-user", Password: hashedPassword,
				Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
				OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
				OrganizationStatus: model.OrganizationMemberStatusActive, AuthVersion: 1, LastLoginAt: 123,
			}
			require.NoError(t, db.Create(user).Error)
			test.disable(t, db, organization, user)

			router := gin.New()
			router.POST("/api/user/login", Login)
			request := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"username":"inactive-organization-user","password":"password123"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			assert.Equal(t, http.StatusUnauthorized, response.Code)
			var sessionCount int64
			require.NoError(t, db.Model(&model.UserSession{}).Count(&sessionCount).Error)
			assert.Zero(t, sessionCount)
			var stored model.User
			require.NoError(t, db.First(&stored, user.Id).Error)
			assert.Equal(t, int64(123), stored.LastLoginAt)
		})
	}
}

func TestSetupLoginConvergesOnCurrentMembershipState(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
	})

	organization := &model.Organization{
		Name: "Login Convergence Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: 1001, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	user := &model.User{
		Username: "stale-login-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, LastLoginAt: 456,
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(user).Error)
	staleLoginSnapshot := *user
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).
		Update("organization_status", model.OrganizationMemberStatusDisabled).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
	setupLogin(&staleLoginSnapshot, c)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	var sessionCount int64
	require.NoError(t, db.Model(&model.UserSession{}).Count(&sessionCount).Error)
	assert.Zero(t, sessionCount)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, int64(456), stored.LastLoginAt)
}
