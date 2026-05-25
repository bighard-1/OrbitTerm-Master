package controller

import (
	"errors"
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthController 负责处理认证相关 HTTP 请求。
type AuthController struct {
	authService service.AuthService
}

func NewAuthController(authService service.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

// registerRequest 对应注册接口请求体。
type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
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

	user, err := c.authService.Register(req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "用户名至少 3 位且密码至少 8 位")
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
