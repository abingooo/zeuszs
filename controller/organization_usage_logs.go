package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func ListOrganizationUsageLogs(c *gin.Context) {
	organizationID := 0
	if raw := strings.TrimSpace(c.Query("organization_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
			return
		}
		organizationID = parsed
	}
	actorUserID := 0
	if raw := strings.TrimSpace(c.Query("actor_user_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
			return
		}
		actorUserID = parsed
	}
	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
			return
		}
		startTimestamp = parsed
	}
	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
			return
		}
		endTimestamp = parsed
	}
	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}

	pageInfo := common.GetPageQuery(c)
	if pageInfo.GetPage() < 1 || pageInfo.GetPageSize() < 1 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	result, err := service.ListOrganizationUsageLogs(c.GetInt("id"), service.ListOrganizationUsageLogsParams{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(),
		OrganizationID: organizationID, ActorUserID: actorUserID,
		StartTimestamp: startTimestamp, EndTimestamp: endTimestamp,
		Action: c.Query("action"), TargetType: c.Query("target_type"),
		TargetID: c.Query("target_id"), RequestID: c.Query("request_id"),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}
