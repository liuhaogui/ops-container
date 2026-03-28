package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)
	httpLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
		httpLatency.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}
