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
	ContextUsernameKey = "auth_username"
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
		if user.ClearExpiredBan(time.Now().UTC()) {
			if err := userRepo.Save(user); err != nil {
				common.Error(ctx, http.StatusInternalServerError, "账号状态更新失败")
				ctx.Abort()
				return
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

		ctx.Set(ContextUserIDKey, claims.UserID)
		ctx.Set(ContextUsernameKey, claims.Username)
		ctx.Next()
	}
}
