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
	"github.com/liuhaogui/ops-container/internal/webui"
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

	if err := webui.RegisterRoutes(engine); err != nil {
		log.Warn("register web ui routes failed", zap.Error(err))
	}

	docs.SwaggerInfo.Title = "Ops Container API"
	docs.SwaggerInfo.Description = "Gin backend solution scaffold."
	docs.SwaggerInfo.Version = cfg.App.Version
	docs.SwaggerInfo.BasePath = ""

	healthHandler := handler.NewHealthHandler(cfgManager)
	opsHandler := handler.NewOpsHandler()
	var containerHandler *handler.ContainerHandler
	if containerService != nil {
		containerHandler = handler.NewContainerHandler(containerService, log)
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
			protected.POST("/container/log/:id", containerHandler.GetContainerLog)
			protected.GET("/container/tail-log/:id", containerHandler.TailContainerLog)
		}
	}

	return engine
}
