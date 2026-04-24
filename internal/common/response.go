package common

import "github.com/gin-gonic/gin"

// Success 统一成功响应结构。
func Success(ctx *gin.Context, status int, data any) {
	ctx.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

// Error 统一失败响应结构。
func Error(ctx *gin.Context, status int, message string) {
	ctx.JSON(status, gin.H{
		"success": false,
		"error":   message,
	})
}
