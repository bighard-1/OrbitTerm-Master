package controller

import (
	"net/http"

	"orbitterm-server/internal/common"
	"orbitterm-server/internal/service"

	"github.com/gin-gonic/gin"
)

type HealthController struct {
	health service.SystemHealthService
}

func NewHealthController(health service.SystemHealthService) *HealthController {
	return &HealthController{health: health}
}

func (c *HealthController) Health(ctx *gin.Context) {
	report := c.health.PublicHealth()
	status := http.StatusOK
	if report.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	common.Success(ctx, status, report)
}

func (c *HealthController) RuntimeStatus(ctx *gin.Context) {
	common.Success(ctx, http.StatusOK, c.health.RuntimeStatus())
}
