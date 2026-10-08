package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type invitationTestState struct {
	db                  *gorm.DB
	previousRequired    bool
	previousMethods     []string
	previousAffAllowed  bool
	previousRegister    bool
	previousPassword    bool
	previousEmailVerify bool
	previousGitHubOAuth bool
}

func setupInvitationControllerTest(t *testing.T) *invitationTestState {
	t.Helper()
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.InvitationCode{}))
	require.NoError(t, db.AutoMigrate(&model.UserOAuthBinding{}))

	state := &invitationTestState{
		db:                  db,
		previousRequired:    common.InvitationRequired,
		previousMethods:     common.InvitationMethods,
		previousAffAllowed:  common.InvitationAffAllowed,
		previousRegister:    common.RegisterEnabled,
		previousPassword:    common.PasswordRegisterEnabled,
		previousEmailVerify: common.EmailVerificationEnabled,
		previousGitHubOAuth: common.GitHubOAuthEnabled,
	}
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.GitHubOAuthEnabled = true
	common.InvitationRequired = false
	common.InvitationMethods = []string{common.RegisterMethodPassword}
	common.InvitationAffAllowed = true

	t.Cleanup(func() {
		common.InvitationRequired = state.previousRequired
		common.InvitationMethods = state.previousMethods
		common.InvitationAffAllowed = state.previousAffAllowed
		common.RegisterEnabled = state.previousRegister
		common.PasswordRegisterEnabled = state.previousPassword
		common.EmailVerificationEnabled = state.previousEmailVerify
		common.GitHubOAuthEnabled = state.previousGitHubOAuth
	})
	return state
}

func enableInvitationGate(methods ...string) {
	common.InvitationRequired = true
	if len(methods) > 0 {
		common.InvitationMethods = methods
	}
}

func createInvitationCodesForTest(t *testing.T, name string, count int) []string {
	t.Helper()
	codes, err := model.CreateInvitationCodes(name, count, 1, 0)
	require.NoError(t, err)
	return codes
}

func createAffInviterForTest(t *testing.T, username string, affCode string) int {
	t.Helper()
	inviter := &model.User{
		Username: username, Password: "password", AffCode: affCode,
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(inviter).Error)
	return inviter.Id
}

func performRegisterRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	Register(c)
	return recorder
}

type registerResult struct {
	recorder *httptest.ResponseRecorder
	success  bool
	message  string
}

func decodeRegisterResult(t *testing.T, recorder *httptest.ResponseRecorder) registerResult {
	t.Helper()
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &response))
	return registerResult{recorder: recorder, success: response.Success, message: response.Message}
}

func TestRegisterGateOffKeepsExistingBehavior(t *testing.T) {
	setupInvitationControllerTest(t)
	common.InvitationRequired = false

	result := decodeRegisterResult(t, performRegisterRequest(t, `{"username":"gate-off-user","password":"password123"}`))
	require.True(t, result.success, result.message)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "gate-off-user").First(&user).Error)
	assert.Zero(t, user.InvitationCodeId)
}

func TestRegisterGateRequiresAndConsumesInvitationCode(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate()

	// Missing code is rejected before any account is written.
	result := decodeRegisterResult(t, performRegisterRequest(t, `{"username":"gated-no-code","password":"password123"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationRequired), result.message)

	// Unknown codes get the unified rejection without enumeration hints.
	result = decodeRegisterResult(t, performRegisterRequest(t, `{"username":"gated-bad-code","password":"password123","invitation_code":"INV-NOT-A-REAL-CODE"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationInvalid), result.message)

	// A valid code registers, records the registration info, and is consumed.
	codes := createInvitationCodesForTest(t, "gate-batch", 1)
	body := `{"username":"gated-ok","password":"password123","invitation_code":"` + codes[0] + `"}`
	result = decodeRegisterResult(t, performRegisterRequest(t, body))
	require.True(t, result.success, result.message)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "gated-ok").First(&user).Error)
	assert.Equal(t, common.RegisterMethodPassword, user.RegisterMethod)
	assert.NotZero(t, user.InvitationCodeId)

	var consumed model.InvitationCode
	require.NoError(t, model.DB.Where("id = ?", user.InvitationCodeId).First(&consumed).Error)
	assert.Equal(t, common.InvitationCodeStatusUsed, consumed.Status)
	assert.Equal(t, user.Id, consumed.UsedUserId)

	// The one-time code cannot register a second account.
	result = decodeRegisterResult(t, performRegisterRequest(t, `{"username":"gated-second","password":"password123","invitation_code":"`+codes[0]+`"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationInvalid), result.message)
}

func TestRegisterGateAcceptsAffInvitationCodeAndBindsInviter(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate()
	inviterId := createAffInviterForTest(t, "aff-inviter", "AFF01")

	body := `{"username":"aff-gated","password":"password123","invitation_code":"AFF01"}`
	result := decodeRegisterResult(t, performRegisterRequest(t, body))
	require.True(t, result.success, result.message)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "aff-gated").First(&user).Error)
	assert.Equal(t, inviterId, user.InviterId)
	assert.Zero(t, user.InvitationCodeId, "aff codes pass the gate without consuming an invitation row")

	// Disabling the aff channel makes the same code unusable for the gate.
	common.InvitationAffAllowed = false
	result = decodeRegisterResult(t, performRegisterRequest(t, `{"username":"aff-off","password":"password123","invitation_code":"AFF01"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationInvalid), result.message)
}

func i18nMessage(t *testing.T, key string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return i18n.T(c, key)
}

func performOAuthStateRequest(t *testing.T, body string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	GenerateOAuthCode(c)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &response))
	require.True(t, response.Success, recorder.Body.String())
	return recorder.Code, response.Data.FlowToken
}

func TestOAuthStateResolvesInvitationCodeServerSide(t *testing.T) {
	setupInvitationControllerTest(t)
	codes := createInvitationCodesForTest(t, "state-batch", 1)

	_, flowToken := performOAuthStateRequest(t, `{"provider":"github","intent":"login","invitation_code":"`+codes[0]+`"}`)
	flow, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	require.NoError(t, err)

	var payload oauthFlowPayload
	require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &payload))
	assert.NotZero(t, payload.InvitationCodeID)
	assert.Zero(t, payload.InvitationInviterID)

	// Unknown codes resolve silently to zero so existing users are unaffected.
	_, flowToken = performOAuthStateRequest(t, `{"provider":"github","intent":"login","invitation_code":"INV-NOT-A-REAL-CODE"}`)
	flow, err = model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
	require.NoError(t, err)
	unknownPayload := oauthFlowPayload{}
	require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &unknownPayload))
	assert.Zero(t, unknownPayload.InvitationCodeID)
	assert.Zero(t, unknownPayload.InvitationInviterID)

	// The plaintext never travels through the flow payload.
	assert.NotContains(t, flow.Payload, "INV-")
}

func performFindOrCreateOAuthUser(t *testing.T, payload oauthFlowPayload, oauthUser *oauth.OAuthUser) (*model.User, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	provider := oauth.GetProvider("github")
	user, _, err := findOrCreateOAuthUser(c, "github", provider, oauthUser, nil, payload)
	return user, err
}

func TestFindOrCreateOAuthUserHoldsIdentityAtGateWithoutCode(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodGitHub)

	var invitationErr *InvitationRequiredError
	_, err := performFindOrCreateOAuthUser(t, oauthFlowPayload{}, &oauth.OAuthUser{ProviderUserID: "gh-gate-1"})
	require.ErrorAs(t, err, &invitationErr)
	assert.NotEmpty(t, invitationErr.FlowToken)

	// The pending identity is stored server-side, single-use, and expires.
	flow, err := model.GetAuthFlow(invitationErr.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeInvitation})
	require.NoError(t, err)
	assert.Equal(t, "github", flow.Provider)
	assert.True(t, flow.ExpiresAt.After(time.Now()))
	var pending invitationPendingPayload
	require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &pending))
	assert.Equal(t, "gh-gate-1", pending.ProviderUserId)
	// No user may exist while the identity is held at the gate.
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("github_id = ?", "gh-gate-1").Count(&count).Error)
	assert.Zero(t, count)
}

func TestFindOrCreateOAuthUserCreatesWithPreResolvedCode(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodGitHub)
	codes := createInvitationCodesForTest(t, "pre-batch", 1)
	codeID, err := model.ResolveInvitationCodeID(codes[0])
	require.NoError(t, err)

	payload := oauthFlowPayload{InvitationCodeID: codeID}
	_, err = performFindOrCreateOAuthUser(t, payload, &oauth.OAuthUser{ProviderUserID: "gh-gate-2", DisplayName: "Gate Two"})
	require.NoError(t, err)

	var user model.User
	require.NoError(t, model.DB.Where("github_id = ?", "gh-gate-2").First(&user).Error)
	assert.Equal(t, common.RegisterMethodGitHub, user.RegisterMethod)
	assert.Equal(t, codeID, user.InvitationCodeId)

	var consumed model.InvitationCode
	require.NoError(t, model.DB.Where("id = ?", codeID).First(&consumed).Error)
	assert.Equal(t, common.InvitationCodeStatusUsed, consumed.Status)
	assert.Equal(t, user.Id, consumed.UsedUserId)
}

func TestFindOrCreateOAuthUserSkipsGateForExistingIdentity(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodGitHub)
	existing := &model.User{
		Username: "existing-oauth-user", Password: "password", GitHubId: "gh-existing",
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(existing).Error)

	_, err := performFindOrCreateOAuthUser(t, oauthFlowPayload{}, &oauth.OAuthUser{ProviderUserID: "gh-existing"})
	require.NoError(t, err)
}

func createInvitationPendingFlowForTest(t *testing.T, pending invitationPendingPayload) string {
	t.Helper()
	flowToken, err := createInvitationPendingFlow(pending)
	require.NoError(t, err)
	return flowToken
}

func performInvitationComplete(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/invitation/complete", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	CompleteOAuthInvitation(c)
	return recorder
}

func TestCompleteOAuthInvitationCreatesAccountOnlyWithValidCode(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodGitHub)
	codes := createInvitationCodesForTest(t, "complete-batch", 1)
	flowToken := createInvitationPendingFlowForTest(t, invitationPendingPayload{
		Provider: "github", ProviderUserId: "gh-complete-1", DisplayName: "Complete One",
	})

	// An invalid code must not consume the flow; the user can retry.
	result := decodeRegisterResult(t, performInvitationComplete(t, `{"flow_token":"`+flowToken+`","invitation_code":"INV-NOT-A-REAL-CODE"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationInvalid), result.message)
	_, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeInvitation})
	require.NoError(t, err, "an invalid code must leave the pending flow alive for retry")

	// A valid code creates the account atomically and issues a login.
	result = decodeRegisterResult(t, performInvitationComplete(t, `{"flow_token":"`+flowToken+`","invitation_code":"`+codes[0]+`"}`))
	require.True(t, result.success, result.message)

	var user model.User
	require.NoError(t, model.DB.Where("github_id = ?", "gh-complete-1").First(&user).Error)
	assert.Equal(t, common.RegisterMethodGitHub, user.RegisterMethod)
	assert.NotZero(t, user.InvitationCodeId)

	// The flow is single-use: replaying it no longer works.
	result = decodeRegisterResult(t, performInvitationComplete(t, `{"flow_token":"`+flowToken+`","invitation_code":"`+codes[0]+`"}`))
	assert.False(t, result.success)
}

func TestCompleteOAuthInvitationRejectsWhenGateDisabled(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodGitHub)
	codes := createInvitationCodesForTest(t, "disabled-batch", 1)
	flowToken := createInvitationPendingFlowForTest(t, invitationPendingPayload{
		Provider: "github", ProviderUserId: "gh-complete-2",
	})

	common.InvitationRequired = false
	result := decodeRegisterResult(t, performInvitationComplete(t, `{"flow_token":"`+flowToken+`","invitation_code":"`+codes[0]+`"}`))
	assert.False(t, result.success)
	assert.Equal(t, i18nMessage(t, i18n.MsgInvitationCompleteNotAllowed), result.message)
}

func TestCompleteOAuthInvitationBindsWeChatIdentity(t *testing.T) {
	setupInvitationControllerTest(t)
	enableInvitationGate(common.RegisterMethodWeChat)
	codes := createInvitationCodesForTest(t, "wechat-batch", 1)
	flowToken := createInvitationPendingFlowForTest(t, invitationPendingPayload{
		Provider: "wechat", ProviderUserId: "wx-openid-1",
	})

	result := decodeRegisterResult(t, performInvitationComplete(t, `{"flow_token":"`+flowToken+`","invitation_code":"`+codes[0]+`"}`))
	require.True(t, result.success, result.message)

	var user model.User
	require.NoError(t, model.DB.Where("wechat_id = ?", "wx-openid-1").First(&user).Error)
	assert.Equal(t, common.RegisterMethodWeChat, user.RegisterMethod)
	assert.NotZero(t, user.InvitationCodeId)
}

func performRootInvitationRequest(t *testing.T, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if body != "" {
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		c.Request = httptest.NewRequest(method, path, nil)
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: strings.TrimPrefix(path, "/api/invitation/")}}
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	switch {
	case method == http.MethodPost && path == "/api/invitation/":
		AddInvitationCodes(c)
	case method == http.MethodGet && path == "/api/invitation/search":
		SearchInvitationCodes(c)
	case method == http.MethodPut && path == "/api/invitation/":
		UpdateInvitationCode(c)
	case method == http.MethodDelete && strings.HasPrefix(path, "/api/invitation/"):
		DeleteInvitationCode(c)
	case method == http.MethodDelete && path == "/api/invitation/used":
		DeleteUsedInvitationCodes(c)
	default:
		t.Fatalf("unhandled invitation route %s %s", method, path)
	}
	return recorder
}

func TestInvitationAdminAPIManagesCodes(t *testing.T) {
	setupInvitationControllerTest(t)

	// Invalid inputs are rejected: name length, count bounds, past expiry.
	result := decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodPost, "/api/invitation/", `{"name":"","count":1}`))
	assert.False(t, result.success)
	result = decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodPost, "/api/invitation/", `{"name":"batch","count":0}`))
	assert.False(t, result.success)
	result = decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodPost, "/api/invitation/", `{"name":"batch","count":101}`))
	assert.False(t, result.success)
	result = decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodPost, "/api/invitation/", `{"name":"batch","count":1,"expired_time":1}`))
	assert.False(t, result.success)

	// Batch creation returns the plaintext exactly once.
	recorder := performRootInvitationRequest(t, http.MethodPost, "/api/invitation/", `{"name":"admin-batch","count":3}`)
	var createResponse struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &createResponse))
	require.True(t, createResponse.Success, recorder.Body.String())
	require.Len(t, createResponse.Data, 3)

	// The list carries derived state and never the plaintext.
	recorder = performRootInvitationRequest(t, http.MethodGet, "/api/invitation/search", "")
	var searchResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Items []*model.InvitationCode `json:"items"`
			Total int64                   `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &searchResponse))
	require.True(t, searchResponse.Success)
	require.Len(t, searchResponse.Data.Items, 3)
	for _, code := range searchResponse.Data.Items {
		assert.Equal(t, "enabled", code.State)
		assert.NotContains(t, recorder.Body.String(), createResponse.Data[0], "plaintext codes must not appear in list responses")
	}

	// Consumption details surface in the management list after redemption.
	gin.SetMode(gin.TestMode)
	regRecorder := httptest.NewRecorder()
	regContext, _ := gin.CreateTestContext(regRecorder)
	regContext.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"username":"admin-api-user","password":"password123","invitation_code":"`+createResponse.Data[0]+`"}`))
	regContext.Request.Header.Set("Content-Type", "application/json")
	enableInvitationGate()
	Register(regContext)

	recorder = performRootInvitationRequest(t, http.MethodGet, "/api/invitation/search", "")
	require.NoError(t, common.DecodeJson(recorder.Body, &searchResponse))
	var used *model.InvitationCode
	for _, code := range searchResponse.Data.Items {
		if code.State == "used" {
			used = code
		}
	}
	require.NotNil(t, used)
	assert.Equal(t, "admin-api-user", used.UsedUsername)
	assert.Equal(t, common.RegisterMethodPassword, used.UsedRegisterMethod)

	// status_only toggle and single delete.
	toggleBody := `{"id":` + strconv.Itoa(searchResponse.Data.Items[1].Id) + `,"status":2}`
	result = decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodPut, "/api/invitation/", toggleBody))
	require.True(t, result.success, result.message)

	result = decodeRegisterResult(t, performRootInvitationRequest(t, http.MethodDelete, "/api/invitation/"+strconv.Itoa(searchResponse.Data.Items[2].Id), ""))
	require.True(t, result.success, result.message)
}
