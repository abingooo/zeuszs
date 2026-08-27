package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const oauthAuthFlowTTL = 10 * time.Minute

type oauthStateRequest struct {
	Provider               string `json:"provider"`
	Intent                 string `json:"intent"`
	Aff                    string `json:"aff,omitempty"`
	OrganizationInviteCode string `json:"organization_invite_code,omitempty"`
}

type oauthFlowPayload struct {
	AffiliateCode          string `json:"affiliate_code,omitempty"`
	OrganizationInviteCode string `json:"organization_invite_code,omitempty"`
}

// providerParams returns map with Provider key for i18n templates
func providerParams(name string) map[string]any {
	return map[string]any{"Provider": name}
}

// GenerateOAuthCode generates a state code for OAuth CSRF protection
func GenerateOAuthCode(c *gin.Context) {
	var request oauthStateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.Intent = strings.TrimSpace(request.Intent)
	request.Aff = strings.TrimSpace(request.Aff)
	request.OrganizationInviteCode = service.NormalizeOrganizationInviteCode(request.OrganizationInviteCode)
	if oauth.GetProvider(request.Provider) == nil ||
		(request.Intent != model.AuthFlowIntentLogin && request.Intent != model.AuthFlowIntentBind) ||
		len(request.Aff) > 32 ||
		(request.Intent == model.AuthFlowIntentBind && request.Aff != "") {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userID := 0
	sessionID := ""
	if request.Intent == model.AuthFlowIntentBind {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "绑定操作需要登录"})
			return
		}
		userID = identity.UserID
		sessionID = identity.SessionID
	}
	if len(request.OrganizationInviteCode) > 128 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	payload, err := common.Marshal(oauthFlowPayload{
		AffiliateCode:          request.Aff,
		OrganizationInviteCode: request.OrganizationInviteCode,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiresAt := time.Now().Add(oauthAuthFlowTTL)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeOAuth,
		Provider:  request.Provider,
		Intent:    request.Intent,
		UserId:    userID,
		SessionId: sessionID,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"flow_token": state,
			"expires_at": expiresAt.Unix(),
		},
	})
}

// HandleOAuth handles OAuth callback for all standard OAuth providers
func HandleOAuth(c *gin.Context) {
	providerName := c.Param("provider")
	provider := oauth.GetProvider(providerName)
	if provider == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthUnknownProvider),
		})
		return
	}

	// 1. Validate state (CSRF protection)
	state := c.Query("state")
	pendingFlow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthStateInvalid),
		})
		return
	}

	consumeMatch := model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
		Intent:   pendingFlow.Intent,
	}
	// 2. Bind flows are bound to the live dashboard Session that created them.
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok || identity.UserID != pendingFlow.UserId || identity.SessionID != pendingFlow.SessionId {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgOAuthStateInvalid),
			})
			return
		}
		consumeMatch.UserId = identity.UserID
		consumeMatch.SessionId = identity.SessionID
	} else if pendingFlow.Intent != model.AuthFlowIntentLogin {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 3. Check if provider is enabled
	if !provider.IsEnabled() {
		common.ApiErrorI18n(c, i18n.MsgOAuthNotEnabled, providerParams(provider.GetName()))
		return
	}

	// 4. Handle error from provider
	errorCode := c.Query("error")
	if errorCode != "" {
		if _, err := model.ConsumeAuthFlow(state, consumeMatch); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
			return
		}
		errorDescription := c.Query("error_description")
		if errorDescription == "" {
			errorDescription = errorCode
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errorDescription,
		})
		return
	}
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		handleOAuthBind(c, provider, pendingFlow, state)
		return
	}

	// 5. Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		handleOAuthError(c, err)
		return
	}

	// 6. Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		handleOAuthError(c, err)
		return
	}
	// 7. Decode the state payload. Keep the historical behavior that a
	// syntactically malformed callback consumes the one-time state.
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(pendingFlow.Payload, &payload); err != nil {
		if _, consumeErr := model.ConsumeAuthFlow(state, consumeMatch); consumeErr != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
			return
		}
		common.ApiError(c, err)
		return
	}

	// Resolve existing identities before opening the registration transaction.
	// New-user creation itself is performed by the callback below so a failed
	// organization invite, ledger write, or provider binding leaves state
	// reusable instead of consuming it permanently.
	user, existingErr, existing := findExistingOAuthUser(provider, oauthUser)
	if existing {
		if _, consumeErr := model.ConsumeAuthFlow(state, consumeMatch); consumeErr != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
			return
		}
		err = existingErr
	} else {
		var candidate *model.User
		candidate, err = prepareOAuthRegistrationUser(provider, oauthUser)
		if err == nil {
			var registration *service.UserRegistrationResult
			_, err = model.ConsumeAuthFlowWithAction(state, consumeMatch, func(tx *gorm.DB, _ *model.AuthFlow) error {
				var registerErr error
				registration, registerErr = registerOAuthUserWithTx(tx, c, provider, oauthUser, candidate, payload.AffiliateCode, payload.OrganizationInviteCode)
				return registerErr
			})
			if err == nil {
				service.FinalizeRegisteredUser(registration)
				user = candidate
			}
		} else {
			// Terminal preflight failures historically consumed the callback state;
			// preserve that contract while allowing transactional registration
			// failures to roll back consumption.
			_, _ = model.ConsumeAuthFlow(state, consumeMatch)
		}
	}
	if err != nil {
		if errors.Is(err, model.ErrEmailAlreadyTaken) {
			common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
			return
		}
		switch err.(type) {
		case *OAuthUserDeletedError:
			common.ApiErrorI18n(c, i18n.MsgOAuthUserDeleted)
		case *OAuthRegistrationDisabledError:
			common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		case *OAuthEmailAlreadyTakenError:
			common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
		default:
			common.ApiError(c, err)
		}
		return
	}

	// 8. Check user status
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgOAuthUserBanned)
		return
	}

	// 9. Setup login
	setupLogin(user, c)
}

// handleOAuthBind handles binding OAuth account to existing user
func handleOAuthBind(c *gin.Context, provider oauth.Provider, pendingFlow *model.AuthFlow, flowToken string) {
	// Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		handleOAuthError(c, err)
		return
	}

	// Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		handleOAuthError(c, err)
		return
	}

	// Check if this OAuth account is already bound (check both new ID and legacy ID)
	if provider.IsUserIDTaken(oauthUser.ProviderUserID) {
		common.ApiErrorI18n(c, i18n.MsgOAuthAlreadyBound, providerParams(provider.GetName()))
		return
	}
	// Also check legacy ID to prevent duplicate bindings during migration period
	if legacyID, ok := oauthUser.Extra["legacy_id"].(string); ok && legacyID != "" {
		if provider.IsUserIDTaken(legacyID) {
			common.ApiErrorI18n(c, i18n.MsgOAuthAlreadyBound, providerParams(provider.GetName()))
			return
		}
	}

	if _, err := model.ConsumeAuthFlow(flowToken, model.AuthFlowMatch{
		Purpose:   model.AuthFlowPurposeOAuth,
		Provider:  pendingFlow.Provider,
		Intent:    model.AuthFlowIntentBind,
		UserId:    pendingFlow.UserId,
		SessionId: pendingFlow.SessionId,
	}); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return
	}

	userId := pendingFlow.UserId

	// Handle binding based on provider type
	if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
		// Custom provider: use user_oauth_bindings table
		err = model.UpdateUserOAuthBinding(userId, genericProvider.GetProviderId(), oauthUser.ProviderUserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		// Built-in provider: 只更新绑定列。完整快照的 user.Update 会把读取时刻的
		// role/status/group 一并写回，覆盖并发发生的封禁、降权或分组变更。
		err = model.UpdateUserBindColumn(userId, provider.ProviderUserIDColumn(), oauthUser.ProviderUserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	common.ApiSuccessI18n(c, i18n.MsgOAuthBindSuccess, gin.H{
		"action": "bind",
	})
}

// findExistingOAuthUser resolves an already-bound account. It is kept
// separate from creation so a new-account path can consume its state and
// register the account in one transaction.
func findExistingOAuthUser(provider oauth.Provider, oauthUser *oauth.OAuthUser) (*model.User, error, bool) {
	user := &model.User{}
	if provider.IsUserIDTaken(oauthUser.ProviderUserID) {
		if err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID); err != nil {
			return nil, err, true
		}
		if user.Id == 0 {
			return nil, &OAuthUserDeletedError{}, true
		}
		return user, nil, true
	}

	// GitHub historically keyed accounts by login; migrate that legacy key on
	// the first successful login while retaining the existing behavior.
	if legacyID, ok := oauthUser.Extra["legacy_id"].(string); ok && legacyID != "" && provider.IsUserIDTaken(legacyID) {
		if err := provider.FillUserByProviderID(user, legacyID); err != nil {
			return nil, err, true
		}
		if user.Id != 0 {
			common.SysLog(fmt.Sprintf("[OAuth] Migrating user %d from legacy_id=%s to new_id=%s",
				user.Id, legacyID, oauthUser.ProviderUserID))
			if err := user.UpdateGitHubId(oauthUser.ProviderUserID); err != nil {
				common.SysError(fmt.Sprintf("[OAuth] Failed to migrate user %d: %s", user.Id, err.Error()))
			}
			return user, nil, true
		}
	}
	return nil, nil, false
}

func prepareOAuthRegistrationUser(provider oauth.Provider, oauthUser *oauth.OAuthUser) (*model.User, error) {
	if !common.RegisterEnabled {
		return nil, &OAuthRegistrationDisabledError{}
	}
	user := &model.User{
		Username: provider.GetProviderPrefix() + strconv.Itoa(model.GetMaxUserId()+1),
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	if oauthUser.Username != "" {
		if exists, err := model.CheckUserExistOrDeleted(oauthUser.Username, ""); err == nil && !exists && len(oauthUser.Username) <= model.UserNameMaxLength {
			user.Username = oauthUser.Username
		}
	}
	if oauthUser.DisplayName != "" {
		user.DisplayName = oauthUser.DisplayName
	} else if oauthUser.Username != "" {
		user.DisplayName = oauthUser.Username
	} else {
		user.DisplayName = provider.GetName() + " User"
	}
	if oauthUser.Email != "" {
		user.Email = model.NormalizeEmail(oauthUser.Email)
		if err := model.EnsureEmailAvailable(user.Email, 0); err != nil {
			if errors.Is(err, model.ErrEmailAlreadyTaken) {
				return nil, &OAuthEmailAlreadyTakenError{}
			}
			return nil, err
		}
	}
	return user, nil
}

// registerOAuthUserWithTx provisions a new account and its provider binding
// using the caller's transaction. The caller must invoke it from
// ConsumeAuthFlowWithAction when state consumption must roll back on failure.
func registerOAuthUserWithTx(tx *gorm.DB, c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser, user *model.User, affiliateCode string, organizationInviteCode string) (*service.UserRegistrationResult, error) {
	params := service.UserRegistrationParams{
		User:                   user,
		AffiliateCode:          affiliateCode,
		OrganizationInviteCode: organizationInviteCode,
		RequestID:              c.GetString(common.RequestIdKey),
	}
	if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
		params.AfterCreateTx = func(tx *gorm.DB, createdUser *model.User) error {
			return model.CreateUserOAuthBindingWithTx(tx, &model.UserOAuthBinding{
				UserId:         createdUser.Id,
				ProviderId:     genericProvider.GetProviderId(),
				ProviderUserId: oauthUser.ProviderUserID,
			})
		}
	} else {
		params.AfterCreateTx = func(tx *gorm.DB, createdUser *model.User) error {
			provider.SetProviderUserID(createdUser, oauthUser.ProviderUserID)
			return tx.Model(createdUser).Updates(map[string]interface{}{
				"github_id":   createdUser.GitHubId,
				"discord_id":  createdUser.DiscordId,
				"oidc_id":     createdUser.OidcId,
				"linux_do_id": createdUser.LinuxDOId,
				"wechat_id":   createdUser.WeChatId,
				"telegram_id": createdUser.TelegramId,
			}).Error
		}
	}
	return service.RegisterUserWithTx(tx, params)
}

// findOrCreateOAuthUser is retained for callers that do not need to couple
// state consumption to registration (for example, compatibility integrations).
func findOrCreateOAuthUser(c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser, affiliateCode string, organizationInviteCode string) (*model.User, error) {
	if user, err, found := findExistingOAuthUser(provider, oauthUser); found {
		return user, err
	}
	user, err := prepareOAuthRegistrationUser(provider, oauthUser)
	if err != nil {
		return nil, err
	}
	var registration *service.UserRegistrationResult
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		registration, err = registerOAuthUserWithTx(tx, c, provider, oauthUser, user, affiliateCode, organizationInviteCode)
		return err
	}); err != nil {
		return nil, err
	}
	service.FinalizeRegisteredUser(registration)
	return user, nil
}

// Error types for OAuth
type OAuthUserDeletedError struct{}

func (e *OAuthUserDeletedError) Error() string {
	return "user has been deleted"
}

type OAuthRegistrationDisabledError struct{}

func (e *OAuthRegistrationDisabledError) Error() string {
	return "registration is disabled"
}

type OAuthEmailAlreadyTakenError struct{}

func (e *OAuthEmailAlreadyTakenError) Error() string {
	return "email is already in use"
}

// handleOAuthError handles OAuth errors and returns translated message
func handleOAuthError(c *gin.Context, err error) {
	switch e := err.(type) {
	case *oauth.OAuthError:
		if e.Params != nil {
			common.ApiErrorI18n(c, e.MsgKey, e.Params)
		} else {
			common.ApiErrorI18n(c, e.MsgKey)
		}
	case *oauth.AccessDeniedError:
		common.ApiErrorMsg(c, e.Message)
	case *oauth.TrustLevelError:
		common.ApiErrorI18n(c, i18n.MsgOAuthTrustLevelLow)
	default:
		common.ApiError(c, err)
	}
}
