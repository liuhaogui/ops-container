package router

import (
	"net/http"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/docs"
	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/handler"
	"github.com/liuhaogui/ops-container/internal/middleware"
	"github.com/liuhaogui/ops-container/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
)

func New(cfgManager *config.Manager, log *zap.Logger, containerService *service.ContainerService) *gin.Engine {
	cfg := cfgManager.Current()

	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(ginzap.Ginzap(log, "", true))
	engine.Use(ginzap.RecoveryWithZap(log, true))
	engine.Use(otelgin.Middleware(cfg.Telemetry.ServiceName))
	engine.Use(middleware.Prometheus())

	docs.SwaggerInfo.Title = "Ops Container API"
	docs.SwaggerInfo.Description = "Gin backend solution scaffold."
	docs.SwaggerInfo.Version = cfg.App.Version
	docs.SwaggerInfo.BasePath = ""

	healthHandler := handler.NewHealthHandler(cfgManager)
	opsHandler := handler.NewOpsHandler()
	var containerHandler *handler.ContainerHandler
	var hostHandler *handler.HostHandler
	if containerService != nil {
		containerHandler = handler.NewContainerHandler(containerService, log)
		// HostService 与 ContainerService 复用同一个 docker client，避免双 dialer。
		hostService := service.NewHostService(containerService.Docker(), cfg.App.Version)
		hostHandler = handler.NewHostHandler(hostService)
	}

	if cfg.Prometheus.Enable {
		engine.GET(cfg.Prometheus.Path, gin.WrapH(promhttp.Handler()))
	}

	if cfg.Swagger.Enable {
		swaggerHandler := ginSwagger.WrapHandler(swaggerfiles.Handler)
		engine.GET("/swagger", func(c *gin.Context) {
			c.Redirect(http.StatusTemporaryRedirect, "/swagger/index.html")
		})
		engine.GET("/swagger/*any", swaggerHandler)
	}

	api := engine.Group("/api/v1")
	{
		api.GET("/health", healthHandler.Health)
		api.GET("/config", healthHandler.CurrentConfig)
	}

	ops := engine.Group("/api/ops")
	{
		ops.GET("/ping", opsHandler.Ping)
		ops.GET("/version", opsHandler.Version)
	}

	if containerHandler != nil {
		protected := engine.Group("/api/v1", middleware.TokenAuth(cfgManager))
		{
			protected.GET("/containers", containerHandler.ListContainers)
			protected.GET("/container/:id", containerHandler.GetContainerList)
			protected.GET("/container/exec/:id", containerHandler.ExecContainerTerminal)
			protected.GET("/container/recordings/:id", containerHandler.ListRecordings)
			protected.GET("/recordings/:name", containerHandler.GetRecording)
			protected.POST("/container/stop/:id", containerHandler.StopContainer)
			protected.POST("/container/start/:id", containerHandler.StartContainer)
			protected.POST("/container/restart/:id", containerHandler.RestartContainer)
			// 静态日志：ops-api 用 GET，POST 路径保留作历史兼容。
			protected.GET("/container/log/:id", containerHandler.GetContainerLog)
			protected.POST("/container/log/:id", containerHandler.GetContainerLog)
			// 流式日志：ops-api 走 /container/log/{id}/stream；旧 /tail-log/{id} 保留兼容。
			protected.GET("/container/log/:id/stream", containerHandler.TailContainerLog)
			protected.GET("/container/tail-log/:id", containerHandler.TailContainerLog)

			if hostHandler != nil {
				protected.GET("/host/stats", hostHandler.HostStats)
			}
		}
	}

	return engine
}
