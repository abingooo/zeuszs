package middleware

import (
	"net/http"
	"net/http/httptest"
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

func setupOrganizationAuthMiddlewareTest(t *testing.T) (*model.Organization, *model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB, previousRedis := model.DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.User{}, &model.Token{}, &model.UserSession{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
	})

	organization := &model.Organization{
		Name:          "Middleware Organization",
		Status:        model.OrganizationStatusActive,
		OwnerUserId:   3001,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	user := &model.User{
		Username: "organization-middleware-user", Password: "unused-password-hash",
		Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default",
		OrganizationId: organization.Id, OrganizationRole: model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive, AuthVersion: 1,
		AffCode: "organization-middleware-aff",
	}
	user.SetAccessToken("organization-middleware-pat")
	require.NoError(t, db.Create(user).Error)
	return organization, user
}

func TestRequireActiveOrganizationDerivesPATPrincipalFromServerState(t *testing.T) {
	organization, _ := setupOrganizationAuthMiddlewareTest(t)
	router := gin.New()
	router.GET(
		"/organization-test",
		UserAuth(),
		RequireActiveOrganization(),
		RequireOrganizationAction(service.OrganizationActionRead),
		func(c *gin.Context) {
			principal, ok := GetOrganizationPrincipal(c)
			require.True(t, ok)
			c.JSON(http.StatusOK, gin.H{"organization_id": principal.OrganizationID})
		},
	)

	request := httptest.NewRequest(http.MethodGet, "/organization-test", nil)
	request.Header.Set("Authorization", "Bearer organization-middleware-pat")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	require.NoError(t, model.DB.Model(organization).Update("status", model.OrganizationStatusDisabled).Error)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request.Clone(request.Context()))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "AUTH_SESSION_REVOKED")
}

func TestRequireOrganizationActionDoesNotGrantPlatformRoleTenantAccess(t *testing.T) {
	_, user := setupOrganizationAuthMiddlewareTest(t)
	require.NoError(t, model.DB.Model(user).Update("role", common.RoleAdminUser).Error)

	router := gin.New()
	router.GET(
		"/organization-test",
		UserAuth(),
		RequireActiveOrganization(),
		RequireOrganizationAction(service.OrganizationActionMemberRead),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	request := httptest.NewRequest(http.MethodGet, "/organization-test", nil)
	request.Header.Set("Authorization", "Bearer organization-middleware-pat")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestTokenOrganizationValidationRequiresMatchingActiveSnapshot(t *testing.T) {
	organization, user := setupOrganizationAuthMiddlewareTest(t)
	token := &model.Token{
		UserId: user.Id, OrganizationId: organization.Id, Key: "organizationtoken",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	require.NoError(t, validateTokenOrganization(context, user.ToBaseUser(), token))
	principal, ok := GetOrganizationPrincipal(context)
	require.True(t, ok)
	assert.Equal(t, organization.Id, principal.OrganizationID)

	token.OrganizationId = organization.Id + 1
	err := validateTokenOrganization(context, user.ToBaseUser(), token)
	assert.ErrorIs(t, err, service.ErrOrganizationIdentityInvalid)

	token.OrganizationId = organization.Id
	require.NoError(t, model.DB.Model(organization).Update("status", model.OrganizationStatusDisabled).Error)
	err = validateTokenOrganization(context, user.ToBaseUser(), token)
	assert.ErrorIs(t, err, service.ErrOrganizationInactive)
}
