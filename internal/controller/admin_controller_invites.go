package controller

import (
	"errors"
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

func (c *AdminController) ListRegistrationInvites(ctx *gin.Context) {
	items, total, err := c.registrationInvites.List(parseQueryInt(ctx, "limit", 50), parseQueryInt(ctx, "offset", 0))
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "邀请码读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, gin.H{"items": items, "total": total})
}

func (c *AdminController) CreateRegistrationInvite(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req struct {
		Note         string `json:"note"`
		MaxUses      int    `json:"max_uses"`
		ValidDays    int    `json:"valid_days"`
		Reason       string `json:"reason"`
		Confirmation string `json:"confirmation"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}
	result, err := c.registrationInvites.Create(adminID, req.Note, req.MaxUses, req.ValidDays, req.Reason, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "邀请码参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "邀请码创建失败")
		return
	}
	common.Success(ctx, http.StatusCreated, result)
}

func (c *AdminController) RevokeRegistrationInvite(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	inviteID, validID := parsePathID(ctx, "id")
	if !validID {
		common.Error(ctx, http.StatusBadRequest, "邀请码 ID 不合法")
		return
	}
	var req adminReasonRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}
	invite, err := c.registrationInvites.Revoke(adminID, inviteID, req.Reason, requestMeta(ctx))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
		case errors.Is(err, repository.ErrInviteNotFound):
			common.Error(ctx, http.StatusNotFound, "邀请码不存在")
		default:
			common.Error(ctx, http.StatusInternalServerError, "邀请码撤销失败")
		}
		return
	}
	common.Success(ctx, http.StatusOK, invite)
}
