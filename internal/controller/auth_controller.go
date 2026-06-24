package controller

import (
	"errors"
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthController 负责处理认证相关 HTTP 请求。
type AuthController struct {
	authService    service.AuthService
	recoveryPolicy service.RecoveryPolicyReader
	securityPolicy service.SecurityPolicyProvider
}

func NewAuthController(authService service.AuthService, recoveryPolicy service.RecoveryPolicyReader, securityPolicy service.SecurityPolicyProvider) *AuthController {
	return &AuthController{
		authService:    authService,
		recoveryPolicy: recoveryPolicy,
		securityPolicy: securityPolicy,
	}
}

// registerRequest 对应注册接口请求体。
type registerRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	InviteCode string `json:"invite_code" binding:"required"`
}

// loginRequest 对应登录接口请求体。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register godoc
// @Summary 用户注册
// @Description 创建新用户，密码将使用 Argon2id 算法进行哈希。
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body registerRequest true "注册信息"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v1/auth/register [post]
func (c *AuthController) Register(ctx *gin.Context) {
	var req registerRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	user, err := c.authService.Register(req.Username, req.Password, req.InviteCode)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "注册信息不符合当前安全策略")
		case errors.Is(err, service.ErrEmailDomainDenied):
			common.Error(ctx, http.StatusBadRequest, "请使用管理员允许的邮箱域名注册")
		case errors.Is(err, service.ErrWeakPassword):
			common.Error(ctx, http.StatusBadRequest, "密码至少 12 位，并包含大写字母、小写字母、数字和特殊字符")
		case errors.Is(err, service.ErrInviteRequired):
			common.Error(ctx, http.StatusBadRequest, "注册需要邀请码")
		case errors.Is(err, service.ErrInviteInvalid):
			common.Error(ctx, http.StatusForbidden, "邀请码无效、已过期或已用完")
		case errors.Is(err, service.ErrRegistrationClosed):
			common.Error(ctx, http.StatusForbidden, "注册已关闭，请联系管理员")
		case errors.Is(err, service.ErrUserAlreadyExists):
			common.Error(ctx, http.StatusConflict, "用户名已存在")
		default:
			common.Error(ctx, http.StatusInternalServerError, "注册失败")
		}
		return
	}

	common.Success(ctx, http.StatusCreated, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"created_at": user.CreatedAt,
	})
}

func (c *AuthController) RegistrationPolicy(ctx *gin.Context) {
	policy := model.DefaultSecurityPolicy()
	if c.securityPolicy != nil {
		configured, err := c.securityPolicy.GetSecurityPolicy()
		if err != nil {
			common.Error(ctx, http.StatusInternalServerError, "注册策略读取失败")
			return
		}
		policy = configured
	}
	policy.Normalize()
	common.Success(ctx, http.StatusOK, gin.H{
		"invitation_required":        policy.InvitationRequired,
		"min_password_length":        policy.MinPasswordLength,
		"strict_password_complexity": policy.StrictPasswordComplexity,
	})
}

// Login godoc
// @Summary 用户登录
// @Description 校验用户凭证并返回 JWT。
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body loginRequest true "登录信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/v1/auth/login [post]
func (c *AuthController) Login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	pair, err := c.authService.Login(req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "用户名和密码不能为空")
		case errors.Is(err, service.ErrInvalidCredential):
			common.Error(ctx, http.StatusUnauthorized, "用户名或密码错误")
		case errors.Is(err, service.ErrAccountBanned):
			common.Error(ctx, http.StatusForbidden, "账号已被封禁，请联系管理员")
		case errors.Is(err, service.ErrAccountDeleted):
			common.Error(ctx, http.StatusForbidden, "账号已注销")
		default:
			common.Error(ctx, http.StatusInternalServerError, "登录失败")
		}
		return
	}

	common.Success(ctx, http.StatusOK, gin.H{
		"access_token":               pair.AccessToken,
		"refresh_token":              pair.RefreshToken,
		"token":                      pair.AccessToken,
		"type":                       "Bearer",
		"expires_in_seconds":         pair.AccessExpiresInSeconds,
		"refresh_expires_in_seconds": pair.RefreshExpiresInSeconds,
	})
}

// Refresh godoc
// @Summary 刷新访问令牌
// @Description 使用 refresh token 申请新的 access token 与 refresh token（轮换）。
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body refreshRequest true "刷新令牌信息"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/v1/auth/refresh [post]
func (c *AuthController) Refresh(ctx *gin.Context) {
	var req refreshRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	pair, err := c.authService.Refresh(req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "refresh_token 不能为空")
		case errors.Is(err, service.ErrInvalidCredential):
			common.Error(ctx, http.StatusUnauthorized, "refresh token 无效或已过期")
		case errors.Is(err, service.ErrAccountBanned):
			common.Error(ctx, http.StatusForbidden, "账号已被封禁，请联系管理员")
		case errors.Is(err, service.ErrAccountDeleted):
			common.Error(ctx, http.StatusForbidden, "账号已注销")
		default:
			common.Error(ctx, http.StatusInternalServerError, "刷新令牌失败")
		}
		return
	}

	common.Success(ctx, http.StatusOK, gin.H{
		"access_token":               pair.AccessToken,
		"refresh_token":              pair.RefreshToken,
		"type":                       "Bearer",
		"expires_in_seconds":         pair.AccessExpiresInSeconds,
		"refresh_expires_in_seconds": pair.RefreshExpiresInSeconds,
	})
}

func (c *AuthController) RecoveryInfo(ctx *gin.Context) {
	if c.recoveryPolicy == nil {
		policy := service.DefaultPublicRecoveryInfo()
		common.Success(ctx, http.StatusOK, policy)
		return
	}

	policy, err := c.recoveryPolicy.GetRecoveryPolicy()
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "恢复策略读取失败")
		return
	}
	common.Success(ctx, http.StatusOK, service.ToPublicRecoveryInfo(policy))
}
