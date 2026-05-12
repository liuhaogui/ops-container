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

// TokenAuth 鉴权中间件，支持两种模式（优先级从高到低）：
//
//  1. HMAC 模式（推荐）：配置 auth.secret + auth.my_ip，
//     ops-api 用 HMAC-SHA256(secret, my_ip) 计算 token，
//     本侧用同样算法验证，双方不存储 token。
//
//  2. 静态 token 列表：配置 auth.tokens，
//     逐一对比 Authorization header。兜底兼容旧部署。
//
//  两种模式可同时配置：secret 验证通过即放行，不再检查 tokens。
func TokenAuth(cfgManager *config.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.Current()

		// 没有任何鉴权配置 → 放行（开发模式）
		if len(cfg.Auth.Tokens) == 0 && strings.TrimSpace(cfg.Auth.Secret) == "" {
			c.Next()
			return
		}

		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				response.JSON(response.NoAuth, "authorization required", ""))
			return
		}

		// 优先走 HMAC 验证
		if secret := strings.TrimSpace(cfg.Auth.Secret); secret != "" {
			myIP := resolveMyIP(cfg.Auth.MyIP)
			expected := deriveToken(secret, myIP)
			if hmac.Equal([]byte(token), []byte(expected)) {
				c.Next()
				return
			}
		}

		// 兜底：静态 token 列表
		for _, allowed := range cfg.Auth.Tokens {
			if token == strings.TrimSpace(allowed) {
				c.Next()
				return
			}
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
