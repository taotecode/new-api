package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// ErrInvitationRejected deliberately hides whether a gate code was missing,
// unknown, expired, disabled, or already used, so registration responses do
// not disclose which codes exist.
var ErrInvitationRejected = errors.New("invitation code is invalid or has been used")

// RegistrationInvitation is the resolved form of a user-submitted gate code.
// Exactly one of CodeID or InviterID is set: a registration code consumes a
// one-time invitation_codes row, an aff code only binds the inviter.
type RegistrationInvitation struct {
	CodeID    int
	InviterID int
}

// ResolveRegistrationInvitation maps a user-submitted gate code to a
// registration code id or an aff inviter id without consuming anything.
// Unknown codes resolve to zero values; callers decide whether to reject the
// registration or start the invitation completion flow.
func ResolveRegistrationInvitation(rawCode string) RegistrationInvitation {
	codeID, err := model.ResolveInvitationCodeID(rawCode)
	if err != nil || codeID > 0 {
		return RegistrationInvitation{CodeID: codeID}
	}
	if common.InvitationAffAllowed {
		inviterID, err := model.GetUserIdByAffCode(rawCode)
		if err == nil && inviterID > 0 {
			return RegistrationInvitation{InviterID: inviterID}
		}
	}
	return RegistrationInvitation{}
}

// ConsumeRegistrationInvitationWithTx validates and consumes the gate code
// inside the caller's transaction. The userID must already exist in the same
// transaction because consuming a registration code records who redeemed it.
func ConsumeRegistrationInvitationWithTx(tx *gorm.DB, rawCode string, userID int) (RegistrationInvitation, error) {
	resolved := ResolveRegistrationInvitation(rawCode)
	if resolved.CodeID > 0 {
		if _, err := model.ConsumeInvitationCodeReferenceWithTx(tx, resolved.CodeID, userID); err != nil {
			if errors.Is(err, model.ErrInvitationCodeUsed) {
				return RegistrationInvitation{}, ErrInvitationRejected
			}
			return RegistrationInvitation{}, err
		}
		return RegistrationInvitation{CodeID: resolved.CodeID}, nil
	}
	if resolved.InviterID > 0 {
		return RegistrationInvitation{InviterID: resolved.InviterID}, nil
	}
	return RegistrationInvitation{}, ErrInvitationRejected
}
