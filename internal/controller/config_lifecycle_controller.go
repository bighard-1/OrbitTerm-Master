package controller

import (
	"errors"
	"net/http"
	"strconv"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

type assetMutationRequest struct {
	DeviceID     string `json:"device_id" binding:"required"`
	OperationID  string `json:"operation_id" binding:"required"`
	VectorClock  string `json:"vector_clock" binding:"required"`
	Confirmation string `json:"confirmation,omitempty"`
}

func (r assetMutationRequest) serviceInput(assetID string) service.AssetMutationInput {
	return service.AssetMutationInput{
		AssetID:     assetID,
		DeviceID:    r.DeviceID,
		OperationID: r.OperationID,
		VectorClock: r.VectorClock,
	}
}

// DeleteAsset 将资产移入最近删除。请求可安全重试，重复 Operation ID 不会延长保留时间。
func (c *ConfigController) DeleteAsset(ctx *gin.Context) {
	c.handleAssetMutation(ctx, c.configService.DeleteAsset, "删除")
}

// Trash 返回当前用户仍可恢复的云端墓碑；最小 purged 墓碑不会暴露给客户端恢复界面。
func (c *ConfigController) Trash(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	limit, offset, ok := parsePagination(ctx)
	if !ok {
		common.Error(ctx, http.StatusBadRequest, "分页参数非法")
		return
	}
	page, err := c.configService.ListTrash(userID, limit, offset)
	if err != nil {
		if errors.Is(err, service.ErrConfigInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "分页参数非法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "最近删除拉取失败")
		return
	}
	items := make([]gin.H, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, toConfigResponse(&page.Items[i]))
	}
	common.Success(ctx, http.StatusOK, gin.H{
		"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset,
	})
}

// RestoreAsset 从最近删除恢复资产。客户端必须基于最新墓碑递增 Vector Clock。
func (c *ConfigController) RestoreAsset(ctx *gin.Context) {
	c.handleAssetMutation(ctx, c.configService.RestoreAsset, "恢复")
}

// PurgeAsset 清除可恢复密文，仅保留防止旧设备复活资产的最小墓碑。
func (c *ConfigController) PurgeAsset(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req assetMutationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	if req.Confirmation != "CONFIRM" {
		common.Error(ctx, http.StatusBadRequest, "永久删除需要 confirmation=CONFIRM")
		return
	}
	result, err := c.configService.PurgeAsset(userID, req.serviceInput(ctx.Param("asset_id")))
	if !writeAssetMutationResult(ctx, result, err, "永久删除") {
		return
	}
	common.Success(ctx, http.StatusOK, toConfigResponse(result))
}

func (c *ConfigController) handleAssetMutation(
	ctx *gin.Context,
	operation func(uint, service.AssetMutationInput) (*model.ServerConfig, error),
	action string,
) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}
	var req assetMutationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	result, err := operation(userID, req.serviceInput(ctx.Param("asset_id")))
	if !writeAssetMutationResult(ctx, result, err, action) {
		return
	}
	common.Success(ctx, http.StatusOK, toConfigResponse(result))
}

func writeAssetMutationResult(ctx *gin.Context, result *model.ServerConfig, err error, action string) bool {
	if err == nil && result != nil {
		return true
	}
	switch {
	case errors.Is(err, service.ErrConfigInvalidInput):
		common.Error(ctx, http.StatusBadRequest, action+"参数非法")
	case errors.Is(err, service.ErrConfigNotFound):
		common.Error(ctx, http.StatusNotFound, "资产不存在")
	case errors.Is(err, service.ErrConfigPurged):
		common.Error(ctx, http.StatusGone, "资产已经永久删除")
	case errors.Is(err, service.ErrConfigInvalidState):
		common.Error(ctx, http.StatusConflict, "资产当前状态不允许"+action)
	case errors.Is(err, service.ErrVectorClockConflict):
		common.Error(ctx, http.StatusConflict, "版本冲突，请先拉取最新同步状态")
	default:
		common.Error(ctx, http.StatusInternalServerError, action+"失败")
	}
	return false
}

func parsePagination(ctx *gin.Context) (int, int, bool) {
	limit := 100
	offset := 0
	var err error
	if raw := ctx.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, false
		}
	}
	if raw := ctx.Query("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, false
		}
	}
	return limit, offset, limit > 0 && offset >= 0
}
