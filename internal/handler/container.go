package handler

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/liuhaogui/ops-container/internal/response"
	"github.com/liuhaogui/ops-container/internal/service"
	"go.uber.org/zap"
)

type ContainerHandler struct {
	svc      *service.ContainerService
	log      *zap.Logger
	upgrader websocket.Upgrader
}

func NewContainerHandler(svc *service.ContainerService, log *zap.Logger) *ContainerHandler {
	return &ContainerHandler{
		svc: svc,
		log: log,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// ListContainers godoc
// @Summary Get all containers
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param all query bool false "Whether to include stopped containers" default(true)
// @Success 200 {object} model.ResponseBody
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/containers [get]
func (h *ContainerHandler) ListContainers(c *gin.Context) {
	all := true
	if raw := strings.TrimSpace(c.Query("all")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "invalid all parameter", ""))
			return
		}
		all = parsed
	}

	list, err := h.svc.ListContainers(c.Request.Context(), all)
	if err != nil {
		h.log.Error("list containers failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	c.JSON(http.StatusOK, response.JSON(response.Success, "", list))
}

// GetContainerList godoc
// @Summary Get container list by instance id
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Instance ID"
// @Param container_type query int false "1: instance containers, 2: init containers, 3: other containers"
// @Success 200 {object} model.ResponseBody
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/container/{id} [get]
func (h *ContainerHandler) GetContainerList(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	containerType, _ := strconv.Atoi(c.DefaultQuery("container_type", "1"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}

	list, err := h.svc.GetContainerList(c.Request.Context())
	if err != nil {
		h.log.Error("get container list failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	res := make([]types.Container, 0)
	initService := make([]types.Container, 0)
	otherService := make([]types.Container, 0)
	for _, item := range list {
		if len(item.Labels) == 0 {
			continue
		}
		if item.Labels["instance_init"] == "true" {
			initService = append(initService, item)
			continue
		}
		if item.Labels["instance_id"] == id {
			res = append(res, item)
			continue
		}
		otherService = append(otherService, item)
	}

	switch containerType {
	case 2:
		c.JSON(http.StatusOK, response.JSON(response.Success, "", initService))
	case 3:
		c.JSON(http.StatusOK, response.JSON(response.Success, "", otherService))
	default:
		c.JSON(http.StatusOK, response.JSON(response.Success, "", res))
	}
}

// StopContainer godoc
// @Summary Stop a container
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Container ID"
// @Success 200 {object} model.StringDataResponse
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/container/stop/{id} [post]
func (h *ContainerHandler) StopContainer(c *gin.Context) {
	h.changeContainerState(c, "stop")
}

// StartContainer godoc
// @Summary Start a container
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Container ID"
// @Success 200 {object} model.StringDataResponse
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/container/start/{id} [post]
func (h *ContainerHandler) StartContainer(c *gin.Context) {
	h.changeContainerState(c, "start")
}

// GetContainerLog godoc
// @Summary Get container logs
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Container ID"
// @Param lines query int false "Number of log lines" default(100)
// @Success 200 {object} model.StringListResponse
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/container/log/{id} [post]
func (h *ContainerHandler) GetContainerLog(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}
	if lines < 1 {
		lines = 100
	}

	logs, err := h.svc.GetContainerLog(c.Request.Context(), id, lines)
	if err != nil {
		h.log.Error("get container log failed", zap.String("container_id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	c.JSON(http.StatusOK, response.JSON(response.Success, "", logs))
}

// TailContainerLog godoc
// @Summary Tail container logs over websocket
// @Tags container
// @Security ApiKeyAuth
// @Produce plain
// @Param id path string true "Container ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Router /api/v1/container/tail-log/{id} [get]
func (h *ContainerHandler) TailContainerLog(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Error("upgrade websocket failed", zap.Error(err))
		return
	}

	if err := h.svc.TailContainerLog(context.Background(), id, conn); err != nil {
		h.log.Error("tail container log failed", zap.String("container_id", id), zap.Error(err))
	}
}

// ExecContainerTerminal godoc
// @Summary Open an interactive terminal inside a container
// @Tags container
// @Security ApiKeyAuth
// @Produce plain
// @Param id path string true "Container ID"
// @Param shell query string false "Shell executable" default(/bin/sh)
// @Param arg query []string false "Shell arguments, repeatable"
// @Param cols query int false "Terminal columns" default(120)
// @Param rows query int false "Terminal rows" default(40)
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} model.StringDataResponse
// @Failure 401 {object} model.StringDataResponse
// @Router /api/v1/container/exec/{id} [get]
func (h *ContainerHandler) ExecContainerTerminal(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}

	shell := strings.TrimSpace(c.DefaultQuery("shell", "/bin/sh"))
	args := c.QueryArray("arg")
	cmd := append([]string{shell}, args...)

	cols := uint(120)
	rows := uint(40)
	if value := strings.TrimSpace(c.Query("cols")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "invalid cols parameter", ""))
			return
		}
		cols = uint(parsed)
	}
	if value := strings.TrimSpace(c.Query("rows")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "invalid rows parameter", ""))
			return
		}
		rows = uint(parsed)
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Error("upgrade websocket failed", zap.Error(err))
		return
	}

	if err := h.svc.ExecContainerTerminal(context.Background(), id, cmd, cols, rows, conn); err != nil {
		h.log.Error("exec container terminal failed", zap.String("container_id", id), zap.Strings("cmd", cmd), zap.Error(err))
	}
}

// ListRecordings godoc
// @Summary List asciinema recordings for a container
// @Tags container
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Container ID"
// @Success 200 {object} model.ResponseBody
// @Failure 401 {object} model.StringDataResponse
// @Failure 500 {object} model.StringDataResponse
// @Router /api/v1/container/recordings/{id} [get]
func (h *ContainerHandler) ListRecordings(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}

	token := strings.TrimSpace(c.GetHeader("Authorization"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}

	items, err := service.ListRecordings(id, token)
	if err != nil {
		h.log.Error("list recordings failed", zap.String("container_id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	c.JSON(http.StatusOK, response.JSON(response.Success, "", items))
}

func (h *ContainerHandler) GetRecording(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	path, err := service.RecordingPath(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, err, ""))
		return
	}

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, response.JSON(response.NotFound, "recording not found", ""))
			return
		}
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	c.Header("Content-Type", "application/x-asciicast")
	c.File(path)
}

func (h *ContainerHandler) changeContainerState(c *gin.Context, action string) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, response.JSON(response.ParamError, "", ""))
		return
	}

	var err error
	switch action {
	case "start":
		err = h.svc.StartContainer(c.Request.Context(), id)
	case "stop":
		err = h.svc.StopContainer(c.Request.Context(), id)
	}

	if err != nil {
		h.log.Error("change container state failed", zap.String("action", action), zap.String("container_id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, response.JSON(response.Failed, err, ""))
		return
	}

	c.JSON(http.StatusOK, response.JSON(response.Success, "", ""))
}
