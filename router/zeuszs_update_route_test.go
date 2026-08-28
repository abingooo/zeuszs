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

func TestZeusZSUpdateRoutesRequirePlatformAdmin(t *testing.T) {
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

	accessToken := "zeuszs-update-tenant-owner-token"
	user := model.User{
		Username:           "zeuszs-update-tenant-owner",
		Password:           "password-placeholder",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		Group:              "default",
		OrganizationId:     51,
		OrganizationRole:   model.OrganizationRoleOwner,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		AccessToken:        &accessToken,
		AuthVersion:        1,
		AffCode:            "zeuszs-update-route-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.Organization{
		Id:               51,
		Name:             "ZeusZS Update Route Organization",
		Status:           model.OrganizationStatusActive,
		OwnerUserId:      user.Id,
		PolicyVersion:    1,
		AllowMemberTopup: true,
	}).Error)

	engine := gin.New()
	SetApiRouter(engine)
	registeredRoutes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		registeredRoutes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		_, registered := registeredRoutes[method+" /api/zeuszs/update"]
		require.True(t, registered)

		request := httptest.NewRequest(method, "/api/zeuszs/update", nil)
		request.Header.Set("Authorization", "Bearer "+accessToken)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Contains(t, response.Body.String(), "AUTH_INSUFFICIENT_PRIVILEGE")
	}
}
