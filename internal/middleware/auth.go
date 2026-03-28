package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/response"
)

func TokenAuth(cfgManager *config.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.Current()
		if len(cfg.Auth.Tokens) == 0 {
			c.Next()
			return
		}

		token := strings.TrimSpace(c.GetHeader("Authorization"))
		if token == "" {
			token = strings.TrimSpace(c.Query("token"))
		}
		if token == "" {
			if cookie, err := c.Cookie("Authorization"); err == nil {
				token = strings.TrimSpace(cookie)
			}
		}

		for _, allowed := range cfg.Auth.Tokens {
			if token != "" && token == strings.TrimSpace(allowed) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, response.JSON(response.NoAuth, "token not exists.", ""))
	}
}
