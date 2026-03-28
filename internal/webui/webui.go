package webui

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed assets/*
var assets embed.FS

func RegisterRoutes(engine *gin.Engine) error {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return err
	}

	fileServer := http.FileServer(http.FS(sub))
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return err
	}

	engine.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
	engine.GET("/assets/*filepath", func(c *gin.Context) {
		http.StripPrefix("/assets/", fileServer).ServeHTTP(c.Writer, c.Request)
	})

	return nil
}
