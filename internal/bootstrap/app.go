package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/router"
	"github.com/liuhaogui/ops-container/internal/service"
	"github.com/liuhaogui/ops-container/internal/server"
	"github.com/liuhaogui/ops-container/internal/telemetry"
	"go.uber.org/zap"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Application struct {
	cfgManager   *config.Manager
	logger       *zap.Logger
	container    *service.ContainerService
	tracer       *sdktrace.TracerProvider
	server       *server.HTTPServer
	secretHolder *SecretHolder
}

func NewApplication() (*Application, error) {
	configPath := os.Getenv("OPS_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfgManager, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	cfg := cfgManager.Current()

	logger, err := telemetry.NewLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	tracerProvider, err := telemetry.NewTracerProvider(context.Background(), cfg.Telemetry)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}

	containerService, err := service.NewContainerService(logger)
	if err != nil {
		logger.Warn("init docker client failed, container api may be unavailable", zap.Error(err))
	}

	// 启动时从 ops-api 拉取本机 secret，纯内存持有，不落盘
	// ops_api.url 已配置时拉取失败直接拒绝启动
	secretHolder := NewSecretHolder(cfgManager, logger)
	if err := FetchAndHold(cfg, secretHolder, logger); err != nil {
		return nil, fmt.Errorf("startup secret fetch failed: %w", err)
	}

	engine := router.New(cfgManager, logger, containerService, secretHolder)
	httpServer := server.New(cfg.Server, cfg.Development, engine, logger)

	return &Application{
		cfgManager:   cfgManager,
		logger:       logger,
		container:    containerService,
		tracer:       tracerProvider,
		server:       httpServer,
		secretHolder: secretHolder,
	}, nil
}

func (a *Application) Run(ctx context.Context) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		a.logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	}

	return a.shutdown(ctx)
}

func (a *Application) shutdown(ctx context.Context) error {
	cfg := a.cfgManager.Current()

	if err := a.server.Shutdown(ctx, cfg.Server.ShutdownTimeout); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	if err := a.container.Close(); err != nil {
		a.logger.Warn("close docker client failed", zap.Error(err))
	}

	if err := telemetry.ShutdownTracer(ctx, a.tracer); err != nil {
		a.logger.Warn("shutdown tracer failed", zap.Error(err))
	}

	if err := a.logger.Sync(); err != nil {
		return nil
	}
	return nil
}
