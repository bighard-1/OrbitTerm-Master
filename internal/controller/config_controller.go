package controller

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/middleware"
	"orbitterm-server/internal/model"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

// ConfigController 负责处理云同步接口。
type ConfigController struct {
	configService service.ConfigService
}

func NewConfigController(configService service.ConfigService) *ConfigController {
	return &ConfigController{configService: configService}
}

// uploadConfigRequest 上传配置请求体。
// 后端不负责解密，仅负责存储加密后的密文。
type uploadConfigRequest struct {
	ID                  *uint  `json:"id,omitempty"`
	EncryptedBlobBase64 string `json:"encrypted_blob_base64" binding:"required"`
	VectorClock         string `json:"vector_clock" binding:"required"`
}

// Upload godoc
// @Summary 上传加密配置
// @Description 上传加密后的配置数据块（后端不负责解密，仅负责密文存储）。
// @Tags config
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body uploadConfigRequest true "配置上传信息"
// @Success 200 {object} map[string]any
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 409 {object} map[string]any
// @Router /api/v1/config/upload [post]
func (c *ConfigController) Upload(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	var req uploadConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		common.Error(ctx, http.StatusBadRequest, "请求参数格式错误")
		return
	}

	encryptedBlob, err := base64.StdEncoding.DecodeString(req.EncryptedBlobBase64)
	if err != nil {
		common.Error(ctx, http.StatusBadRequest, "encrypted_blob_base64 不是合法的 Base64")
		return
	}

	result, err := c.configService.Upload(userID, req.ID, encryptedBlob, req.VectorClock)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConfigInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "配置参数不合法")
		case errors.Is(err, service.ErrConfigNotFound):
			common.Error(ctx, http.StatusNotFound, "配置不存在")
		case errors.Is(err, service.ErrVectorClockConflict):
			common.Error(ctx, http.StatusConflict, "版本冲突，请先 pull 最新配置")
		default:
			common.Error(ctx, http.StatusInternalServerError, "上传失败")
		}
		return
	}

	status := http.StatusOK
	if req.ID == nil || *req.ID == 0 {
		status = http.StatusCreated
	}

	common.Success(ctx, status, toConfigResponse(result))
}

// Pull godoc
// @Summary 拉取配置列表
// @Description 拉取当前登录用户的全部密文配置（后端不负责解密，仅负责密文存储）。
// @Tags config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Router /api/v1/config/pull [get]
func (c *ConfigController) Pull(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	configs, err := c.configService.Pull(userID)
	if err != nil {
		if errors.Is(err, service.ErrConfigInvalidInput) {
			common.Error(ctx, http.StatusBadRequest, "用户信息非法")
			return
		}
		common.Error(ctx, http.StatusInternalServerError, "拉取失败")
		return
	}

	resp := make([]gin.H, 0, len(configs))
	for i := range configs {
		resp = append(resp, toConfigResponse(&configs[i]))
	}

	common.Success(ctx, http.StatusOK, gin.H{"items": resp})
}

// Delete godoc
// @Summary 删除配置
// @Description 删除当前登录用户指定 ID 的密文配置（后端不负责解密，仅负责密文存储）。
// @Tags config
// @Security BearerAuth
// @Produce json
// @Param id path int true "配置 ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 401 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /api/v1/config/{id} [delete]
func (c *ConfigController) Delete(ctx *gin.Context) {
	userID, ok := extractUserID(ctx)
	if !ok {
		common.Error(ctx, http.StatusUnauthorized, "未授权")
		return
	}

	idValue := ctx.Param("id")
	idUint64, err := strconv.ParseUint(idValue, 10, 64)
	if err != nil || idUint64 == 0 {
		common.Error(ctx, http.StatusBadRequest, "配置 ID 非法")
		return
	}

	if err := c.configService.Delete(userID, uint(idUint64)); err != nil {
		switch {
		case errors.Is(err, service.ErrConfigInvalidInput):
			common.Error(ctx, http.StatusBadRequest, "删除参数非法")
		case errors.Is(err, service.ErrConfigNotFound):
			common.Error(ctx, http.StatusNotFound, "配置不存在")
		default:
			common.Error(ctx, http.StatusInternalServerError, "删除失败")
		}
		return
	}

	common.Success(ctx, http.StatusOK, gin.H{"deleted": true})
}

func extractUserID(ctx *gin.Context) (uint, bool) {
	value, exists := ctx.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	if !ok || userID == 0 {
		return 0, false
	}
	return userID, true
}

func toConfigResponse(cfg *model.ServerConfig) gin.H {
	return gin.H{
		"id":                    cfg.ID,
		"user_id":               cfg.UserID,
		"encrypted_blob_base64": base64.StdEncoding.EncodeToString(cfg.EncryptedBlob),
		"vector_clock":          cfg.VectorClock,
		"updated_at":            cfg.UpdatedAt,
	}
}
