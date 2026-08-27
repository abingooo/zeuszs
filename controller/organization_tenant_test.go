package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantAllocationControllerIgnoresRequestOrganizationFields(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.OrganizationQuotaOperation{}, &model.OrganizationQuotaLedger{}, &model.OrganizationInvite{},
	))
	owner := organizationControllerUser(t, db, "tenant-controller-owner", common.RoleCommonUser)
	member := organizationControllerUser(t, db, "tenant-controller-member", common.RoleCommonUser)
	organization := model.Organization{
		Name: "Tenant Controller", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id, Quota: 100}).Error)
	for userID, role := range map[int]model.OrganizationRole{owner.Id: model.OrganizationRoleOwner, member.Id: model.OrganizationRoleMember} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"organization_id": organization.Id, "organization_role": role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}

	otherOwner := organizationControllerUser(t, db, "tenant-controller-other-owner", common.RoleCommonUser)
	otherOrganization := model.Organization{
		Name: "Other Tenant", Status: model.OrganizationStatusActive,
		OwnerUserId: otherOwner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&otherOrganization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: otherOrganization.Id, Quota: 1000}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id": otherOrganization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST(
		"/api/organization/members/:user_id/allocate",
		func(c *gin.Context) {
			c.Set("id", owner.Id)
			c.Set("role", common.RoleRootUser)
			c.Next()
		},
		middleware.RequireActiveOrganization(),
		middleware.RequireOrganizationAction(service.OrganizationActionMemberAllocate),
		AllocateTenantOrganizationMemberQuota,
	)
	body := fmt.Sprintf(`{"amount":25,"organization_id":%d,"actor_user_id":%d}`, otherOrganization.Id, otherOwner.Id)
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/organization/members/%d/allocate", member.Id), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(common.RequestIdKey, "tenant-controller-allocation")
	request.Header.Set("Idempotency-Key", "tenant-controller-allocation-idempotency")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"pool_quota_after":75`)
	var account, otherAccount model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&account).Error)
	require.NoError(t, db.Where("organization_id = ?", otherOrganization.Id).First(&otherAccount).Error)
	assert.EqualValues(t, 75, account.Quota)
	assert.EqualValues(t, 1000, otherAccount.Quota)
}
