package controller

import (
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

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

type createManagedUserRequest struct {
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required"`
	Role         string `json:"role" binding:"required"`
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
}

func (c *AdminController) CreateManagedUser(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req createManagedUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}

	user, err := c.userService.CreateManagedUser(adminID, req.Username, req.Password, req.Role, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "创建用户失败")
		return
	}
	common.Success(ctx, http.StatusCreated, toAdminUserResponse(user))
}

type updateUserRoleRequest struct {
	Role         string `json:"role" binding:"required"`
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
}

func (c *AdminController) UpdateUserRole(ctx *gin.Context) {
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

	var req updateUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}

	user, err := c.userService.UpdateUserRole(adminID, userID, req.Role, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "调整用户角色失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

type banUserRequest struct {
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	Reason          string `json:"reason"`
	Confirmation    string `json:"confirmation"`
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
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
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
	if !validateReason(ctx, req.Reason) {
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
	NewPassword  string `json:"new_password" binding:"required"`
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
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
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
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
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
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
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}

	user, err := c.userService.ForceLogout(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "强制下线失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

func (c *AdminController) ForceLogoutRegularUsers(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
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

	result, err := c.userService.ForceLogoutRegularUsers(adminID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "普通用户全部下线失败")
		return
	}
	common.Success(ctx, http.StatusOK, result)
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
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
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
	if !validateReason(ctx, req.Reason) {
		return
	}

	user, err := c.userService.RestoreUser(adminID, userID, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "恢复用户失败")
		return
	}
	common.Success(ctx, http.StatusOK, toAdminUserResponse(user))
}

type scanExpiredBansRequest struct {
	Limit        int    `json:"limit"`
	Reason       string `json:"reason"`
	Confirmation string `json:"confirmation"`
}

func (c *AdminController) ScanExpiredBans(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req scanExpiredBansRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}

	result, err := c.userService.ScanExpiredBans(adminID, req.Limit, req.Reason, requestMeta(ctx))
	if err != nil {
		writeAdminUserError(ctx, err, "过期封禁扫描失败")
		return
	}
	common.Success(ctx, http.StatusOK, result)
}
