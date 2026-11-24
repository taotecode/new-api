package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
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
	Provider       string          `json:"provider"`
	Intent         string          `json:"intent"`
	Aff            string          `json:"aff,omitempty"`
	InvitationCode string          `json:"invitation_code,omitempty"`
	Scope          string          `json:"scope,omitempty"`
	Context        json.RawMessage `json:"context,omitempty"`
}

type oauthFlowPayload struct {
	AffiliateCode       string                         `json:"affiliate_code,omitempty"`
	InvitationCodeID    int                            `json:"invitation_code_id,omitempty"`
	InvitationInviterID int                            `json:"invitation_inviter_id,omitempty"`
	Verification        *service.OAuthVerificationFlow `json:"verification,omitempty"`
	Telegram            *oauth.TelegramOAuthFlow       `json:"telegram,omitempty"`
	SessionIdentity     *service.AuthIdentity          `json:"session_identity,omitempty"`
	Authorization       *model.AuthFlowAuthorization   `json:"authorization,omitempty"`
}

// invitationPendingPayload carries a confirmed OAuth identity through the
// invitation completion step. The OAuth tokens are never stored; only the
// provider identity and profile fields needed to create the account later.
type invitationPendingPayload struct {
	Provider       string `json:"provider"`
	ProviderUserId string `json:"provider_user_id"`
	Username       string `json:"username,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	Email          string `json:"email,omitempty"`
	AffiliateCode  string `json:"affiliate_code,omitempty"`
}

// providerParams returns the params of messages that name the OAuth provider.
func providerParams(name string) map[string]any {
	return map[string]any{"provider": name}
}

// GenerateOAuthCode generates a state code for OAuth CSRF protection
func GenerateOAuthCode(c *gin.Context) {
	var request oauthStateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.Intent = strings.TrimSpace(request.Intent)
	request.Aff = strings.TrimSpace(request.Aff)
	if oauth.GetProvider(request.Provider) == nil ||
		(request.Intent != model.AuthFlowIntentLogin && request.Intent != model.AuthFlowIntentBind && request.Intent != model.AuthFlowIntentVerify) ||
		len(request.Aff) > 32 ||
		(request.Intent != model.AuthFlowIntentLogin && request.Aff != "") ||
		(request.Intent != model.AuthFlowIntentVerify && (request.Scope != "" || len(request.Context) != 0)) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	userID := 0
	sessionID := ""
	flowPayload := oauthFlowPayload{AffiliateCode: request.Aff}
	// Resolve the registration gate code server-side so the plaintext never
	// reaches the OAuth redirect URL or state. Unknown codes resolve to zero:
	// existing users log in either way and new users get the completion flow.
	invitationCode := strings.TrimSpace(request.InvitationCode)
	if request.Intent == model.AuthFlowIntentLogin && invitationCode != "" {
		if len(invitationCode) > common.InvitationCodeMaxLength {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		resolved := service.ResolveRegistrationInvitation(invitationCode)
		flowPayload.InvitationCodeID = resolved.CodeID
		flowPayload.InvitationInviterID = resolved.InviterID
	}
	bindingStarted := false
	if request.Provider == "telegram" {
		telegramFlow, err := oauth.NewTelegramOAuthFlow()
		if err != nil {
			writeSecurityOperationError(c, err)
			return
		}
		flowPayload.Telegram = telegramFlow
	}
	if request.Intent == model.AuthFlowIntentBind || request.Intent == model.AuthFlowIntentVerify {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok {
			writeAccountStatusError(c, http.StatusUnauthorized, "Sign in to bind an account")
			return
		}
		userID = identity.UserID
		sessionID = identity.SessionID
		if request.Intent == model.AuthFlowIntentBind {
			defer func() {
				recordUserSecurityAudit(c, userID, "user.binding_start", map[string]any{"provider": request.Provider, "success": bindingStarted})
			}()
			context, err := common.Marshal(service.AccountBindingContext{Provider: request.Provider})
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			flowPayload.Authorization = middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context})
			if flowPayload.Authorization == nil {
				return
			}
			flowPayload.SessionIdentity = &identity
		}
		if flowPayload.Telegram != nil {
			if _, _, err := service.ValidateLoginSession(identity); err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			flowPayload.SessionIdentity = &identity
		}
		if request.Intent == model.AuthFlowIntentVerify {
			verification, err := service.StartOAuthVerification(identity, service.VerificationOperation{Scope: request.Scope, Context: request.Context}, request.Provider)
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			flowPayload.Verification = verification
		}
	}
	payload, err := common.Marshal(flowPayload)
	if err != nil {
		writeSecurityOperationError(c, err)
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
		writeSecurityOperationError(c, err)
		return
	}
	bindingStarted = request.Intent == model.AuthFlowIntentBind
	data := gin.H{"flow_token": state, "expires_at": expiresAt.Unix()}
	if flowPayload.Telegram != nil {
		data["authorization_url"] = flowPayload.Telegram.AuthorizationURL(state)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

// HandleOAuth handles OAuth callback for all standard OAuth providers
func HandleOAuth(c *gin.Context) {
	providerName := c.Param("provider")
	provider := oauth.GetProvider(providerName)
	if provider == nil {
		writeAccountStatusError(c, http.StatusBadRequest, "Unknown OAuth provider")
		return
	}

	// 1. Validate state (CSRF protection)
	state := c.Query("state")
	pendingFlow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
	})
	if err != nil {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return
	}

	consumeMatch := model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
		Intent:   pendingFlow.Intent,
	}
	bindSucceeded, notificationFailed := false, false
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		defer func() {
			recordUserSecurityAudit(c, pendingFlow.UserId, "user.binding_bind", map[string]any{"provider": providerName, "success": bindSucceeded, "notification_failed": notificationFailed})
		}()
	}
	// Bind and verification callbacks must use the dashboard session that started them.
	if pendingFlow.Intent == model.AuthFlowIntentBind || pendingFlow.Intent == model.AuthFlowIntentVerify {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok || identity.UserID != pendingFlow.UserId || identity.SessionID != pendingFlow.SessionId {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
			return
		}
		consumeMatch.UserId = identity.UserID
		consumeMatch.SessionId = identity.SessionID
		if pendingFlow.Intent == model.AuthFlowIntentBind {
			var payload oauthFlowPayload
			if err := common.UnmarshalJsonStr(pendingFlow.Payload, &payload); err != nil {
				writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
				return
			}
			context, err := common.Marshal(service.AccountBindingContext{Provider: providerName})
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}, payload.Authorization); err != nil {
				writeSecurityOperationError(c, err)
				return
			}
		}
	} else if pendingFlow.Intent != model.AuthFlowIntentLogin {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	// 3. Check if provider is enabled
	var telegramPayload oauthFlowPayload
	if providerName == "telegram" {
		if err := oauth.TelegramConfigurationError(); err != nil {
			writeSecurityOperationError(c, err)
			return
		}
		if err := common.UnmarshalJsonStr(pendingFlow.Payload, &telegramPayload); err != nil || telegramPayload.Telegram == nil {
			writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
			return
		}
		if pendingFlow.Intent != model.AuthFlowIntentLogin {
			identity, _ := middleware.GetSessionAuthIdentity(c)
			if telegramPayload.SessionIdentity == nil || *telegramPayload.SessionIdentity != identity {
				writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
				return
			}
			if _, _, err := service.ValidateLoginSession(identity); err != nil {
				writeSecurityOperationError(c, err)
				return
			}
		}
		c.Set(oauth.TelegramOAuthFlowContextKey, telegramPayload.Telegram)
	}
	if !provider.IsEnabled() {
		common.ApiErrorT(c, "{{provider}} login and registration has not been enabled by administrator", providerParams(provider.GetName()))
		return
	}

	// 4. Handle error from provider
	errorCode := c.Query("error")
	if errorCode != "" {
		if _, err := model.ConsumeAuthFlow(state, consumeMatch); err != nil {
			writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
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
	// 5. Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		if providerName == "telegram" {
			writeSecurityOperationError(c, err)
			return
		}
		handleOAuthError(c, err)
		return
	}

	// 6. Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		if providerName == "telegram" {
			writeSecurityOperationError(c, err)
			return
		}
		handleOAuthError(c, err)
		return
	}
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		bindSucceeded, notificationFailed = handleOAuthBind(c, providerName, provider, oauthUser, pendingFlow, state, consumeMatch)
		return
	}
	flow, err := model.ConsumeAuthFlow(state, consumeMatch)
	if err != nil {
		writeAccountStatusError(c, http.StatusForbidden, "State parameter is empty or mismatched")
		return
	}

	switch flow.Intent {
	case model.AuthFlowIntentLogin:
		handleOAuthLogin(c, providerName, provider, oauthUser, token, flow)
	case model.AuthFlowIntentVerify:
		handleOAuthVerification(c, providerName, oauthUser, flow)
	}
}

func handleOAuthVerification(c *gin.Context, provider string, oauthUser *oauth.OAuthUser, flow *model.AuthFlow) {
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	identity, _ := middleware.GetSessionAuthIdentity(c)
	proof, err := service.FinishOAuthVerification(identity, provider, oauthUser.ProviderUserID, payload.Verification)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "user.security_verify", map[string]any{"method": proof.Method, "scope": proof.Scope, "provider": provider})
	common.ApiSuccess(c, proof)
}

func handleOAuthLogin(c *gin.Context, providerName string, provider oauth.Provider, oauthUser *oauth.OAuthUser, token *oauth.OAuthToken, flow *model.AuthFlow) {
	// 7. Find or create user
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	user, migration, err := findOrCreateOAuthUser(c, providerName, provider, oauthUser, token, payload)
	if err != nil {
		var invitationErr *InvitationRequiredError
		if errors.As(err, &invitationErr) {
			writeInvitationRequired(c, invitationErr.FlowToken)
			return
		}
		if errors.Is(err, model.ErrEmailAlreadyTaken) {
			common.ApiErrorT(c, "Email address is already in use")
			return
		}
		switch err.(type) {
		case *OAuthUserDeletedError:
			common.ApiErrorT(c, "User has been deleted")
		case *OAuthRegistrationDisabledError:
			common.ApiErrorT(c, "New user registration has been disabled by administrator")
		case *OAuthEmailAlreadyTakenError:
			common.ApiErrorT(c, "Email address is already in use")
		case *OAuthLegacyBindingNotConfirmedError:
			common.ApiErrorT(c, "This {{provider}} account cannot be linked to an existing account automatically, please sign in or register another way and then link {{provider}} in account settings", providerParams(provider.GetName()))
		default:
			writeSecurityOperationError(c, err)
		}
		return
	}

	// 8. Check user status
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorT(c, "User has been banned")
		return
	}

	// 9. Setup login
	setupLogin(user, migration, c)
}

// handleOAuthBind handles binding OAuth account to existing user
func handleOAuthBind(c *gin.Context, providerName string, provider oauth.Provider, oauthUser *oauth.OAuthUser, flow *model.AuthFlow, state string, match model.AuthFlowMatch) (bool, bool) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return false, false
	}
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return false, false
	}
	context, err := common.Marshal(service.AccountBindingContext{Provider: providerName})
	if err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	// Recheck after the external provider round trip, then validate the session
	// under the transaction's locks before consuming the flow and writing.
	if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: context}, payload.Authorization); err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	if provider.IsUserIDTaken(oauthUser.ProviderUserID) {
		common.ApiErrorT(c, "This {{provider}} account has already been bound", providerParams(provider.GetName()))
		return false, false
	}
	_, err = model.ConsumeAuthFlowWithAction(state, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		if providerName == "telegram" {
			return model.BindTelegramForSessionWithTx(tx, identity, oauthUser.ProviderUserID)
		}
		if custom, ok := provider.(*oauth.GenericOAuthProvider); ok {
			return model.UpdateUserOAuthBindingForSessionWithTx(tx, identity, custom.GetProviderId(), oauthUser.ProviderUserID)
		}
		return model.UpdateUserBindColumnForSessionWithTx(tx, identity, provider.ProviderUserIDColumn(), oauthUser.ProviderUserID)
	})
	if err != nil {
		writeSecurityOperationError(c, err)
		return false, false
	}
	user, err := model.GetUserById(identity.UserID, false)
	if err != nil {
		writeSecurityOperationError(c, err)
		return true, true
	}
	notificationFailed := service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, user.Id), user.Email, i18n.MsgSecurityEventLoginLinked, map[string]any{"Provider": provider.GetName()}) != nil
	common.ApiSuccessT(c, "Binding successful", gin.H{"action": "bind", "notification_warning": notificationFailed})
	return true, notificationFailed
}

// findOrCreateOAuthUser finds the existing user or creates a new one. For a
// legacy GitHub binding that still waits for the login verification, it also
// returns the rewrite to carry into the challenge. When a new identity hits
// the registration gate without a valid code it returns an
// InvitationRequiredError carrying the pending flow token instead of creating
// the account.
func findOrCreateOAuthUser(c *gin.Context, providerName string, provider oauth.Provider, oauthUser *oauth.OAuthUser, token *oauth.OAuthToken, payload oauthFlowPayload) (*model.User, *service.LegacyGitHubMigration, error) {
	user := &model.User{}
	if provider.ProviderUserIDColumn() == "telegram_id" {
		err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, oauth.ErrTelegramAccountNotBound
		}
		return user, nil, err
	}

	// Check if user already exists with new ID
	if provider.IsUserIDTaken(oauthUser.ProviderUserID) {
		err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID)
		if err != nil {
			return nil, nil, err
		}
		// Check if user has been deleted
		if user.Id == 0 {
			return nil, nil, &OAuthUserDeletedError{}
		}
		return user, nil, nil
	}

	// Legacy GitHub bindings stored the login name, which only points at a
	// candidate account. Values that are all digits are already numeric account
	// IDs and never take part in the comparison.
	legacyID, _ := oauthUser.Extra["legacy_id"].(string)
	if strings.ContainsFunc(legacyID, func(r rune) bool { return r < '0' || r > '9' }) && provider.IsUserIDTaken(legacyID) {
		if err := provider.FillUserByProviderID(user, legacyID); err != nil {
			return nil, nil, err
		}
		if user.Id != 0 {
			state, err := model.GetUserVerificationState(user.Id)
			if err != nil {
				return nil, nil, err
			}
			if state.HasTwoFA || state.HasPasskey {
				// The rewrite is written in the transaction that issues the session
				// once the login verification completes.
				return user, &service.LegacyGitHubMigration{GitHubID: oauthUser.ProviderUserID, LegacyID: legacyID}, nil
			}
			// Without a second factor, one of the addresses the provider has
			// confirmed must match the account email. The list is fetched only here
			// and never recorded.
			reason, matched := "no_matching_evidence", false
			if emailProvider, ok := provider.(oauth.VerifiedEmailProvider); ok && user.Email != "" {
				emails, err := emailProvider.GetVerifiedEmails(c.Request.Context(), token)
				if err != nil {
					common.SysError(common.LogText("[OAuth] Failed to load verified emails for user %d: %s", user.Id, err.Error()))
					reason = "verified_emails_unavailable"
				}
				accountEmail := model.NormalizeEmail(user.Email)
				matched = slices.ContainsFunc(emails, func(email string) bool { return model.NormalizeEmail(email) == accountEmail })
			}
			if !matched {
				recordLegacyGitHubBindingAudit(c, user, false, map[string]any{"legacy_id": legacyID, "provider_user_id": oauthUser.ProviderUserID, "reason": reason})
				return nil, nil, &OAuthLegacyBindingNotConfirmedError{}
			}
			written := false
			err = model.DB.Transaction(func(tx *gorm.DB) error {
				var err error
				written, err = model.MigrateLegacyGitHubBindingWithTx(tx, user.Id, legacyID, oauthUser.ProviderUserID)
				return err
			})
			if err != nil {
				return nil, nil, err
			}
			if written {
				user.GitHubId = oauthUser.ProviderUserID
				notificationFailed := service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, user.Id), user.Email, i18n.MsgSecurityEventLoginLinked, map[string]any{"Provider": provider.GetName()}) != nil
				recordLegacyGitHubBindingAudit(c, user, true, map[string]any{
					"legacy_id": legacyID, "provider_user_id": oauthUser.ProviderUserID,
					"verified_email_matched": true, "notification_failed": notificationFailed,
				})
			}
			return user, nil, nil
		}
	}

	// User doesn't exist, create new user if registration is enabled
	if !common.RegisterEnabled {
		return nil, nil, &OAuthRegistrationDisabledError{}
	}

	// Handle affiliate code
	inviterId := 0
	if payload.AffiliateCode != "" {
		inviterId, _ = model.GetUserIdByAffCode(payload.AffiliateCode)
	}

	// Registration gate: an OAuth entry only needs a gate code when the
	// corresponding method is configured. Existing users never reach this
	// branch.
	method := oauthInvitationMethod(providerName, provider)
	invitation := service.RegistrationInvitation{}
	if common.IsInvitationRequiredForMethod(method) {
		switch {
		case payload.InvitationCodeID > 0:
			invitation.CodeID = payload.InvitationCodeID
		case payload.InvitationInviterID > 0:
			invitation.InviterID = payload.InvitationInviterID
		case common.InvitationAffAllowed && inviterId > 0:
			// A valid affiliate code passes the gate on its own.
		default:
			// Hold the confirmed identity and ask for a gate code before any
			// account is written.
			flowToken, err := createInvitationPendingFlow(invitationPendingPayload{
				Provider:       providerName,
				ProviderUserId: oauthUser.ProviderUserID,
				Username:       oauthUser.Username,
				DisplayName:    oauthUser.DisplayName,
				Email:          oauthUser.Email,
				AffiliateCode:  payload.AffiliateCode,
			})
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, &InvitationRequiredError{FlowToken: flowToken}
		}
	}
	if invitation.InviterID > 0 {
		inviterId = invitation.InviterID
	}

	// Set up new user
	user.Username = provider.GetProviderPrefix() + strconv.Itoa(model.GetMaxUserId()+1)

	if oauthUser.Username != "" {
		if exists, err := model.CheckUserExistOrDeleted(oauthUser.Username, ""); err == nil && !exists {
			// 防止索引退化
			if len(oauthUser.Username) <= model.UserNameMaxLength {
				user.Username = oauthUser.Username
			}
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
				return nil, nil, &OAuthEmailAlreadyTakenError{}
			}
			return nil, nil, err
		}
	}
	user.Role = common.RoleCommonUser
	user.Status = common.UserStatusEnabled
	user.RegisterMethod = method
	user.InvitationCodeId = invitation.CodeID
	user.InviterId = inviterId

	// Use transaction to ensure user creation, gate consumption, and OAuth
	// binding are atomic.
	bindOAuthIdentity := func(tx *gorm.DB) error {
		if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
			binding := &model.UserOAuthBinding{
				UserId:         user.Id,
				ProviderId:     genericProvider.GetProviderId(),
				ProviderUserId: oauthUser.ProviderUserID,
			}
			return model.CreateUserOAuthBindingWithTx(tx, binding)
		}
		provider.SetProviderUserID(user, oauthUser.ProviderUserID)
		return tx.Model(user).Updates(map[string]any{
			"github_id":   user.GitHubId,
			"discord_id":  user.DiscordId,
			"oidc_id":     user.OidcId,
			"linux_do_id": user.LinuxDOId,
			"wechat_id":   user.WeChatId,
			"telegram_id": user.TelegramId,
		}).Error
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := user.InsertWithTx(tx, inviterId); err != nil {
			return err
		}
		if invitation.CodeID > 0 {
			if _, err := model.ConsumeInvitationCodeReferenceWithTx(tx, invitation.CodeID, user.Id); err != nil {
				return err
			}
		}
		return bindOAuthIdentity(tx)
	})
	if err != nil {
		// The pre-filled code lost its race (used, disabled, or expired between
		// state creation and callback). Fall back to the completion flow so the
		// user can enter another code without repeating the provider round trip.
		if invitation.CodeID > 0 && (errors.Is(err, model.ErrInvitationCodeUsed) || errors.Is(err, model.ErrInvitationCodeInvalid)) {
			flowToken, perr := createInvitationPendingFlow(invitationPendingPayload{
				Provider:       providerName,
				ProviderUserId: oauthUser.ProviderUserID,
				Username:       oauthUser.Username,
				DisplayName:    oauthUser.DisplayName,
				Email:          oauthUser.Email,
				AffiliateCode:  payload.AffiliateCode,
			})
			if perr != nil {
				return nil, nil, perr
			}
			return nil, nil, &InvitationRequiredError{FlowToken: flowToken}
		}
		return nil, nil, err
	}

	// Perform post-transaction tasks (logs, sidebar config, inviter rewards)
	user.FinalizeOAuthUserCreation(inviterId)
	return user, nil, nil
}

// recordLegacyGitHubBindingAudit records the outcome of a legacy GitHub binding
// rewrite as an account binding event. No session exists yet on the login path,
// so the row carries the user's own role instead of the request context.
func recordLegacyGitHubBindingAudit(c *gin.Context, user *model.User, success bool, params map[string]any) {
	params["provider"], params["legacy_migration"], params["success"] = "github", true, success
	model.RecordOperationAuditLog(user.Id, user.Role, auditContentEN("user.binding_bind", params), c.ClientIP(), "user.binding_bind", params, nil, &model.AuditRequestInfo{
		Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: c.Writer.Status(), Success: success,
	}, c)
}

// InvitationRequiredError reports that a confirmed OAuth identity paused at
// the registration gate; no account was created. FlowToken references the
// pending identity so the user can complete registration with a gate code.
type InvitationRequiredError struct {
	FlowToken string
}

func (e *InvitationRequiredError) Error() string {
	return "invitation code required to complete registration"
}

func writeInvitationRequired(c *gin.Context, flowToken string) {
	setAuthNoStore(c)
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": i18n.T(c, i18n.MsgInvitationRequired),
		"data": gin.H{
			"invitation_required": true,
			"flow_token":          flowToken,
		},
	})
}

// createInvitationPendingFlow stores a provider-confirmed identity for the
// invitation completion step. The pending flow is single-use and expires with
// the same TTL as the OAuth state, and never contains OAuth tokens.
func createInvitationPendingFlow(pending invitationPendingPayload) (string, error) {
	payload, err := common.Marshal(pending)
	if err != nil {
		return "", err
	}
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeInvitation,
		Provider:  pending.Provider,
		Payload:   string(payload),
		ExpiresAt: time.Now().Add(oauthAuthFlowTTL),
	})
	return state, err
}

// oauthInvitationMethod maps a provider to the registration method recorded on
// the user and matched against the invitation gate configuration.
func oauthInvitationMethod(providerName string, provider oauth.Provider) string {
	if _, ok := provider.(*oauth.GenericOAuthProvider); ok {
		return common.RegisterMethodCustomOAuth
	}
	switch providerName {
	case "github":
		return common.RegisterMethodGitHub
	case "discord":
		return common.RegisterMethodDiscord
	case "oidc":
		return common.RegisterMethodOIDC
	case "linuxdo":
		return common.RegisterMethodLinuxDO
	default:
		return ""
	}
}

type invitationCompleteRequest struct {
	FlowToken      string `json:"flow_token"`
	InvitationCode string `json:"invitation_code"`
}

// CompleteOAuthInvitation finishes a registration that paused at the
// invitation gate. The pending provider identity is only written after the
// submitted gate code is atomically consumed or resolved inside the same
// transaction that consumes the flow, so an invalid code never creates an
// account.
func CompleteOAuthInvitation(c *gin.Context) {
	var request invitationCompleteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	request.FlowToken = strings.TrimSpace(request.FlowToken)
	request.InvitationCode = strings.TrimSpace(request.InvitationCode)
	if request.FlowToken == "" || request.InvitationCode == "" || len(request.InvitationCode) > common.InvitationCodeMaxLength {
		common.ApiErrorI18n(c, i18n.MsgInvitationRequired)
		return
	}
	flow, err := model.GetAuthFlow(request.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeInvitation})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return
	}
	var pending invitationPendingPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &pending); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return
	}

	var provider oauth.Provider
	method := ""
	isWeChat := pending.Provider == "wechat"
	// Registration may have been disabled while the flow was pending; a held
	// identity must not bypass that policy.
	if !common.RegisterEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		return
	}
	if isWeChat {
		if !common.WeChatAuthEnabled {
			common.ApiErrorI18n(c, i18n.MsgOAuthNotEnabled, providerParams("WeChat"))
			return
		}
		method = common.RegisterMethodWeChat
	} else {
		provider = oauth.GetProvider(pending.Provider)
		if provider == nil || !provider.IsEnabled() || provider.ProviderUserIDColumn() == "telegram_id" {
			common.ApiErrorI18n(c, i18n.MsgInvitationCompleteFailed)
			return
		}
		method = oauthInvitationMethod(pending.Provider, provider)
	}
	// The gate may have been turned off or narrowed while the flow was pending.
	if !common.IsInvitationRequiredForMethod(method) {
		common.ApiErrorI18n(c, i18n.MsgInvitationCompleteNotAllowed)
		return
	}

	// Re-check the identity: it must not have been bound to an account while
	// the user was entering the code.
	if isWeChat {
		if model.IsWeChatIdAlreadyTaken(pending.ProviderUserId) {
			common.ApiErrorI18n(c, i18n.MsgInvitationCompleteFailed)
			return
		}
	} else if provider.IsUserIDTaken(pending.ProviderUserId) {
		common.ApiErrorI18n(c, i18n.MsgInvitationCompleteFailed)
		return
	}

	invitation := service.ResolveRegistrationInvitation(request.InvitationCode)
	if invitation.CodeID == 0 && invitation.InviterID == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvitationInvalid)
		return
	}
	inviterId := invitation.InviterID
	if inviterId == 0 && pending.AffiliateCode != "" {
		inviterId, _ = model.GetUserIdByAffCode(pending.AffiliateCode)
	}

	// Assemble the account from the confirmed identity, mirroring the create
	// branch of findOrCreateOAuthUser.
	user := &model.User{}
	if isWeChat {
		user.Username = "wechat_" + strconv.Itoa(model.GetMaxUserId()+1)
		user.DisplayName = "WeChat User"
		user.WeChatId = pending.ProviderUserId
	} else {
		user.Username = provider.GetProviderPrefix() + strconv.Itoa(model.GetMaxUserId()+1)
		if pending.Username != "" {
			if exists, err := model.CheckUserExistOrDeleted(pending.Username, ""); err == nil && !exists {
				// 防止索引退化
				if len(pending.Username) <= model.UserNameMaxLength {
					user.Username = pending.Username
				}
			}
		}
		if pending.DisplayName != "" {
			user.DisplayName = pending.DisplayName
		} else if pending.Username != "" {
			user.DisplayName = pending.Username
		} else {
			user.DisplayName = provider.GetName() + " User"
		}
	}
	if pending.Email != "" {
		user.Email = model.NormalizeEmail(pending.Email)
		if err := model.EnsureEmailAvailable(user.Email, 0); err != nil {
			if errors.Is(err, model.ErrEmailAlreadyTaken) {
				common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
				return
			}
			common.ApiError(c, err)
			return
		}
	}
	user.Role = common.RoleCommonUser
	user.Status = common.UserStatusEnabled
	user.RegisterMethod = method
	user.InvitationCodeId = invitation.CodeID

	bindIdentity := func(tx *gorm.DB) error {
		if isWeChat {
			return nil
		}
		if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
			binding := &model.UserOAuthBinding{
				UserId:         user.Id,
				ProviderId:     genericProvider.GetProviderId(),
				ProviderUserId: pending.ProviderUserId,
			}
			return model.CreateUserOAuthBindingWithTx(tx, binding)
		}
		provider.SetProviderUserID(user, pending.ProviderUserId)
		return tx.Model(user).Updates(map[string]any{
			"github_id":   user.GitHubId,
			"discord_id":  user.DiscordId,
			"oidc_id":     user.OidcId,
			"linux_do_id": user.LinuxDOId,
			"wechat_id":   user.WeChatId,
			"telegram_id": user.TelegramId,
		}).Error
	}
	if _, err := model.ConsumeAuthFlowWithAction(request.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeInvitation}, func(tx *gorm.DB, _ *model.AuthFlow) error {
		if err := user.InsertWithTx(tx, inviterId); err != nil {
			return err
		}
		if invitation.CodeID > 0 {
			if _, err := model.ConsumeInvitationCodeReferenceWithTx(tx, invitation.CodeID, user.Id); err != nil {
				if errors.Is(err, model.ErrInvitationCodeUsed) || errors.Is(err, model.ErrInvitationCodeInvalid) {
					return service.ErrInvitationRejected
				}
				return err
			}
		}
		return bindIdentity(tx)
	}); err != nil {
		if errors.Is(err, service.ErrInvitationRejected) {
			common.ApiErrorI18n(c, i18n.MsgInvitationInvalid)
			return
		}
		if errors.Is(err, model.ErrAuthFlowInvalid) || errors.Is(err, model.ErrAuthFlowExpired) || errors.Is(err, model.ErrAuthFlowConsumed) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
			return
		}
		common.ApiError(c, err)
		return
	}

	user.FinalizeOAuthUserCreation(inviterId)
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgOAuthUserBanned)
		return
	}
	setupLogin(user, nil, c)
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

// OAuthLegacyBindingNotConfirmedError reports a legacy GitHub binding match that
// neither a second factor nor a confirmed provider email backed.
type OAuthLegacyBindingNotConfirmedError struct{}

func (e *OAuthLegacyBindingNotConfirmedError) Error() string {
	return "legacy binding was not confirmed"
}

// handleOAuthError handles OAuth errors and returns a web console message
func handleOAuthError(c *gin.Context, err error) {
	switch e := err.(type) {
	case *oauth.OAuthError:
		common.ApiError(c, e.Message)
	case *oauth.AccessDeniedError:
		common.ApiErrorMsg(c, e.Message)
	case *oauth.TrustLevelError:
		common.ApiErrorT(c, "Linux DO trust level does not meet the minimum required by administrator")
	default:
		writeSecurityOperationError(c, err)
	}
}
