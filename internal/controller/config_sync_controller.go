package controller

import (
	"errors"
	"net/http"
	"strconv"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

type syncAcknowledgementRequest struct {
	DeviceID      string `json:"device_id" binding:"required"`
	Revision      uint64 `json:"revision"`
	Platform      string `json:"platform,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

// IdentityMatches 查找同一零知识身份指纹对应的活动或已删除资产，用于安全地提示恢复。
func (c *ConfigController) IdentityMatches(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	matches, err := c.configService.FindIdentityMatches(userID, ctx.Query("fingerprint"))
	if err != nil {
		if errors.Is(err, service.ErrConfigInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "身份指纹非法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "身份匹配查询失败")
		return
	}
	items := make([]gin.H, 0, len(matches))
	for _, match := range matches {
		items = append(items, gin.H{
			"asset_id": match.AssetID, "state": match.State,
			"deleted_at": match.DeletedAt, "purge_after": match.PurgeAfter,
			"server_revision": match.ServerRevision,
		})
	}
	common.Success(ctx, http.StatusOK, gin.H{"items": items})
}

// PullChanges 返回指定游标之后的最新资产快照，包含 active、deleted 与 purged。
func (c *ConfigController) PullChanges(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	cursor, limit, ok := parseSyncPullQuery(ctx)
	if !ok {
		common.Error(ctx, http.StatusBadRequest, "同步游标或分页参数非法")
		return
	}
	page, err := c.configService.PullChanges(userID, cursor, limit)
	if err != nil {
		common.Error(ctx, http.StatusInternalServerError, "增量同步拉取失败")
		return
	}
	items := make([]gin.H, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, toConfigResponse(&page.Items[i]))
	}
	common.Success(ctx, http.StatusOK, gin.H{
		"items":          items,
		"next_cursor":    page.NextCursor,
		"has_more":       page.HasMore,
		"reset_required": page.ResetRequired,
	})
}

// AcknowledgeSync 记录设备已成功应用的修订水位，为安全回收最小墓碑提供依据。
func (c *ConfigController) AcknowledgeSync(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req syncAcknowledgementRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	err := c.configService.AcknowledgeSync(userID, service.SyncAcknowledgementInput{
		DeviceID: req.DeviceID, Revision: req.Revision, Platform: req.Platform, ClientVersion: req.ClientVersion,
	})
	if err != nil {
		if errors.Is(err, service.ErrConfigInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "设备确认参数非法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "同步确认失败")
		return
	}
	common.Success(ctx, http.StatusOK, gin.H{"acknowledged_revision": req.Revision})
}

func parseSyncPullQuery(ctx *gin.Context) (uint64, int, bool) {
	cursor := uint64(0)
	limit := 100
	var err error
	if raw := ctx.Query("cursor"); raw != "" {
		cursor, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	if raw := ctx.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, false
		}
	}
	return cursor, limit, limit > 0
}
