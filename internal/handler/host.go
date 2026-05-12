package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/response"
	"github.com/liuhaogui/ops-container/internal/service"
)

type HostHandler struct {
	svc *service.HostService
}

func NewHostHandler(svc *service.HostService) *HostHandler {
	return &HostHandler{svc: svc}
}

// HostStats godoc
// @Summary Get host runtime stats (cpu/memory/disk/container count)
// @Description ops-api 多 IP 主机父行渲染的数据源。失败时各字段降级到 0，不返 5xx。
// @Tags host
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} model.ResponseBody
// @Failure 401 {object} model.StringDataResponse
// @Router /api/v1/host/stats [get]
func (h *HostHandler) HostStats(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.JSON(http.StatusOK, response.JSON(response.Success, "", service.HostStats{}))
		return
	}
	stats := h.svc.Stats(c.Request.Context())
	c.JSON(http.StatusOK, response.JSON(response.Success, "", stats))
}
