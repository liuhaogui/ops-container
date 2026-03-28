package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/config"
	"go.uber.org/zap"
)

type HTTPServer struct {
	srv *http.Server
	log *zap.Logger
}

func New(cfg config.ServerConfig, devCfg config.DevelopmentConfig, engine *gin.Engine, log *zap.Logger) *HTTPServer {
	if devCfg.EnablePProf {
		pprof.Register(engine)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return &HTTPServer{
		log: log,
		srv: &http.Server{
			Addr:         addr,
			Handler:      engine,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}
}

func (s *HTTPServer) Start() error {
	s.log.Info("http server listening", zap.String("addr", s.srv.Addr))
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *HTTPServer) Shutdown(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.srv.Shutdown(ctx)
}
