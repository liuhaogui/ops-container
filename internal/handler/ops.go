package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/response"
	"github.com/liuhaogui/ops-container/internal/version"
)

type OpsHandler struct{}

func NewOpsHandler() *OpsHandler {
	return &OpsHandler{}
}

// Ping godoc
// @Summary Ping
// @Tags ops
// @Produce json
// @Success 200 {object} model.StringDataResponse
// @Router /api/ops/ping [get]
func (h *OpsHandler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, response.JSON(response.Success, "", "pong"))
}

// Version godoc
// @Summary Get service version
// @Tags ops
// @Produce json
// @Success 200 {object} model.VersionDataResponse
// @Router /api/ops/version [get]
func (h *OpsHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, response.JSON(response.Success, "", version.Info()))
}
