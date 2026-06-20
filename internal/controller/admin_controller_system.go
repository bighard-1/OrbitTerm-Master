package controller

import (
	"errors"
	"net/http"
	"runtime"
	"strings"
	"time"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

func (c *AdminController) GetAssetDeletionPolicy(ctx *gin.Context) {
	policy, err := c.assetDeletionPolicy.GetAssetDeletionPolicy()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "资产删除策略读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) UpdateAssetDeletionPolicy(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req struct {
		service.AssetDeletionPolicyUpdate
		Confirmation string `json:"confirmation"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}
	policy, err := c.assetDeletionPolicy.UpdateAssetDeletionPolicy(adminID, req.AssetDeletionPolicyUpdate, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "资产删除策略更新失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) CleanupExpiredAssetTrash(ctx *gin.Context) {
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
	result, err := c.assetDeletionPolicy.CleanupExpiredAssets(adminID, req.Reason, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "过期资产清理失败")
		return
	}
	common.Success(ctx, http.StatusOK, result)
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
	limit := parseQueryInt(ctx, "limit", 50)
	offset := parseQueryInt(ctx, "offset", 0)
	adminUserID := parseQueryUint(ctx, "admin_user_id")
	targetUserID := parseQueryUint(ctx, "target_user_id")

	logs, total, err := c.auditService.List(service.AdminAuditListFilter{
		Action:       ctx.Query("action"),
		ResourceType: ctx.Query("resource_type"),
		AdminUserID:  adminUserID,
		TargetUserID: targetUserID,
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "审计日志读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, gin.H{
		"items":  logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (c *AdminController) DashboardOverview(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	overview, err := c.dashboard.Overview(adminID, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "仪表盘数据读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, overview)
}

func (c *AdminController) GetSecurityPolicy(ctx *gin.Context) {
	policy, err := c.securityPolicy.GetSecurityPolicy()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "安全策略读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) UpdateSecurityPolicy(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req service.SecurityPolicyUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	policy, err := c.securityPolicy.UpdateSecurityPolicy(adminID, req, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "安全策略更新失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) GetRecoveryPolicy(ctx *gin.Context) {
	policy, err := c.recoveryPolicy.GetRecoveryPolicy()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "恢复策略读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) UpdateRecoveryPolicy(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req service.RecoveryPolicyUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	policy, err := c.recoveryPolicy.UpdateRecoveryPolicy(adminID, req, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "恢复策略更新失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) GetAuditPolicy(ctx *gin.Context) {
	policy, err := c.auditPolicy.GetAuditPolicy()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "审计策略读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) UpdateAuditPolicy(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req service.AuditPolicyUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	policy, err := c.auditPolicy.UpdateAuditPolicy(adminID, req, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "审计策略更新失败")
		return
	}
	common.Success(ctx, http.StatusOK, policy)
}

func (c *AdminController) CleanupAuditLogs(ctx *gin.Context) {
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

	result, err := c.auditPolicy.CleanupExpiredAuditLogs(adminID, req.Reason, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrAdminReasonRequired) {
			common.Error(ctx, http.StatusBadRequest, "清理原因必填")
			return
		}
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "审计日志清理失败")
		return
	}
	common.Success(ctx, http.StatusOK, result)
}

func (c *AdminController) BackupReadiness(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	report, err := c.backupReadiness.GetReadiness(adminID, requestMeta(ctx))
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "备份自检失败")
		return
	}
	common.Success(ctx, http.StatusOK, report)
}

func (c *AdminController) Diagnostics(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	meta := requestMeta(ctx)
	backup, err := c.backupReadiness.GetReadiness(adminID, meta)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "请求参数不合法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "诊断包生成失败")
		return
	}

	recentAudits, err := c.auditService.ListRecent(20)
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "诊断包审计摘要读取失败")
		return
	}

	auditSummary := make([]gin.H, 0, len(recentAudits))
	for _, log := range recentAudits {
		auditSummary = append(auditSummary, gin.H{
			"id":              log.ID,
			"admin_user_id":   log.AdminUserID,
			"target_user_id":  log.TargetUserID,
			"action":          log.Action,
			"resource_type":   log.ResourceType,
			"resource_id":     log.ResourceID,
			"created_at":      log.CreatedAt,
			"reason_present":  strings.TrimSpace(log.Reason) != "",
			"snapshot_fields": snapshotPresence(log.BeforeSnapshot, log.AfterSnapshot),
		})
	}

	report := gin.H{
		"generated_at": time.Now().UTC(),
		"runtime": gin.H{
			"go_version": runtime.Version(),
			"go_os":      runtime.GOOS,
			"go_arch":    runtime.GOARCH,
			"gin_mode":   gin.Mode(),
		},
		"backup_readiness": backup,
		"recent_audit_summary": gin.H{
			"limit": 20,
			"items": auditSummary,
		},
		"redaction_policy": []string{
			"诊断包不包含 JWT_SECRET、Refresh Token、数据库密码、ADMIN_BOOTSTRAP_TOKEN 原文。",
			"诊断包不包含用户主密码、主密码派生密钥、服务器密码、私钥或加密资产明文。",
			"环境变量仅输出是否配置、强度判断与脱敏值。",
		},
		"operator_next_steps": []string{
			"若 backup_readiness.ready 为 false，优先处理 warnings 中的配置或数据库问题。",
			"生产环境请将完整密钥保存在独立密码库，诊断包只适合提交给开发/运维定位问题。",
			"导出诊断包后可同时导出审计日志 JSON，交叉排查具体管理员操作。",
		},
	}

	_ = c.auditService.Record(service.AdminAuditEntry{
		AdminUserID:  adminID,
		Action:       model.AuditActionSystemDiagnosticsExport,
		ResourceType: "system",
		ResourceID:   "diagnostics",
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	})

	common.Success(ctx, http.StatusOK, report)
}
