package controller

import (
	"crypto/subtle"
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
	auditService        service.AdminAuditService
	userService         service.AdminUserService
	adminAuthService    service.AdminAuthService
	adminBootstrapToken string
}

func NewAdminController(
	auditService service.AdminAuditService,
	userService service.AdminUserService,
	adminAuthService service.AdminAuthService,
	adminBootstrapToken string,
) *AdminController {
	return &AdminController{
		auditService:        auditService,
		userService:         userService,
		adminAuthService:    adminAuthService,
		adminBootstrapToken: adminBootstrapToken,
	}
}

func (c *AdminController) BootstrapStatus(ctx *gin.Context) {
	status, err := c.adminAuthService.BootstrapStatus()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "管理端初始化状态读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, status)
}

type bootstrapSuperAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (c *AdminController) BootstrapSuperAdmin(ctx *gin.Context) {
	if !c.validateBootstrapToken(ctx) {
		return
	}

	var req bootstrapSuperAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.adminAuthService.BootstrapSuperAdmin(req.Username, req.Password, requestMeta(ctx))
	if err != nil {
		writeAdminAuthError(ctx, err, "首个管理员创建失败")
		return
	}
	common.Success(ctx, http.StatusCreated, toAdminUserResponse(user))
}

type adminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (c *AdminController) Login(ctx *gin.Context) {
	var req adminLoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	pair, err := c.adminAuthService.Login(req.Username, req.Password, requestMeta(ctx))
	if err != nil {
		writeAdminAuthError(ctx, err, "管理员登录失败")
		return
	}
	common.Success(ctx, http.StatusOK, pair)
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

type resetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
	Reason      string `json:"reason"`
}

func (c *AdminController) ResetPassword(ctx *gin.Context) {
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

	var req resetPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.ResetPassword(adminID, userID, req.NewPassword, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "重置登录密码失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

type adminReasonRequest struct {
	Reason string `json:"reason"`
}

func (c *AdminController) ForceLogout(ctx *gin.Context) {
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

	var req adminReasonRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.ForceLogout(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "强制下线失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

func (c *AdminController) SoftDeleteUser(ctx *gin.Context) {
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

	var req adminReasonRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.SoftDeleteUser(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "软删除用户失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

func (c *AdminController) RestoreUser(ctx *gin.Context) {
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

	var req adminReasonRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.userService.RestoreUser(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "恢复用户失败")
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
