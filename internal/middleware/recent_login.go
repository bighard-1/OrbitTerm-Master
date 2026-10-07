package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RequireRecentLogin uses the original credential verification time. Refresh
// rotation carries auth_time forward, so a refresh cannot bypass this gate.
func RequireRecentLogin(maxAge time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		value, ok := ctx.Get(ContextAuthTimeKey)
		seconds, valid := value.(int64)
		if !ok || !valid || seconds <= 0 || time.Since(time.Unix(seconds, 0)) > maxAge || time.Until(time.Unix(seconds, 0)) > time.Minute {
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "code": "ADMIN_REAUTH_REQUIRED", "error": "敏感操作需要重新登录"})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
