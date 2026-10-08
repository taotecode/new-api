package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvitationCodeFixture(t *testing.T) (*InvitationCode, string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&InvitationCode{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCode{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCode{}).Error)
	})
	codes, err := CreateInvitationCodes("fixture", 1, 0, 0)
	require.NoError(t, err)
	require.Len(t, codes, 1)
	var code InvitationCode
	require.NoError(t, DB.Where("code_hash = ?", HashInvitationCode(codes[0])).First(&code).Error)
	return &code, codes[0]
}

func setupInvitationConsumers(t *testing.T, count int) []int {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&User{}))
	users := make([]int, count)
	for i := range users {
		user := &User{
			Username: fmt.Sprintf("invitation-user-%d", i),
			Password: "password",
			AffCode:  fmt.Sprintf("inv%d", i),
			Status:   common.UserStatusEnabled,
		}
		require.NoError(t, DB.Create(user).Error)
		users[i] = user.Id
	}
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE username LIKE 'invitation-user-%'")
	})
	return users
}

func TestCreateInvitationCodesStoresHashAndPrefixOnly(t *testing.T) {
	code, plaintext := setupInvitationCodeFixture(t)

	assert.Equal(t, common.InvitationCodeStatusEnabled, code.Status)
	assert.Len(t, code.CodeHash, 64)
	assert.Equal(t, strings.ToLower(code.CodeHash), code.CodeHash)
	assert.Len(t, code.CodePrefix, 8)
	assert.True(t, strings.HasPrefix(plaintext, "INV-"))
	assert.NotContains(t, code.CodeHash, plaintext)
}

func TestConsumeInvitationCodeIsSingleWinnerUnderConcurrency(t *testing.T) {
	code, plaintext := setupInvitationCodeFixture(t)
	users := setupInvitationConsumers(t, 8)

	raceConsume := func(rawCode string) (winners, rejections int) {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, userID := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := DB.Transaction(func(tx *gorm.DB) error {
					_, err := ConsumeInvitationCodeWithTx(tx, rawCode, userID)
					return err
				})
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					winners++
				} else {
					rejections++
				}
			}()
		}
		wg.Wait()
		return winners, rejections
	}

	// Unknown codes never consume.
	winners, rejections := raceConsume(code.CodePrefix + "-REST-NOT-USED")
	assert.Equal(t, 0, winners)
	assert.Equal(t, len(users), rejections)

	// Exactly one concurrent consumer may redeem a one-time code.
	winners, rejections = raceConsume(plaintext)
	assert.Equal(t, 1, winners)
	assert.Equal(t, len(users)-1, rejections)

	var consumed InvitationCode
	require.NoError(t, DB.Where("id = ?", code.Id).First(&consumed).Error)
	assert.Equal(t, common.InvitationCodeStatusUsed, consumed.Status)
	assert.NotZero(t, consumed.UsedUserId)
	assert.NotZero(t, consumed.UsedTime)
}

func TestConsumeInvitationCodeRejectsInvalidStates(t *testing.T) {
	code, plaintext := setupInvitationCodeFixture(t)
	users := setupInvitationConsumers(t, 1)

	consume := func(rawCode string) error {
		return DB.Transaction(func(tx *gorm.DB) error {
			_, err := ConsumeInvitationCodeWithTx(tx, rawCode, users[0])
			return err
		})
	}

	assert.ErrorIs(t, consume("INV-DOES-NOT-EXIST"), ErrInvitationCodeInvalid)
	assert.ErrorIs(t, consume(""), ErrInvitationCodeRequired)

	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Update("status", common.InvitationCodeStatusDisabled).Error)
	assert.ErrorIs(t, consume(plaintext), ErrInvitationCodeInvalid)

	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Updates(map[string]any{
		"status":       common.InvitationCodeStatusEnabled,
		"expired_time": common.GetTimestamp() - 10,
	}).Error)
	assert.ErrorIs(t, consume(plaintext), ErrInvitationCodeInvalid)

	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Update("expired_time", 0).Error)
	require.NoError(t, consume(plaintext))
	assert.ErrorIs(t, consume(plaintext), ErrInvitationCodeUsed)
}

func TestInvitationCodeNormalizationAndResolve(t *testing.T) {
	code, plaintext := setupInvitationCodeFixture(t)

	// Case and surrounding whitespace are normalized before hashing.
	resolved, err := ResolveInvitationCodeID("  " + strings.ToLower(plaintext) + " \t")
	require.NoError(t, err)
	assert.Equal(t, code.Id, resolved)

	resolved, err = ResolveInvitationCodeID("INV-UNKNOWN-CODE")
	require.NoError(t, err)
	assert.Zero(t, resolved)
}

func TestInvitationCodeLifecycleManagement(t *testing.T) {
	code, _ := setupInvitationCodeFixture(t)

	// Search by name keyword and status filter.
	codes, total, err := SearchInvitationCodes("fixture", "", 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, codes, 1)
	assert.Equal(t, "enabled", codes[0].State)

	codes, _, err = SearchInvitationCodes("", "enabled", 0, 10)
	require.NoError(t, err)
	require.Len(t, codes, 1)

	// status_only updates flip enabled/disabled.
	updated, err := UpdateInvitationCode(code.Id, "", common.InvitationCodeStatusDisabled, 0, true)
	require.NoError(t, err)
	assert.Equal(t, common.InvitationCodeStatusDisabled, updated.Status)

	// Setting a code directly to "used" is never a valid status transition.
	_, err = UpdateInvitationCode(code.Id, "", common.InvitationCodeStatusUsed, 0, true)
	assert.Error(t, err)

	// Full updates rename and adjust expiry while the code is not consumed.
	updated, err = UpdateInvitationCode(code.Id, "renamed-batch", 0, 0, false)
	require.NoError(t, err)
	assert.Equal(t, "renamed-batch", updated.Name)

	// A consumed code can no longer change status or be edited at all.
	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Update("status", common.InvitationCodeStatusUsed).Error)
	_, err = UpdateInvitationCode(code.Id, "", common.InvitationCodeStatusEnabled, 0, true)
	assert.ErrorIs(t, err, ErrInvitationCodeUsed)
	_, err = UpdateInvitationCode(code.Id, "rename-used-batch", 0, 0, false)
	assert.ErrorIs(t, err, ErrInvitationCodeUsed, "used codes keep their consumption record intact")

	// Delete used codes only removes consumed rows.
	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Update("status", common.InvitationCodeStatusUsed).Error)
	rows, err := DeleteUsedInvitationCodes()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)
	_, err = GetInvitationCodeById(code.Id)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
