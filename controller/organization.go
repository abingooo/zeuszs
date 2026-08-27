package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createPlatformOrganizationRequest struct {
	Name             string `json:"name"`
	OwnerUserID      int    `json:"owner_user_id,omitempty"`
	OwnerID          int    `json:"owner_id,omitempty"`
	OwnerUsername    string `json:"owner_username,omitempty"`
	OwnerPassword    string `json:"owner_password,omitempty"`
	OwnerDisplayName string `json:"owner_display_name,omitempty"`
	OwnerEmail       string `json:"owner_email,omitempty"`
	AllowMemberTopup *bool  `json:"allow_member_topup,omitempty"`
}

type updatePlatformOrganizationStatusRequest struct {
	Status model.OrganizationStatus `json:"status"`
}

type assignPlatformOrganizationRoleRequest struct {
	Role             model.OrganizationRole `json:"role"`
	OrganizationRole model.OrganizationRole `json:"organization_role,omitempty"`
}

type transferPlatformOrganizationOwnershipRequest struct {
	NewOwnerUserID int `json:"new_owner_user_id"`
	OwnerUserID    int `json:"owner_user_id,omitempty"`
}

type createPlatformOrganizationInviteRequest struct {
	Code      string `json:"code,omitempty"`
	MaxUses   int    `json:"max_uses,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type updatePlatformOrganizationTopupPolicyRequest struct {
	AllowMemberTopup *bool `json:"allow_member_topup"`
}

type creditPlatformOrganizationFundRequest struct {
	Amount    int64  `json:"amount"`
	Reference string `json:"reference,omitempty"`
}

type provisionPlatformOrganizationMemberRequest struct {
	Username         string                 `json:"username"`
	Password         string                 `json:"password"`
	DisplayName      string                 `json:"display_name,omitempty"`
	Email            string                 `json:"email,omitempty"`
	OrganizationRole model.OrganizationRole `json:"organization_role"`
}

func organizationRequestID(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader(common.RequestIdKey))
}

func organizationManagementError(c *gin.Context, err error) {
	if err == nil {
		err = service.ErrOrganizationManagementInvalid
	}
	status := http.StatusOK
	if errors.Is(err, service.ErrPlatformProvisioningForbidden) {
		status = http.StatusForbidden
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{
		"success": false,
		"code":    service.OrganizationManagementErrorCode(err),
		"message": err.Error(),
	})
}

// ListPlatformOrganizations lists all organizations for a platform admin. It
// is intentionally separate from the tenant member view, since organization
// roles do not grant platform-wide visibility.
func ListPlatformOrganizations(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var status *model.OrganizationStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		parsed := model.OrganizationStatus(raw)
		status = &parsed
	}
	result, err := service.ListOrganizationsForPlatform(c.GetInt("id"), service.ListOrganizationsParams{
		Offset: pageInfo.GetStartIdx(),
		Limit:  pageInfo.GetPageSize(),
		Status: status,
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

// CreatePlatformOrganization normally creates a new common account together
// with its organization. The owner_user_id branch remains for legacy callers
// that hold an intentionally unassigned pre-migration account.
func CreatePlatformOrganization(c *gin.Context) {
	var request createPlatformOrganizationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	ownerID := request.OwnerUserID
	if ownerID == 0 {
		ownerID = request.OwnerID
	}
	if ownerID == 0 {
		result, err := service.CreateOrganizationWithOwnerForPlatform(c.GetInt("id"), service.CreateOrganizationWithOwnerParams{
			Name:             request.Name,
			OwnerUsername:    request.OwnerUsername,
			OwnerPassword:    request.OwnerPassword,
			OwnerDisplayName: request.OwnerDisplayName,
			OwnerEmail:       request.OwnerEmail,
			AllowMemberTopup: request.AllowMemberTopup,
			RequestID:        organizationRequestID(c),
		})
		if err != nil {
			organizationManagementError(c, err)
			return
		}
		common.ApiSuccess(c, result)
		return
	}
	organization, err := service.CreateOrganizationForPlatform(c.GetInt("id"), service.CreateOrganizationParams{
		Name:             request.Name,
		OwnerUserID:      ownerID,
		AllowMemberTopup: request.AllowMemberTopup,
		RequestID:        organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, organization)
}

// CreditPlatformOrganizationFund records an external receipt or manual
// adjustment in the organization budget pool. It never changes user wallets.
func CreditPlatformOrganizationFund(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	var request creditPlatformOrganizationFundRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Amount <= 0 {
		organizationManagementError(c, model.ErrOrganizationAccountingInvalid)
		return
	}
	result, err := service.CreditOrganizationFundForPlatform(c.GetInt("id"), service.CreditOrganizationFundForPlatformParams{
		OrganizationID: organizationID,
		Amount:         request.Amount,
		Reference:      request.Reference,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"ledger_id":               result.LedgerId,
		"pool_quota_after":        result.PoolQuotaAfter,
		"already_applied":         result.AlreadyApplied,
		"user_quota_after":        result.UserQuotaAfter,
		"recoverable_quota_after": result.RecoverableQuotaAfter,
	})
}

// ProvisionPlatformOrganizationMember creates a new tenant account directly
// as Admin or Member. The platform-only route and transactional service check
// jointly prevent organization roles from using this as a promotion path.
func ProvisionPlatformOrganizationMember(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	var request provisionPlatformOrganizationMemberRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	member, err := service.ProvisionOrganizationMemberForPlatform(c.GetInt("id"), service.ProvisionOrganizationMemberParams{
		OrganizationID: organizationID,
		Username:       request.Username,
		Password:       request.Password,
		DisplayName:    request.DisplayName,
		Email:          request.Email,
		Role:           request.OrganizationRole,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

// UpdatePlatformOrganizationStatus changes an organization's lifecycle state
// and invalidates all member sessions/API-key snapshots transactionally.
func UpdatePlatformOrganizationStatus(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	var request updatePlatformOrganizationStatusRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	organization, err := service.UpdateOrganizationStatusForPlatform(c.GetInt("id"), service.UpdateOrganizationStatusParams{
		OrganizationID: organizationID,
		Status:         request.Status,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, organization)
}

// AssignPlatformOrganizationMemberRole assigns a single user to a tenant or
// changes their role. Cross-organization reassignment is intentionally
// rejected to preserve the one-organization-per-user invariant.
func AssignPlatformOrganizationMemberRole(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	var request assignPlatformOrganizationRoleRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	role := request.Role
	if strings.TrimSpace(string(role)) == "" {
		role = request.OrganizationRole
	}
	user, err := service.AssignOrganizationMemberRoleForPlatform(c.GetInt("id"), service.AssignOrganizationRoleParams{
		OrganizationID: organizationID,
		UserID:         userID,
		Role:           role,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, user)
}

// TransferPlatformOrganizationOwnership is the only ownership replacement
// endpoint. Platform Admin/Root authorization is re-read inside the service
// transaction; organization roles in the request context grant no authority.
func TransferPlatformOrganizationOwnership(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	var request transferPlatformOrganizationOwnershipRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationManagementInvalid)
		return
	}
	newOwnerUserID := request.NewOwnerUserID
	if newOwnerUserID == 0 {
		newOwnerUserID = request.OwnerUserID
	}
	organization, err := service.TransferOrganizationOwnershipForPlatform(c.GetInt("id"), service.TransferOrganizationOwnershipParams{
		OrganizationID: organizationID,
		NewOwnerUserID: newOwnerUserID,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, organization)
}

// ListPlatformOrganizationMembers lists a safe member projection for one
// organization. It remains behind the platform-admin route while tenant-level
// member authorization is being phased in separately.
func ListPlatformOrganizationMembers(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationMemberQueryInvalid)
		return
	}
	pageInfo := common.GetPageQuery(c)
	var status *model.OrganizationMemberStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		parsed := model.OrganizationMemberStatus(raw)
		status = &parsed
	}
	result, err := service.ListOrganizationMembersForPlatform(c.GetInt("id"), organizationID, service.ListOrganizationMembersParams{
		Offset:  pageInfo.GetStartIdx(),
		Limit:   pageInfo.GetPageSize(),
		Status:  status,
		Keyword: c.Query("keyword"),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

// UpdatePlatformOrganizationTopupPolicy toggles whether members may perform
// self-funded top-ups. The service updates policy version and auth fences in
// one transaction, so stale sessions cannot retain the old policy.
func UpdatePlatformOrganizationTopupPolicy(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationPolicyInvalid)
		return
	}
	var request updatePlatformOrganizationTopupPolicyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.AllowMemberTopup == nil {
		organizationManagementError(c, service.ErrOrganizationPolicyInvalid)
		return
	}
	organization, err := service.UpdateOrganizationTopupPolicyForPlatform(c.GetInt("id"), service.UpdateOrganizationTopupPolicyParams{
		OrganizationID:   organizationID,
		AllowMemberTopup: *request.AllowMemberTopup,
		RequestID:        organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, organization)
}

// CreatePlatformOrganizationInvite creates a member-only invite and returns
// the plaintext code exactly once. Subsequent list responses contain only the
// prefix and usage metadata.
func CreatePlatformOrganizationInvite(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	var request createPlatformOrganizationInviteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		organizationManagementError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	invite, err := service.CreateOrganizationInviteForPlatform(c.GetInt("id"), service.CreateOrganizationInviteParams{
		OrganizationID: organizationID,
		Code:           request.Code,
		MaxUses:        request.MaxUses,
		ExpiresAt:      request.ExpiresAt,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, invite)
}

// ListPlatformOrganizationInvites lists invite metadata and never exposes the
// code hash or plaintext code.
func ListPlatformOrganizationInvites(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	pageInfo := common.GetPageQuery(c)
	var status *model.OrganizationInviteStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		parsed := model.OrganizationInviteStatus(raw)
		status = &parsed
	}
	result, err := service.ListOrganizationInvitesForPlatform(c.GetInt("id"), organizationID, service.ListOrganizationInvitesParams{
		Offset: pageInfo.GetStartIdx(),
		Limit:  pageInfo.GetPageSize(),
		Status: status,
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	pageInfo.SetTotal(int(result.Total))
	pageInfo.SetItems(result.Items)
	common.ApiSuccess(c, pageInfo)
}

// DisablePlatformOrganizationInvite revokes an invite while retaining its
// usage history for audit and compliance.
func DisablePlatformOrganizationInvite(c *gin.Context) {
	organizationID, err := strconv.Atoi(c.Param("id"))
	if err != nil || organizationID <= 0 {
		organizationManagementError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	inviteID, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil || inviteID <= 0 {
		organizationManagementError(c, service.ErrOrganizationInviteManagementInvalid)
		return
	}
	invite, err := service.DisableOrganizationInviteForPlatform(c.GetInt("id"), service.DisableOrganizationInviteParams{
		OrganizationID: organizationID,
		InviteID:       inviteID,
		RequestID:      organizationRequestID(c),
	})
	if err != nil {
		organizationManagementError(c, err)
		return
	}
	common.ApiSuccess(c, invite)
}

// Compatibility aliases keep controller naming consistent with existing
// Admin* handlers while the canonical names above remain descriptive.
func AdminListOrganizations(c *gin.Context) { ListPlatformOrganizations(c) }

func AdminCreateOrganization(c *gin.Context) { CreatePlatformOrganization(c) }

func AdminUpdateOrganizationStatus(c *gin.Context) { UpdatePlatformOrganizationStatus(c) }

func AdminAssignOrganizationMemberRole(c *gin.Context) {
	AssignPlatformOrganizationMemberRole(c)
}

func AdminTransferOrganizationOwnership(c *gin.Context) {
	TransferPlatformOrganizationOwnership(c)
}

func AdminListOrganizationMembers(c *gin.Context) { ListPlatformOrganizationMembers(c) }

func AdminUpdateOrganizationTopupPolicy(c *gin.Context) {
	UpdatePlatformOrganizationTopupPolicy(c)
}

func AdminCreateOrganizationInvite(c *gin.Context) { CreatePlatformOrganizationInvite(c) }

func AdminListOrganizationInvites(c *gin.Context) { ListPlatformOrganizationInvites(c) }

func AdminDisableOrganizationInvite(c *gin.Context) { DisablePlatformOrganizationInvite(c) }
