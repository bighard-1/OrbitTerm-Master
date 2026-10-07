package middleware

import (
	"net/http"
	"strings"
	"time"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/repository"
	"orbitterm-server/internal/utils"

	"github.com/gin-gonic/gin"
)

const (
	// ContextUserIDKey 是 Gin 上下文中存放用户 ID 的键。
	ContextUserIDKey = "auth_user_id"
	// ContextUsernameKey 是 Gin 上下文中存放用户名的键。
	ContextUsernameKey  = "auth_username"
	ContextSessionIDKey = "auth_session_id"
	ContextAuthTimeKey  = "auth_time"
)

// JWTAuthMiddleware 校验 Authorization: Bearer <Token>。
// 本中间件同时检查账号封禁/注销/TokenVersion，确保管理端后续操作可实时影响已登录用户。
func JWTAuthMiddleware(jwtManager *utils.JWTManager, userRepo repository.UserRepository) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := strings.TrimSpace(ctx.GetHeader("Authorization"))
		if authHeader == "" {
			common.Error(ctx, http.StatusUnauthorized, "缺少 Authorization 头")
			ctx.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			common.Error(ctx, http.StatusUnauthorized, "Authorization 格式错误，应为 Bearer <Token>")
			ctx.Abort()
			return
		}

		claims, err := jwtManager.ParseAndVerifyAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			common.Error(ctx, http.StatusUnauthorized, "Token 无效或已过期")
			ctx.Abort()
			return
		}

		user, err := userRepo.FindByID(claims.UserID)
		if err != nil {
			common.Error(ctx, http.StatusInternalServerError, "用户状态校验失败")
			ctx.Abort()
			return
		}
		if user == nil || user.Username != claims.Username {
			common.Error(ctx, http.StatusUnauthorized, "Token 用户不存在")
			ctx.Abort()
			return
		}
		now := time.Now().UTC()
		if user.BanExpired(now) {
			if atomic, ok := userRepo.(interface {
				ClearExpiredBanIfDue(uint, time.Time) (bool, error)
			}); ok {
				if _, err := atomic.ClearExpiredBanIfDue(user.ID, now); err != nil {
					common.Error(ctx, http.StatusInternalServerError, "账号状态更新失败")
					ctx.Abort()
					return
				}
				user, err = userRepo.FindByID(claims.UserID)
				if err != nil || user == nil {
					common.Error(ctx, http.StatusInternalServerError, "账号状态校验失败")
					ctx.Abort()
					return
				}
			} else if user.ClearExpiredBan(now) {
				if err := userRepo.Save(user); err != nil {
					common.Error(ctx, http.StatusInternalServerError, "账号状态更新失败")
					ctx.Abort()
					return
				}
			}
		}
		if user.IsDeleted {
			common.Error(ctx, http.StatusForbidden, "账号已注销")
			ctx.Abort()
			return
		}
		if user.IsBanned {
			common.Error(ctx, http.StatusForbidden, "账号已被封禁，请联系管理员")
			ctx.Abort()
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			common.Error(ctx, http.StatusUnauthorized, "Token 已失效，请重新登录")
			ctx.Abort()
			return
		}
		active, err := jwtManager.SessionActive(claims)
		if err != nil {
			common.Error(ctx, http.StatusInternalServerError, "会话状态校验失败")
			ctx.Abort()
			return
		}
		if !active {
			common.Error(ctx, http.StatusUnauthorized, "会话已退出，请重新登录")
			ctx.Abort()
			return
		}
		if user.MustChangePassword && ctx.FullPath() != "/api/v1/auth/password" {
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "code": "PASSWORD_CHANGE_REQUIRED", "error": "必须先修改登录密码", "password_change_path": "/api/v1/auth/password"})
			ctx.Abort()
			return
		}

		ctx.Set(ContextUserIDKey, claims.UserID)
		ctx.Set(ContextUsernameKey, claims.Username)
		ctx.Set(ContextSessionIDKey, claims.SessionID)
		ctx.Set(ContextAuthTimeKey, claims.AuthTime)
		ctx.Set(ContextUserRoleKey, user.Role)
		ctx.Next()
	}
}
