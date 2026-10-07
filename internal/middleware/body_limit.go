package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"orbitterm-server/internal/common"
)

// LimitBody rejects known oversize requests before parsing and also bounds
// chunked bodies through MaxBytesReader. Handlers map MaxBytesError to 413.
func LimitBody(maxBytes int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.Request.ContentLength > maxBytes {
			common.Error(ctx, http.StatusRequestEntityTooLarge, "请求体超过大小上限")
			ctx.Abort()
			return
		}
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes)
		ctx.Next()
	}
}
