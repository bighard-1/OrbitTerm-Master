package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

const migrationUploadLimit = 257 << 20

func (c *AdminController) ExportMigrationBundle(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req struct {
		Passphrase   string `json:"passphrase" binding:"required"`
		Reason       string `json:"reason"`
		Confirmation string `json:"confirmation"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if !validateHighRiskRequest(ctx, req.Reason, req.Confirmation) {
		return
	}
	bundle, summary, err := c.migrationBundles.Export(adminID, req.Passphrase, requestMeta(ctx))
	if err != nil {
		writeMigrationError(ctx, err, "迁移包导出失败")
		return
	}
	filename := fmt.Sprintf("orbitterm-full-migration-%s.otbackup", time.Now().UTC().Format("20060102T150405Z"))
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	ctx.Header("X-Orbit-Migration-Schema", fmt.Sprintf("%d", summary.SchemaVersion))
	ctx.Data(http.StatusOK, "application/vnd.orbitterm.migration+octet-stream", bundle)
}

func (c *AdminController) RestoreMigrationBundle(ctx *gin.Context) {
	adminID, ok := extractContextUint(ctx, middleware.ContextUserIDKey)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, migrationUploadLimit)
	if err := ctx.Request.ParseMultipartForm(migrationUploadLimit); err != nil {
		common.Error(ctx, http.StatusBadRequest, "迁移包过大或表单无效")
		return
	}
	if !validateHighRiskRequest(ctx, ctx.PostForm("reason"), ctx.PostForm("confirmation")) {
		return
	}
	fileHeader, err := ctx.FormFile("bundle")
	if err != nil {
		common.Error(ctx, http.StatusBadRequest, "请选择迁移包文件")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.Error(ctx, http.StatusBadRequest, "迁移包无法读取")
		return
	}
	defer file.Close()
	encrypted, err := io.ReadAll(io.LimitReader(file, migrationUploadLimit))
	if err != nil {
		common.Error(ctx, http.StatusBadRequest, "迁移包读取失败")
		return
	}
	result, err := c.migrationBundles.Restore(adminID, encrypted, ctx.PostForm("passphrase"), requestMeta(ctx))
	if err != nil {
		writeMigrationError(ctx, err, "迁移包恢复失败，数据库未做部分覆盖")
		return
	}
	common.Success(ctx, http.StatusOK, result)
}

func writeMigrationError(ctx *gin.Context, err error, fallback string) {
	if errors.Is(err, service.ErrMigrationPassphrase) {
		common.Error(ctx, http.StatusBadRequest, "迁移包口令至少 16 位，且首尾不能有空格")
		return
	}
	if errors.Is(err, service.ErrMigrationBundle) {
		common.Error(ctx, http.StatusBadRequest, "迁移包无效、损坏、口令错误或不包含管理员账号")
		return
	}
	common.Error(ctx, http.StatusInternalServerError, fallback)
}
