package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationAdminRoutesRejectTenantOwnerAndAdminWithoutPlatformRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.RedisEnabled = previousRedisEnabled
	})

	accessTokens := map[model.OrganizationRole]string{
		model.OrganizationRoleOwner: "tenant-owner-platform-user-token",
		model.OrganizationRoleAdmin: "tenant-admin-platform-user-token",
	}
	ownerUserID := 0
	for organizationRole, accessToken := range accessTokens {
		user := model.User{
			Username:           "route-" + string(organizationRole),
			Password:           "password-placeholder",
			Role:               common.RoleCommonUser,
			Status:             common.UserStatusEnabled,
			Group:              "default",
			OrganizationId:     17,
			OrganizationRole:   organizationRole,
			OrganizationStatus: model.OrganizationMemberStatusActive,
			AccessToken:        &accessToken,
			AuthVersion:        1,
			AffCode:            "route-aff-" + string(organizationRole),
		}
		require.NoError(t, db.Create(&user).Error)
		if organizationRole == model.OrganizationRoleOwner {
			ownerUserID = user.Id
		}
	}
	require.Positive(t, ownerUserID)
	require.NoError(t, db.Create(&model.Organization{
		Id:               17,
		Name:             "Route Authorization Organization",
		Status:           model.OrganizationStatusActive,
		OwnerUserId:      ownerUserID,
		PolicyVersion:    1,
		AllowMemberTopup: true,
	}).Error)

	engine := gin.New()
	SetApiRouter(engine)
	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "create organization",
			method: http.MethodPost,
			path:   "/api/organization/admin/",
			body:   `{"name":"Bypass Organization","owner_username":"bypass-owner","owner_password":"password123"}`,
		},
		{
			name:   "assign organization role",
			method: http.MethodPatch,
			path:   "/api/organization/admin/17/members/23/role",
			body:   `{"role":"admin"}`,
		},
		{
			name:   "transfer ownership",
			method: http.MethodPatch,
			path:   "/api/organization/admin/17/ownership",
			body:   `{"new_owner_user_id":23}`,
		},
		{
			name:   "provision organization account",
			method: http.MethodPost,
			path:   "/api/organization/admin/17/members",
			body:   `{"username":"bypass-admin","password":"password123","organization_role":"admin"}`,
		},
		{
			name:   "complete organization topup",
			method: http.MethodPost,
			path:   "/api/organization/admin/topup/complete",
			body:   `{"trade_no":"bypass-order"}`,
		},
	}

	for organizationRole, accessToken := range accessTokens {
		for _, route := range routes {
			t.Run(string(organizationRole)+"/"+route.name, func(t *testing.T) {
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				request.Header.Set("Authorization", "Bearer "+accessToken)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()

				engine.ServeHTTP(response, request)

				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.Contains(t, response.Body.String(), "AUTH_INSUFFICIENT_PRIVILEGE")
			})
		}
	}
}
