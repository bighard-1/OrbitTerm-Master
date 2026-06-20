package controller

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

func extractContextUint(ctx *gin.Context, key string) (uint, bool) {
	value, exists := ctx.Get(key)
	if !exists {
		return 0, false
	}
	typed, ok := value.(uint)
	if !ok || typed == 0 {
		return 0, false
	}
	return typed, true
}

func parsePathID(ctx *gin.Context, key string) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param(key), 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return uint(value), true
}

func parseQueryInt(ctx *gin.Context, key string, fallback int) int {
	raw := ctx.Query(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseQueryUint(ctx *gin.Context, key string) uint {
	raw := ctx.Query(key)
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return uint(value)
}

func requestMeta(ctx *gin.Context) service.AdminRequestMeta {
	return service.AdminRequestMeta{
		IPAddress: ctx.ClientIP(),
		UserAgent: ctx.Request.UserAgent(),
	}
}

func snapshotPresence(before, after string) gin.H {
	return gin.H{
		"before": strings.TrimSpace(before) != "",
		"after":  strings.TrimSpace(after) != "",
	}
}

func validateReason(ctx *gin.Context, reason string) bool {
	if len(strings.TrimSpace(reason)) < 2 {
		common.Error(ctx, http.StatusBadRequest, "管理操作原因必填，且至少 2 个字符")
		return false
	}
	return true
}

func validateHighRiskRequest(ctx *gin.Context, reason, confirmation string) bool {
	if !validateReason(ctx, reason) {
		return false
	}
	if strings.TrimSpace(confirmation) != "CONFIRM" {
		common.Error(ctx, http.StatusBadRequest, "高危操作需要二次确认，请传入 confirmation=CONFIRM")
		return false
	}
	return true
}

func (c *AdminController) validateBootstrapToken(ctx *gin.Context) bool {
	if c.adminBootstrapToken == "" {
		common.Error(ctx, http.StatusServiceUnavailable, "管理员初始化令牌未配置")
		return false
	}

	token := ctx.GetHeader("X-Orbit-Admin-Bootstrap-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(c.adminBootstrapToken)) != 1 {
		common.Error(ctx, http.StatusForbidden, "管理员初始化令牌无效")
		return false
	}
	return true
}

func writeAdminAuthError(ctx *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
	case errors.Is(err, service.ErrUserAlreadyExists):
		common.Error(ctx, http.StatusConflict, "用户名已存在")
	case errors.Is(err, service.ErrAdminAlreadyInitialized):
		common.Error(ctx, http.StatusConflict, "管理端已初始化")
	case errors.Is(err, service.ErrInvalidCredential):
		common.Error(ctx, http.StatusUnauthorized, "用户名或密码错误")
	case errors.Is(err, service.ErrAdminPermissionDenied):
		common.Error(ctx, http.StatusForbidden, "该账号无管理端权限")
	case errors.Is(err, service.ErrAccountBanned):
		common.Error(ctx, http.StatusForbidden, "账号已被封禁")
	case errors.Is(err, service.ErrAccountDeleted):
		common.Error(ctx, http.StatusForbidden, "账号已注销")
	default:
		common.Error(ctx, http.StatusInternalServerError, fallback)
	}
}

func writeAdminUserError(ctx *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
	case errors.Is(err, service.ErrAdminInvalidAction):
		common.Error(ctx, http.StatusBadRequest, "不允许执行该管理操作")
	case errors.Is(err, service.ErrAdminReasonRequired):
		common.Error(ctx, http.StatusBadRequest, "管理操作原因必填")
	case errors.Is(err, service.ErrAdminTargetNotFound):
		common.Error(ctx, http.StatusNotFound, "目标用户不存在")
	default:
		common.Error(ctx, http.StatusInternalServerError, fallback)
	}
}

func toAdminUserResponse(user *model.User) gin.H {
	return gin.H{
		"id":                   user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"status":               user.Status,
		"is_banned":            user.IsBanned,
		"ban_until":            user.BanUntil,
		"ban_reason":           user.BanReason,
		"banned_at":            user.BannedAt,
		"banned_by":            user.BannedBy,
		"is_deleted":           user.IsDeleted,
		"deleted_at":           user.DeletedAt,
		"must_change_password": user.MustChangePassword,
		"token_version":        user.TokenVersion,
		"last_login_at":        user.LastLoginAt,
		"last_login_ip":        user.LastLoginIP,
		"created_at":           user.CreatedAt,
		"updated_at":           user.UpdatedAt,
	}
}
