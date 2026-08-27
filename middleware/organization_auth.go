package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const organizationPrincipalContextKey = "organization_principal"

// GetOrganizationPrincipal returns only a principal resolved from current
// server-side user and organization state.
func GetOrganizationPrincipal(c *gin.Context) (service.OrganizationPrincipal, bool) {
	value, ok := c.Get(organizationPrincipalContextKey)
	if !ok {
		return service.OrganizationPrincipal{}, false
	}
	principal, ok := value.(service.OrganizationPrincipal)
	return principal, ok
}

// RequireActiveOrganization is chained after UserAuth/AdminAuth for tenant
// routes. Platform-only provisioning routes intentionally do not use it.
func RequireActiveOrganization() func(c *gin.Context) {
	return func(c *gin.Context) {
		user, err := model.GetUserCache(c.GetInt("id"))
		if err != nil {
			writeOrganizationAuthError(c, err)
			return
		}
		principal, err := service.ResolveOrganizationPrincipal(user)
		if err != nil {
			writeOrganizationAuthError(c, err)
			return
		}
		setOrganizationPrincipal(c, principal)
		c.Next()
	}
}

// ResolveOrganizationAllowInactive is restricted to an organization's own
// status view and platform recovery flows. It resolves identity but does not
// authorize any tenant action.
func ResolveOrganizationAllowInactive() func(c *gin.Context) {
	return func(c *gin.Context) {
		user, err := model.GetUserCache(c.GetInt("id"))
		if err != nil {
			writeOrganizationAuthError(c, err)
			return
		}
		principal, err := service.ResolveOrganizationPrincipalAllowInactive(user)
		if err != nil {
			writeOrganizationAuthError(c, err)
			return
		}
		setOrganizationPrincipal(c, principal)
		c.Next()
	}
}

func RequireOrganizationAction(action service.OrganizationAction) func(c *gin.Context) {
	return func(c *gin.Context) {
		principal, ok := GetOrganizationPrincipal(c)
		if !ok || !service.CanOrganizationAction(principal, action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "ORGANIZATION_ACTION_FORBIDDEN",
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			return
		}
		c.Next()
	}
}

// OrganizationFundTopUpTarget marks a payment request as an organization
// fund purchase. It is only used after active-organization authentication and
// the explicit fund.topup permission check on tenant routes.
func OrganizationFundTopUpTarget() func(c *gin.Context) {
	return func(c *gin.Context) {
		principal, ok := GetOrganizationPrincipal(c)
		if !ok || !service.CanOrganizationAction(principal, service.OrganizationActionFundTopup) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "ORGANIZATION_ACTION_FORBIDDEN",
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			return
		}
		common.SetContextKey(c, constant.ContextKeyTopUpTarget, model.TopUpTargetOrganization)
		c.Next()
	}
}

// RequireOrganizationWalletTopup re-checks the current organization policy
// immediately before amount estimation or payment-order creation. Only
// ordinary Members are blocked by the policy; Owner/Admin behavior is decided
// by the service from current database state, not request context fields.
func RequireOrganizationWalletTopup() func(c *gin.Context) {
	return func(c *gin.Context) {
		policy, err := service.RequireOrganizationWalletTopup(c.GetInt("id"))
		if err == nil {
			common.SetContextKey(c, constant.ContextKeyOrganizationId, policy.OrganizationID)
			common.SetContextKey(c, constant.ContextKeyOrganizationRole, string(policy.OrganizationRole))
			c.Next()
			return
		}
		if errors.Is(err, service.ErrOrganizationMemberTopupDisabled) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "ORGANIZATION_MEMBER_TOPUP_DISABLED",
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			return
		}
		writeOrganizationAuthError(c, err)
	}
}

func setOrganizationPrincipal(c *gin.Context, principal service.OrganizationPrincipal) {
	c.Set(organizationPrincipalContextKey, principal)
	common.SetContextKey(c, constant.ContextKeyOrganizationId, principal.OrganizationID)
	common.SetContextKey(c, constant.ContextKeyOrganizationRole, string(principal.Role))
}

func validateTokenOrganization(c *gin.Context, user *model.UserBase, token *model.Token) error {
	principal, err := service.ResolveOrganizationPrincipal(user)
	if err != nil {
		return err
	}
	if token == nil || token.OrganizationId <= 0 || token.OrganizationId != principal.OrganizationID {
		return service.ErrOrganizationIdentityInvalid
	}
	setOrganizationPrincipal(c, principal)
	return nil
}

func writeOrganizationAuthError(c *gin.Context, err error) {
	code := "ORGANIZATION_IDENTITY_INVALID"
	switch {
	case errors.Is(err, service.ErrOrganizationInactive):
		code = "ORGANIZATION_INACTIVE"
	case errors.Is(err, service.ErrOrganizationMembershipInactive):
		code = "ORGANIZATION_MEMBERSHIP_INACTIVE"
	case errors.Is(err, service.ErrOrganizationIdentityInvalid):
		code = "ORGANIZATION_IDENTITY_INVALID"
	default:
		common.SysLog("organization authentication error: " + err.Error())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    "ORGANIZATION_AUTH_INTERNAL_ERROR",
			"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
		})
		return
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"code":    code,
		"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
	})
}

func writeTokenOrganizationAuthError(c *gin.Context, err error, openAIResponse bool) {
	status := http.StatusForbidden
	message := common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)
	if errors.Is(err, service.ErrOrganizationIdentityInvalid) {
		status = http.StatusUnauthorized
		message = common.TranslateMessage(c, i18n.MsgTokenInvalid)
	} else if !errors.Is(err, service.ErrOrganizationInactive) &&
		!errors.Is(err, service.ErrOrganizationMembershipInactive) {
		common.SysLog("token organization authentication error: " + err.Error())
		status = http.StatusInternalServerError
		message = common.TranslateMessage(c, i18n.MsgDatabaseError)
	}
	if openAIResponse {
		abortWithOpenAiMessage(c, status, message)
		return
	}
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"message": message,
	})
}
