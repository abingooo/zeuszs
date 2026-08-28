package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type organizationUsageLogsControllerResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    struct {
		Page     int                                `json:"page"`
		PageSize int                                `json:"page_size"`
		Total    int                                `json:"total"`
		Items    []service.OrganizationUsageLogView `json:"items"`
	} `json:"data"`
}

func createOrganizationUsageLogsControllerFixture(t *testing.T) (*model.Organization, model.User, model.User) {
	t.Helper()
	db := model.DB
	owner := organizationControllerUser(t, db, "usage-controller-owner", common.RoleCommonUser)
	organization := model.Organization{
		Name:             "Controller Usage Organization",
		Status:           model.OrganizationStatusActive,
		OwnerUserId:      owner.Id,
		AllowMemberTopup: true,
		PolicyVersion:    1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	owner.OrganizationId = organization.Id
	owner.OrganizationRole = model.OrganizationRoleOwner
	owner.OrganizationStatus = model.OrganizationMemberStatusActive
	member := organizationControllerUser(t, db, "usage-controller-member", common.RoleCommonUser)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
		"organization_id":     organization.Id,
		"organization_role":   model.OrganizationRoleMember,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	member.OrganizationId = organization.Id
	member.OrganizationRole = model.OrganizationRoleMember
	member.OrganizationStatus = model.OrganizationMemberStatusActive
	return &organization, owner, member
}

func TestListOrganizationUsageLogsControllerReturnsPaginatedEvents(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	organization, owner, member := createOrganizationUsageLogsControllerFixture(t)
	for index := 0; index < 2; index++ {
		require.NoError(t, db.Create(&model.OrganizationAuditEvent{
			OrganizationId: organization.Id,
			ActorUserId:    owner.Id,
			Action:         "organization.quota.allocate",
			TargetType:     "user",
			TargetId:       strconv.Itoa(member.Id),
			RequestId:      fmt.Sprintf("controller-usage-%d", index),
			Metadata:       `{"user_quota_delta":10}`,
			CreatedAt:      int64(100 + index),
		}).Error)
	}

	recorder := invokeOrganizationController(
		t,
		http.MethodGet,
		"/api/organization/logs?p=2&page_size=1&action=organization.quota.allocate&start_timestamp=100&end_timestamp=101",
		"",
		ListOrganizationUsageLogs,
		owner.Id,
	)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response organizationUsageLogsControllerResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, 2, response.Data.Page)
	assert.Equal(t, 1, response.Data.PageSize)
	assert.Equal(t, 2, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, "controller-usage-0", response.Data.Items[0].RequestID)
}

func TestListOrganizationUsageLogsControllerRejectsInvalidFilters(t *testing.T) {
	setupOrganizationControllerTestDB(t)
	_, owner, _ := createOrganizationUsageLogsControllerFixture(t)
	testCases := []string{
		"p=-1",
		"page_size=-1",
		"organization_id=0",
		"organization_id=-1",
		"organization_id=invalid",
		"actor_user_id=0",
		"actor_user_id=-1",
		"start_timestamp=-1",
		"end_timestamp=-1",
		"start_timestamp=20&end_timestamp=10",
	}
	for _, query := range testCases {
		t.Run(query, func(t *testing.T) {
			recorder := invokeOrganizationController(
				t,
				http.MethodGet,
				"/api/organization/logs?"+query,
				"",
				ListOrganizationUsageLogs,
				owner.Id,
			)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "ORGANIZATION_REQUEST_INVALID")
		})
	}
}

func TestListOrganizationUsageLogsControllerRejectsCrossOrganizationFilter(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	_, owner, _ := createOrganizationUsageLogsControllerFixture(t)
	otherOwner := organizationControllerUser(t, db, "usage-controller-other-owner", common.RoleCommonUser)
	otherOrganization := model.Organization{
		Name:          "Controller Other Organization",
		Status:        model.OrganizationStatusActive,
		OwnerUserId:   otherOwner.Id,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&otherOrganization).Error)

	recorder := invokeOrganizationController(
		t,
		http.MethodGet,
		fmt.Sprintf("/api/organization/logs?organization_id=%d", otherOrganization.Id),
		"",
		ListOrganizationUsageLogs,
		owner.Id,
	)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "ORGANIZATION_RESOURCE_NOT_FOUND")
}

func TestListOrganizationUsageLogsControllerRejectsDefaultOrganizationMember(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	organization, _, member := createOrganizationUsageLogsControllerFixture(t)
	defaultKey := model.DefaultOrganizationSystemKey
	require.NoError(t, db.Model(&model.Organization{}).
		Where("id = ?", organization.Id).
		Update("system_key", defaultKey).Error)

	recorder := invokeOrganizationController(
		t,
		http.MethodGet,
		"/api/organization/logs",
		"",
		ListOrganizationUsageLogs,
		member.Id,
	)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "ORGANIZATION_ACTION_FORBIDDEN")
}
