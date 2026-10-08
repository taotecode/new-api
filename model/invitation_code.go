package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const invitationCodePrefixLength = 8

var (
	ErrInvitationCodeInvalid  = errors.New("invitation code is invalid")
	ErrInvitationCodeRequired = errors.New("invitation code is required")
	ErrInvitationCodeUsed     = errors.New("invitation code has been used")
)

// InvitationCode is an administrator-issued one-time gate code for new account
// registration. Only the SHA-256 hash and an 8-character display prefix are
// stored; the plaintext code is returned exactly once at creation time.
type InvitationCode struct {
	Id          int            `json:"id"`
	Name        string         `json:"name" gorm:"type:varchar(255);index"`
	CodeHash    string         `json:"-" gorm:"type:char(64);uniqueIndex"`
	CodePrefix  string         `json:"code_prefix" gorm:"type:varchar(8);index"`
	Status      int            `json:"status" gorm:"index"`
	CreatedBy   int            `json:"created_by" gorm:"index"`
	UsedUserId  int            `json:"used_user_id" gorm:"index"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UsedTime    int64          `json:"used_time" gorm:"bigint"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint;index"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`

	// Display-only fields populated after loading; never stored.
	State              string `json:"state" gorm:"-:all"`
	UsedUsername       string `json:"used_username,omitempty" gorm:"-:all"`
	UsedRegisterMethod string `json:"used_register_method,omitempty" gorm:"-:all"`
	CreatedByUsername  string `json:"created_by_username,omitempty" gorm:"-:all"`
}

func NormalizeInvitationCode(rawCode string) string {
	return strings.ToUpper(strings.TrimSpace(rawCode))
}

func HashInvitationCode(rawCode string) string {
	sum := sha256.Sum256([]byte(NormalizeInvitationCode(rawCode)))
	return fmt.Sprintf("%x", sum)
}

// CreateInvitationCodes generates count one-time codes in one transaction and
// returns their plaintext values. The plaintext is shown to the administrator
// only in this response and can never be recovered afterwards.
func CreateInvitationCodes(name string, count int, createdBy int, expiredTime int64) ([]string, error) {
	if count <= 0 || count > 100 {
		return nil, errors.New("invitation code count must be between 1 and 100")
	}
	name = strings.TrimSpace(name)
	codes := make([]string, 0, count)
	err := DB.Transaction(func(tx *gorm.DB) error {
		for range count {
			randomBytes := make([]byte, 20)
			if _, err := rand.Read(randomBytes); err != nil {
				return err
			}
			rawCode := "INV-" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
			invitationCode := &InvitationCode{
				Name:        name,
				CodeHash:    HashInvitationCode(rawCode),
				CodePrefix:  rawCode[:invitationCodePrefixLength],
				Status:      common.InvitationCodeStatusEnabled,
				CreatedBy:   createdBy,
				CreatedTime: common.GetTimestamp(),
				ExpiredTime: expiredTime,
			}
			if err := tx.Create(invitationCode).Error; err != nil {
				return err
			}
			codes = append(codes, rawCode)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func GetAllInvitationCodes(startIdx int, num int) ([]*InvitationCode, int64, error) {
	return SearchInvitationCodes("", "", startIdx, num)
}

func SearchInvitationCodes(keyword string, status string, startIdx int, num int) ([]*InvitationCode, int64, error) {
	query := DB.Model(&InvitationCode{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		normalizedCode := NormalizeInvitationCode(keyword)
		if id, parseErr := strconv.Atoi(keyword); parseErr == nil {
			query = query.Where(
				"id = ? OR name LIKE ? OR code_prefix LIKE ? OR code_hash = ?",
				id,
				keyword+"%",
				normalizedCode+"%",
				HashInvitationCode(normalizedCode),
			)
		} else {
			query = query.Where(
				"name LIKE ? OR code_prefix LIKE ? OR code_hash = ?",
				keyword+"%",
				normalizedCode+"%",
				HashInvitationCode(normalizedCode),
			)
		}
	}

	now := common.GetTimestamp()
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "enabled", strconv.Itoa(common.InvitationCodeStatusEnabled):
		query = query.Where(
			"status = ? AND (expired_time = 0 OR expired_time > ?)",
			common.InvitationCodeStatusEnabled,
			now,
		)
	case "disabled", strconv.Itoa(common.InvitationCodeStatusDisabled):
		query = query.Where("status = ?", common.InvitationCodeStatusDisabled)
	case "used", strconv.Itoa(common.InvitationCodeStatusUsed):
		query = query.Where("status = ?", common.InvitationCodeStatusUsed)
	case "expired":
		query = query.Where(
			"status = ? AND expired_time != 0 AND expired_time <= ?",
			common.InvitationCodeStatusEnabled,
			now,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var codes []*InvitationCode
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&codes).Error; err != nil {
		return nil, 0, err
	}
	if err := populateInvitationCodeDisplayFields(codes); err != nil {
		return nil, 0, err
	}
	return codes, total, nil
}

func GetInvitationCodeById(id int) (*InvitationCode, error) {
	if id <= 0 {
		return nil, errors.New("invalid invitation code id")
	}
	code := &InvitationCode{}
	if err := DB.First(code, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := populateInvitationCodeDisplayFields([]*InvitationCode{code}); err != nil {
		return nil, err
	}
	return code, nil
}

// UpdateInvitationCode applies administrator edits. A used code can no longer
// change because its consumption record is part of the audit trail.
func UpdateInvitationCode(id int, name string, status int, expiredTime int64, statusOnly bool) (*InvitationCode, error) {
	code, err := GetInvitationCodeById(id)
	if err != nil {
		return nil, err
	}
	if statusOnly {
		if status != common.InvitationCodeStatusEnabled && status != common.InvitationCodeStatusDisabled {
			return nil, errors.New("invalid invitation code status")
		}
		if code.Status == common.InvitationCodeStatusUsed {
			return nil, ErrInvitationCodeUsed
		}
		if code.Status == status {
			return code, nil
		}
		result := DB.Model(&InvitationCode{}).
			Where("id = ? AND status IN ?", id, []int{
				common.InvitationCodeStatusEnabled,
				common.InvitationCodeStatusDisabled,
			}).
			Update("status", status)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			latest, latestErr := GetInvitationCodeById(id)
			if latestErr != nil {
				return nil, latestErr
			}
			if latest.Status == common.InvitationCodeStatusUsed {
				return nil, ErrInvitationCodeUsed
			}
			return nil, errors.New("invitation code status changed concurrently")
		}
	} else {
		// A used code can no longer change: its consumption record is part of
		// the audit trail, including the batch name. The conditional update
		// closes the race where a registration consumes the code between the
		// status check above and this write.
		if code.Status == common.InvitationCodeStatusUsed {
			return nil, ErrInvitationCodeUsed
		}
		result := DB.Model(&InvitationCode{}).
			Where("id = ? AND status IN ?", id, []int{
				common.InvitationCodeStatusEnabled,
				common.InvitationCodeStatusDisabled,
			}).
			Updates(map[string]any{
				"name":         strings.TrimSpace(name),
				"expired_time": expiredTime,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			latest, latestErr := GetInvitationCodeById(id)
			if latestErr != nil {
				return nil, latestErr
			}
			if latest.Status == common.InvitationCodeStatusUsed {
				return nil, ErrInvitationCodeUsed
			}
			return nil, errors.New("invitation code status changed concurrently")
		}
	}
	return GetInvitationCodeById(id)
}

func DeleteInvitationCodeById(id int) error {
	if id <= 0 {
		return errors.New("invalid invitation code id")
	}
	result := DB.Delete(&InvitationCode{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteUsedInvitationCodes() (int64, error) {
	result := DB.Where("status = ?", common.InvitationCodeStatusUsed).Delete(&InvitationCode{})
	return result.RowsAffected, result.Error
}

func BatchDeleteInvitationCodes(ids []int) (int64, error) {
	result := DB.Where("id IN ?", ids).Delete(&InvitationCode{})
	return result.RowsAffected, result.Error
}

// ResolveInvitationCodeID maps a plaintext gate code to its database id
// without consuming it. Codes that are missing, disabled, or expired resolve
// to zero so callers can fall back to the completion flow without revealing
// which codes exist.
func ResolveInvitationCodeID(rawCode string) (int, error) {
	normalized := strings.TrimSpace(rawCode)
	if normalized == "" || utf8.RuneCountInString(normalized) > common.InvitationCodeMaxLength {
		return 0, nil
	}
	var code InvitationCode
	err := DB.Select("id").Where("code_hash = ?", HashInvitationCode(normalized)).First(&code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return code.Id, nil
}

// ConsumeInvitationCodeWithTx atomically validates and consumes a plaintext
// gate code inside the caller's transaction. Concurrent registration attempts
// racing for the same code resolve to exactly one winner.
func ConsumeInvitationCodeWithTx(tx *gorm.DB, rawCode string, userID int) (*InvitationCode, error) {
	normalized := strings.TrimSpace(rawCode)
	if normalized == "" {
		return nil, ErrInvitationCodeRequired
	}
	if utf8.RuneCountInString(normalized) > common.InvitationCodeMaxLength {
		return nil, ErrInvitationCodeInvalid
	}
	if tx == nil || userID <= 0 {
		return nil, ErrInvitationCodeInvalid
	}
	code := &InvitationCode{}
	err := lockForUpdate(tx).Where("code_hash = ?", HashInvitationCode(normalized)).First(code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvitationCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	return consumeInvitationCodeRecordWithTx(tx, code, userID)
}

// ConsumeInvitationCodeReferenceWithTx consumes a previously resolved
// invitation code id. It never accepts a plaintext or pre-hashed code.
func ConsumeInvitationCodeReferenceWithTx(tx *gorm.DB, codeID int, userID int) (*InvitationCode, error) {
	if tx == nil || codeID <= 0 || userID <= 0 {
		return nil, ErrInvitationCodeInvalid
	}
	code := &InvitationCode{}
	err := lockForUpdate(tx).Where("id = ?", codeID).First(code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvitationCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	return consumeInvitationCodeRecordWithTx(tx, code, userID)
}

func consumeInvitationCodeRecordWithTx(tx *gorm.DB, code *InvitationCode, userID int) (*InvitationCode, error) {
	switch code.Status {
	case common.InvitationCodeStatusUsed:
		return nil, ErrInvitationCodeUsed
	case common.InvitationCodeStatusDisabled:
		return nil, ErrInvitationCodeInvalid
	case common.InvitationCodeStatusEnabled:
	default:
		return nil, ErrInvitationCodeInvalid
	}
	now := common.GetTimestamp()
	if code.ExpiredTime != 0 && code.ExpiredTime <= now {
		return nil, ErrInvitationCodeInvalid
	}
	// The conditional update is the atomic gate: concurrent consumers race on
	// this single statement and every loser sees zero affected rows.
	result := tx.Model(&InvitationCode{}).
		Where("id = ? AND status = ? AND (expired_time = 0 OR expired_time > ?)", code.Id, common.InvitationCodeStatusEnabled, now).
		Updates(map[string]any{
			"status":       common.InvitationCodeStatusUsed,
			"used_user_id": userID,
			"used_time":    now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrInvitationCodeUsed
	}
	code.Status = common.InvitationCodeStatusUsed
	code.UsedUserId = userID
	code.UsedTime = now
	code.State = "used"
	return code, nil
}

// populateInvitationCodeDisplayFields fills the consumption detail of listed
// codes: derived state plus the consuming and creating users' names. All
// failure paths fall back to empty strings so an unrelated user row problem
// never breaks the management list.
func populateInvitationCodeDisplayFields(codes []*InvitationCode) error {
	if len(codes) == 0 {
		return nil
	}
	userIDs := make([]int, 0, len(codes)*2)
	now := common.GetTimestamp()
	for _, code := range codes {
		switch {
		case code.Status == common.InvitationCodeStatusUsed:
			code.State = "used"
		case code.Status == common.InvitationCodeStatusDisabled:
			code.State = "disabled"
		case code.ExpiredTime != 0 && code.ExpiredTime <= now:
			code.State = "expired"
		default:
			code.State = "enabled"
		}
		if code.UsedUserId > 0 {
			userIDs = append(userIDs, code.UsedUserId)
		}
		if code.CreatedBy > 0 {
			userIDs = append(userIDs, code.CreatedBy)
		}
	}
	if len(userIDs) == 0 {
		return nil
	}
	type invitationCodeUser struct {
		Id             int
		Username       string
		RegisterMethod string
	}
	var users []invitationCodeUser
	if err := DB.Model(&User{}).Select("id", "username", "register_method").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return err
	}
	usernames := make(map[int]invitationCodeUser, len(users))
	for _, user := range users {
		usernames[user.Id] = user
	}
	for _, code := range codes {
		if used := usernames[code.UsedUserId]; used.Id != 0 {
			code.UsedUsername = used.Username
			code.UsedRegisterMethod = used.RegisterMethod
		}
		if creator := usernames[code.CreatedBy]; creator.Id != 0 {
			code.CreatedByUsername = creator.Username
		}
	}
	return nil
}

// GetInvitationCodeSummaries returns the display prefix and batch name of the
// given invitation code ids, keyed by id, for user detail views.
func GetInvitationCodeSummaries(ids []int) (map[int]*InvitationCode, error) {
	summaries := make(map[int]*InvitationCode, len(ids))
	if len(ids) == 0 {
		return summaries, nil
	}
	var codes []*InvitationCode
	if err := DB.Select("id", "name", "code_prefix", "status", "created_time", "used_time", "expired_time").Where("id IN ?", ids).Find(&codes).Error; err != nil {
		return nil, err
	}
	for _, code := range codes {
		summaries[code.Id] = code
	}
	return summaries, nil
}
