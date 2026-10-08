package common

import (
	"slices"
	"strings"
)

// Registration methods that can create accounts. Recorded on every user and
// used to select which entries the invitation gate covers.
const (
	RegisterMethodAdminCreate = "admin_create"
	RegisterMethodPassword    = "password"
	RegisterMethodGitHub      = "github"
	RegisterMethodDiscord     = "discord"
	RegisterMethodOIDC        = "oidc"
	RegisterMethodLinuxDO     = "linuxdo"
	RegisterMethodCustomOAuth = "custom_oauth"
	RegisterMethodWeChat      = "wechat"
)

// InvitationCodeStatus values for model.InvitationCode.
const (
	InvitationCodeStatusEnabled  = 1
	InvitationCodeStatusDisabled = 2
	InvitationCodeStatusUsed     = 3
)

const InvitationCodeMaxLength = 128

// Invitation gate settings. All default off so upgrading changes nothing
// until an administrator turns the gate on in system settings.
var (
	InvitationRequired   = false
	InvitationAffAllowed = true
	// InvitationMethods lists the registration methods the gate covers. The
	// default of every public method means enabling the gate protects all
	// registration entries until the administrator narrows the list.
	InvitationMethods = defaultInvitationMethods()
)

func defaultInvitationMethods() []string {
	return []string{
		RegisterMethodPassword,
		RegisterMethodGitHub,
		RegisterMethodDiscord,
		RegisterMethodOIDC,
		RegisterMethodLinuxDO,
		RegisterMethodCustomOAuth,
		RegisterMethodWeChat,
	}
}

var invitationRegistrationMethods = []string{
	RegisterMethodPassword,
	RegisterMethodGitHub,
	RegisterMethodDiscord,
	RegisterMethodOIDC,
	RegisterMethodLinuxDO,
	RegisterMethodCustomOAuth,
	RegisterMethodWeChat,
}

func IsValidRegisterMethod(method string) bool {
	return slices.Contains(invitationRegistrationMethods, method)
}

// IsInvitationRequiredForMethod reports whether new accounts created through
// the given registration method must present a valid gate code.
func IsInvitationRequiredForMethod(method string) bool {
	return InvitationRequired && slices.Contains(InvitationMethods, method)
}

// AnyInvitationMethodRequired reports whether the gate covers at least one
// public registration method. The registration UI uses it to decide whether to
// show the invitation code input.
func AnyInvitationMethodRequired() bool {
	if !InvitationRequired {
		return false
	}
	for _, method := range InvitationMethods {
		if slices.Contains(invitationRegistrationMethods, method) {
			return true
		}
	}
	return false
}

// ParseInvitationMethodsOption validates and normalizes the methods option
// value. Unknown methods are rejected so a typo cannot silently disable the
// gate for a registration entry.
func ParseInvitationMethodsOption(value string) ([]string, bool) {
	methods := make([]string, 0, len(invitationRegistrationMethods))
	for _, method := range strings.Split(value, ",") {
		method = strings.TrimSpace(method)
		if method == "" {
			continue
		}
		if !slices.Contains(invitationRegistrationMethods, method) {
			return nil, false
		}
		if !slices.Contains(methods, method) {
			methods = append(methods, method)
		}
	}
	return methods, true
}
