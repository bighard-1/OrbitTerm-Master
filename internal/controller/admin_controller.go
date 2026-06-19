package controller

import (
	"errors"
	"net/http"
	"strconv"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AdminController struct {
	auditService service.AdminAuditService
	userService  service.AdminUserService
}

func NewAdminController(auditService service.AdminAuditService, userService service.AdminUserService) *AdminController {
	return &AdminController{
		auditService: auditService,
		userService:  userService,
	}
}

func (c *AdminController) Me(ctx *gin.Context) {
	userID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	username, _ := ctx.Get(middleware.ContextUsernameKey)
	role, _ := ctx.Get(middleware.ContextUserRoleKey)

	_ = c.auditService.Record(service.AdminAuditEntry{
		AdminUserID:  userID,
		Action:       model.AuditActionAdminMe,
		ResourceType: "admin",
		ResourceID:   "me",
		IPAddress:    ctx.ClientIP(),
		UserAgent:    ctx.Request.UserAgent(),
	})

	common.Success(ctx, http.StatusOK, gin.H{
		"id":       userID,
		"username": username,
		"role":     role,
	})
}

func (c *AdminController) AuditLogs(ctx *gin.Context) {
	logs, err := c.auditService.ListRecent(50)
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "审计日志读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, gin.H{"items": logs})
}

func (c *AdminController) ListUsers(ctx *gin.Context) {
	limit := parseQueryInt(ctx, "limit", 50)
	offset := parseQueryInt(ctx, "offset", 0)

	users, total, err := c.userService.ListUsers(service.AdminUserListFilter{
		Query:  ctx.Query("q"),
		Role:   ctx.Query("role"),
		Status: ctx.Query("status"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "用户列表读取失败")
		return
	}

	items := make([]gin.H, 0, len(users))
	for i := range users {
		items = append(items, toAdminUserResponse(&users[i]))
	}
	common.Success(ctx, http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (c *AdminController) GetUser(ctx *gin.Context) {
	userID, ok := parsePathID(ctx, "id")
	if !ok {
		common.Error(ctx, http.StatusBadRequest, "用户 ID 非法")
		return
	}

	user, err := c.userService.GetUser(userID)
	if err != nil {
		writeAdminUserError(ctx, err, "用户详情读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

type banUserRequest struct {
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	Reason          string `json:"reason"`
}

func (c *AdminController) BanUser(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	userID, ok := parsePathID(ctx, "id")
	if !ok {
		common.Error(ctx, http.StatusBadRequest, "用户 ID 非法")
		return
	}

	var req banUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.BanUser(adminID, userID, req.DurationMinutes, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "封禁用户失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

type unbanUserRequest struct {
	Reason string `json:"reason"`
}

func (c *AdminController) UnbanUser(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	userID, ok := parsePathID(ctx, "id")
	if !ok {
		common.Error(ctx, http.StatusBadRequest, "用户 ID 非法")
		return
	}

	var req unbanUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.UnbanUser(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "解封用户失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

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

func requestMeta(ctx *gin.Context) service.AdminRequestMeta {
	return service.AdminRequestMeta{
		IPAddress: ctx.ClientIP(),
		UserAgent: ctx.Request.UserAgent(),
	}
}

func writeAdminUserError(ctx *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
	case errors.Is(err, service.ErrAdminInvalidAction):
		common.Error(ctx, http.StatusBadRequest, "不允许执行该管理操作")
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
