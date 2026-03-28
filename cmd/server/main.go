package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/liuhaogui/ops-container/internal/bootstrap"
	"github.com/liuhaogui/ops-container/internal/version"
)

// @title Ops Container API
// @version 1.0
// @description A concise Gin backend solution with config, db, tracing, metrics and graceful shutdown.
// @contact.name Liu Haogui
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
var (
	Version   = "dev"
	BuildTime = ""
	GitHash   = ""
)

func main() {
	version.Version = Version
	version.BuildTime = BuildTime
	version.GoVersion = runtime.Version()
	version.GitHash = GitHash

	app, err := bootstrap.NewApplication()
	if err != nil {
		log.Fatalf("bootstrap application failed: %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("run application failed: %v", err)
	}

	fmt.Println("application exited cleanly")
}
