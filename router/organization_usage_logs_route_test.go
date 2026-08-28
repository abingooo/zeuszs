package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationUsageLogsRouteRegistersUserAuthenticatedHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Organization{}, &model.OrganizationAuditEvent{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.RedisEnabled = previousRedisEnabled
	})

	accessToken := "organization-usage-route-admin-token"
	admin := model.User{
		Username:    "organization-usage-route-admin",
		Password:    "password-placeholder",
		Role:        common.RoleAdminUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &accessToken,
		AuthVersion: 1,
		AffCode:     "organization-usage-route-admin-aff",
	}
	require.NoError(t, db.Create(&admin).Error)
	defaultKey := model.DefaultOrganizationSystemKey
	organization := model.Organization{
		Name:             model.DefaultOrganizationName,
		SystemKey:        &defaultKey,
		Status:           model.OrganizationStatusActive,
		OwnerUserId:      admin.Id,
		AllowMemberTopup: true,
		PolicyVersion:    1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", admin.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	engine := gin.New()
	SetApiRouter(engine)
	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, registered := routes[http.MethodGet+" /api/organization/logs"]
	require.True(t, registered)

	request := httptest.NewRequest(http.MethodGet, "/api/organization/logs", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":true`)
}
