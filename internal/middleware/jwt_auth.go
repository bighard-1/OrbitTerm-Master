package middleware

import (
	"net/http"
	"strings"

	"orbitterm-server/internal/common"
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
// 注意：本中间件仅做身份校验，不做业务权限控制。
func JWTAuthMiddleware(jwtManager *utils.JWTManager) gin.HandlerFunc {
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

		claims, err := jwtManager.ParseAndVerifyToken(strings.TrimSpace(parts[1]))
		if err != nil {
			common.Error(ctx, http.StatusUnauthorized, "Token 无效或已过期")
			ctx.Abort()
			return
		}

		ctx.Set(ContextUserIDKey, claims.UserID)
		ctx.Set(ContextUsernameKey, claims.Username)
		ctx.Next()
	}
}
