package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetAllInvitationCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.GetAllInvitationCodes(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func SearchInvitationCodes(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.SearchInvitationCodes(keyword, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func GetInvitationCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	code, err := model.GetInvitationCodeById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, code)
}

type invitationCodeCreateRequest struct {
	Name        string `json:"name"`
	Count       int    `json:"count"`
	ExpiredTime int64  `json:"expired_time"`
}

// AddInvitationCodes generates one-time registration gate codes. The plaintext
// codes are returned exactly once in this response and cannot be recovered
// later; only their SHA-256 hash and display prefix are stored.
func AddInvitationCodes(c *gin.Context) {
	var request invitationCodeCreateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if utf8.RuneCountInString(request.Name) == 0 || utf8.RuneCountInString(request.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeNameLength)
		return
	}
	if request.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeCountPositive)
		return
	}
	if request.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeCountMax)
		return
	}
	if request.ExpiredTime != 0 && request.ExpiredTime < common.GetTimestamp() {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeExpireInvalid)
		return
	}
	codes, err := model.CreateInvitationCodes(request.Name, request.Count, c.GetInt("id"), request.ExpiredTime)
	if err != nil {
		common.SysError("failed to create invitation codes: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeCreateFailed)
		return
	}
	recordManageAudit(c, "invitation.create", map[string]any{
		"name":  request.Name,
		"count": request.Count,
	})
	common.ApiSuccess(c, codes)
}

type invitationCodeUpdateRequest struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	ExpiredTime int64  `json:"expired_time"`
}

func UpdateInvitationCode(c *gin.Context) {
	statusOnly := c.Query("status_only")
	var request invitationCodeUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	statusOnlyFlag := statusOnly != ""
	if !statusOnlyFlag {
		if request.ExpiredTime != 0 && request.ExpiredTime < common.GetTimestamp() {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeExpireInvalid)
			return
		}
	}
	code, err := model.UpdateInvitationCode(request.Id, request.Name, request.Status, request.ExpiredTime, statusOnlyFlag)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeNotFound)
			return
		}
		if errors.Is(err, model.ErrInvitationCodeUsed) || strings.Contains(err.Error(), "invalid invitation code status") {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeStatusInvalid)
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invitation.update", map[string]any{
		"id":          request.Id,
		"status_only": statusOnlyFlag,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    code,
	})
}

func DeleteUsedInvitationCodes(c *gin.Context) {
	rows, err := model.DeleteUsedInvitationCodes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invitation.delete_used", map[string]any{"count": rows})
	common.ApiSuccess(c, rows)
}

func DeleteInvitationCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteInvitationCodeById(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeNotFound)
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invitation.delete", map[string]any{"id": id})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteInvitationCodeBatch(c *gin.Context) {
	var request struct {
		Ids []int `json:"ids" binding:"required,min=1,max=1000,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	count, err := model.BatchDeleteInvitationCodes(request.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invitation.delete_batch", map[string]any{
		"count": count,
		"total": len(request.Ids),
		"ids":   request.Ids,
	})
	common.ApiSuccess(c, count)
}
