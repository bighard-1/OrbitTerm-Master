package controller

import (
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AdminController struct {
	auditService service.AdminAuditService
}

func NewAdminController(auditService service.AdminAuditService) *AdminController {
	return &AdminController{auditService: auditService}
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
