package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createTenantOrganizationInviteRequest struct {
	Code      string `json:"code,omitempty"`
	MaxUses   int    `json:"max_uses,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type updateTenantOrganizationTopupPolicyRequest struct {
	AllowMemberTopup *bool `json:"allow_member_topup"`
}

type updateTenantOrganizationMemberStatusRequest struct {
	Status model.OrganizationMemberStatus `json:"status"`
}

type updateTenantOrganizationMemberLimitRequest struct {
	ConsumptionLimit json.RawMessage `json:"consumption_limit"`
}

type transferTenantOrganizationQuotaRequest struct {
	Amount int64 `json:"amount"`
}

func tenantOrganizationPrincipal(c *gin.Context) (service.OrganizationPrincipal, bool) {
	principal, ok := middleware.GetOrganizationPrincipal(c)
	if ok {
		return principal, true
	}
	tenantOrganizationError(c, service.ErrOrganizationIdentityInvalid)
	return service.OrganizationPrincipal{}, false
}

func tenantOrganizationError(c *gin.Context, err error) {
	if err == nil {
		err = service.ErrTenantOrganizationRequestInvalid
	}
	status := http.StatusInternalServerError
	code := "ORGANIZATION_INTERNAL_ERROR"
	message := "organization operation failed"
	switch {
	case errors.Is(err, service.ErrOrganizationActionForbidden),
		errors.Is(err, service.ErrOrganizationIdentityInvalid),
		errors.Is(err, service.ErrOrganizationInactive),
		errors.Is(err, service.ErrOrganizationMembershipInactive),
		errors.Is(err, model.ErrOrganizationAccountingForbidden),
		errors.Is(err, model.ErrOrganizationNotActive),
		errors.Is(err, model.ErrOrganizationMemberNotActive),
		errors.Is(err, model.ErrOrganizationTargetNotMember):
		status = http.StatusForbidden
		code = "ORGANIZATION_ACTION_FORBIDDEN"
		message = err.Error()
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = http.StatusNotFound
		code = "ORGANIZATION_RESOURCE_NOT_FOUND"
		message = "organization resource not found"
	case errors.Is(err, service.ErrOrganizationInviteConflict):
		status = http.StatusConflict
		code = "ORGANIZATION_INVITE_CONFLICT"
		message = err.Error()
	case errors.Is(err, model.ErrOrganizationFundInsufficient),
		errors.Is(err, model.ErrOrganizationFundOverflow),
		errors.Is(err, model.ErrOrganizationUserQuotaInsufficient),
		errors.Is(err, model.ErrOrganizationUserQuotaLimit),
		errors.Is(err, model.ErrOrganizationRecoverableInsufficient),
		errors.Is(err, model.ErrOrganizationConsumptionLimit),
		errors.Is(err, model.ErrOrganizationAccountingIdempotency),
		errors.Is(err, model.ErrOrganizationAccountingPending):
		status = http.StatusConflict
		code = "ORGANIZATION_ACCOUNTING_CONFLICT"
		message = err.Error()
	case errors.Is(err, service.ErrTenantOrganizationRequestInvalid),
		errors.Is(err, service.ErrOrganizationMemberQueryInvalid),
		errors.Is(err, service.ErrOrganizationInviteManagementInvalid),
		errors.Is(err, service.ErrOrganizationInviteMaxUsesInvalid),
		errors.Is(err, service.ErrOrganizationInviteExpiryInvalid),
		errors.Is(err, service.ErrOrganizationPolicyInvalid),
		errors.Is(err, service.ErrOrganizationMemberStatusInvalid),
		errors.Is(err, service.ErrOrganizationMemberLimitInvalid),
		errors.Is(err, model.ErrOrganizationAccountingInvalid):
		status = http.StatusBadRequest
		code = "ORGANIZATION_REQUEST_INVALID"
		message = err.Error()
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": message})
}

func GetTenantOrganizationSummary(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	summary, err := service.GetTenantOrganizationSummary(principal)
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func ListTenantOrganizationMembers(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	var status *model.OrganizationMemberStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		value := model.OrganizationMemberStatus(raw)
		status = &value
	}
	result, err := service.ListTenantOrganizationMembers(principal, service.ListOrganizationMembersParams{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(), Status: status, Keyword: c.Query("keyword"),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

func CreateTenantOrganizationInvite(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	var request createTenantOrganizationInviteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		tenantOrganizationError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	invite, err := service.CreateTenantOrganizationInvite(principal, service.CreateTenantOrganizationInviteParams{
		Code: request.Code, MaxUses: request.MaxUses, ExpiresAt: request.ExpiresAt, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, invite)
}

func ListTenantOrganizationInvites(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	var status *model.OrganizationInviteStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		value := model.OrganizationInviteStatus(raw)
		status = &value
	}
	result, err := service.ListTenantOrganizationInvites(principal, service.ListOrganizationInvitesParams{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(), Status: status,
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

func DisableTenantOrganizationInvite(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	inviteID, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil || inviteID <= 0 {
		tenantOrganizationError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	invite, err := service.DisableTenantOrganizationInvite(principal, service.DisableTenantOrganizationInviteParams{
		InviteID: inviteID, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, invite)
}

func UpdateTenantOrganizationTopupPolicy(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	var request updateTenantOrganizationTopupPolicyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.AllowMemberTopup == nil {
		tenantOrganizationError(c, service.ErrOrganizationPolicyInvalid)
		return
	}
	organization, err := service.UpdateTenantOrganizationTopupPolicy(principal, service.UpdateTenantOrganizationTopupPolicyParams{
		AllowMemberTopup: *request.AllowMemberTopup, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, organization)
}

func UpdateTenantOrganizationMemberStatus(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		tenantOrganizationError(c, service.ErrOrganizationMemberStatusInvalid)
		return
	}
	var request updateTenantOrganizationMemberStatusRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		tenantOrganizationError(c, service.ErrOrganizationMemberStatusInvalid)
		return
	}
	member, err := service.UpdateTenantOrganizationMemberStatus(principal, service.UpdateTenantOrganizationMemberStatusParams{
		UserID: userID, Status: request.Status, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

func UpdateTenantOrganizationMemberLimit(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		tenantOrganizationError(c, service.ErrOrganizationMemberLimitInvalid)
		return
	}
	var request updateTenantOrganizationMemberLimitRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || len(request.ConsumptionLimit) == 0 {
		tenantOrganizationError(c, service.ErrOrganizationMemberLimitInvalid)
		return
	}
	var consumptionLimit *int64
	switch common.GetJsonType(request.ConsumptionLimit) {
	case "null":
	case "number":
		var value int64
		if err := common.Unmarshal(request.ConsumptionLimit, &value); err != nil {
			tenantOrganizationError(c, service.ErrOrganizationMemberLimitInvalid)
			return
		}
		consumptionLimit = &value
	default:
		tenantOrganizationError(c, service.ErrOrganizationMemberLimitInvalid)
		return
	}
	member, err := service.UpdateTenantOrganizationMemberLimit(principal, service.UpdateTenantOrganizationMemberLimitParams{
		UserID: userID, ConsumptionLimit: consumptionLimit, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

func DisableTenantOrganizationMemberTokens(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	disabled, err := service.DisableTenantOrganizationMemberTokens(principal, service.DisableTenantOrganizationMemberTokensParams{
		UserID: userID, RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"disabled_token_count": disabled})
}

func AllocateTenantOrganizationMemberQuota(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	var request transferTenantOrganizationQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Amount <= 0 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	result, err := service.AllocateTenantOrganizationQuota(principal, service.TransferTenantOrganizationQuotaParams{
		UserID: userID, Amount: request.Amount, IdempotencyKey: c.GetHeader("Idempotency-Key"), RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func RecoverTenantOrganizationMemberQuota(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	var request transferTenantOrganizationQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Amount <= 0 {
		tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
		return
	}
	result, err := service.RecoverTenantOrganizationQuota(principal, service.TransferTenantOrganizationQuotaParams{
		UserID: userID, Amount: request.Amount, IdempotencyKey: c.GetHeader("Idempotency-Key"), RequestID: organizationRequestID(c),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ListTenantOrganizationLedger(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
	}
	userID := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			tenantOrganizationError(c, service.ErrTenantOrganizationRequestInvalid)
			return
		}
		userID = parsed
	}
	pageInfo := common.GetPageQuery(c)
	result, err := service.ListTenantOrganizationLedger(principal, service.ListTenantOrganizationLedgerParams{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(), UserID: userID, Operation: c.Query("operation"),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

func ListTenantOrganizationAudit(c *gin.Context) {
	principal, ok := tenantOrganizationPrincipal(c)
	if !ok {
		return
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
	pageInfo := common.GetPageQuery(c)
	result, err := service.ListTenantOrganizationAudit(principal, service.ListTenantOrganizationAuditParams{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(), ActorUserID: actorUserID, Action: c.Query("action"),
	})
	if err != nil {
		tenantOrganizationError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}
