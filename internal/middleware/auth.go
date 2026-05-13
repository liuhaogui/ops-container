package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/response"
)

// SecretGetter 提供运行时动态 secret 及失败计数能力。
type SecretGetter interface {
	Get() string
	RecordFailure()
	RecordSuccess()
}

// TokenAuth 鉴权中间件，优先级：
//  1. 动态 secret（ops-api 启动时拉取，每台机器独立随机）：HMAC-SHA256(secret, my_ip)
//  2. 静态 secret（auth.secret，config 兜底）
//  3. 静态 token 列表（auth.tokens）
//  4. 无任何鉴权配置 → 放行（开发模式）
//
// 连续 3 次 token 验证失败时，自动触发从 ops-api 重新拉取 secret。
func TokenAuth(cfgManager *config.Manager, secretHolder SecretGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.Current()

		dynamicSecret := ""
		if secretHolder != nil {
			dynamicSecret = secretHolder.Get()
		}

		// 无任何鉴权配置 → 拒绝（不允许无鉴权运行）
		if len(cfg.Auth.Tokens) == 0 && dynamicSecret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				response.JSON(response.NoAuth, "no auth configured", ""))
			return
		}

		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				response.JSON(response.NoAuth, "authorization required", ""))
			return
		}

		myIP := resolveMyIP(cfg.Auth.MyIP)

		// 1. 动态 secret（ops-api 拉取，每台独立随机）
		if dynamicSecret != "" {
			if hmac.Equal([]byte(token), []byte(deriveToken(dynamicSecret, myIP))) {
				if secretHolder != nil {
					secretHolder.RecordSuccess()
				}
				c.Next()
				return
			}
		}

		// 2. 静态 token 列表
		for _, allowed := range cfg.Auth.Tokens {
			if token == strings.TrimSpace(allowed) {
				c.Next()
				return
			}
		}

		// 验证失败：计数，达阈值触发重新拉取 secret
		if secretHolder != nil {
			secretHolder.RecordFailure()
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			response.JSON(response.NoAuth, "invalid token", ""))
	}
}

// extractToken 按优先级从请求中取 token：Authorization header → ?token= query → Cookie。
func extractToken(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("Authorization")); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.Query("token")); v != "" {
		return v
	}
	if cookie, err := c.Cookie("Authorization"); err == nil {
		if v := strings.TrimSpace(cookie); v != "" {
			return v
		}
	}
	return ""
}

// deriveToken 用 HMAC-SHA256(secret, ip) 派生 token，与 ops-api 侧算法完全对称。
func deriveToken(secret, ip string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.TrimSpace(ip)))
	return hex.EncodeToString(mac.Sum(nil))
}

// resolveMyIP 返回本机 IP：优先用配置值，否则自动取第一个非 loopback 的 IPv4。
func resolveMyIP(configured string) string {
	if v := strings.TrimSpace(configured); v != "" {
		return v
	}
	// 自动检测：取第一个非 loopback 的 IPv4 地址
	if hostname, err := os.Hostname(); err == nil {
		if addrs, err := net.LookupHost(hostname); err == nil {
			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil && !ip.IsLoopback() {
					return addr
				}
			}
		}
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return ""
}
