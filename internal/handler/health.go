package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/model"
)

type HealthHandler struct {
	cfgManager *config.Manager
}

func NewHealthHandler(cfgManager *config.Manager) *HealthHandler {
	return &HealthHandler{cfgManager: cfgManager}
}

// Health godoc
// @Summary Health check
// @Tags system
// @Produce json
// @Success 200 {object} model.HealthResponse
// @Router /api/v1/health [get]
func (h *HealthHandler) Health(c *gin.Context) {
	cfg := h.cfgManager.Current()
	c.JSON(http.StatusOK, model.HealthResponse{
		Status:  "ok",
		Service: cfg.App.Name,
		Version: cfg.App.Version,
	})
}

// CurrentConfig godoc
// @Summary Read current runtime config
// @Tags system
// @Produce json
// @Success 200 {object} config.Config
// @Router /api/v1/config [get]
func (h *HealthHandler) CurrentConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.cfgManager.Current())
}
