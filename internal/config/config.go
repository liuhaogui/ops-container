package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	App         AppConfig         `mapstructure:"app"`
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Auth        AuthConfig        `mapstructure:"auth"`
	OpsAPI      OpsAPIConfig      `mapstructure:"ops_api"`
	Log         LogConfig         `mapstructure:"log"`
	Telemetry   TelemetryConfig   `mapstructure:"telemetry"`
	Prometheus  PrometheusConfig  `mapstructure:"prometheus"`
	Swagger     SwaggerConfig     `mapstructure:"swagger"`
	Development DevelopmentConfig `mapstructure:"development"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Env     string `mapstructure:"env"`
	Version string `mapstructure:"version"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout" swaggertype:"string"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout" swaggertype:"string"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" swaggertype:"string"`
}

type DatabaseConfig struct {
	Enable          bool   `mapstructure:"enable"`
	Driver          string `mapstructure:"driver"`
	DSN             string `mapstructure:"dsn"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type AuthConfig struct {
	Tokens []string `mapstructure:"tokens"`
	// MyIP 是本机对外暴露给 ops-api 的 IP（用于 HMAC 验证）。
	// 留空时自动从 os.Hostname 解析；若多网卡建议明确配置。
	MyIP string `mapstructure:"my_ip"`
}

type OpsAPIConfig struct {
	// URL ops-api 地址，用于启动时拉取本机 secret（如 http://ops-api:8080）
	URL     string        `mapstructure:"url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Filename   string `mapstructure:"filename"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}

type TelemetryConfig struct {
	ServiceName     string `mapstructure:"service_name"`
	Exporter        string `mapstructure:"exporter"`
	OTLPEndpoint    string `mapstructure:"otlp_endpoint"`
	Insecure        bool   `mapstructure:"insecure"`
	SamplingPercent int    `mapstructure:"sampling_percent"`
}

type PrometheusConfig struct {
	Enable bool   `mapstructure:"enable"`
	Path   string `mapstructure:"path"`
}

type SwaggerConfig struct {
	Enable bool   `mapstructure:"enable"`
	Path   string `mapstructure:"path"`
}

type DevelopmentConfig struct {
	EnablePProf bool `mapstructure:"enable_pprof"`
}

type Manager struct {
	viper *viper.Viper
	mu    sync.RWMutex
	cfg   Config
}

func Load(path string) (*Manager, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("OPS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	mgr := &Manager{viper: v}
	if err := mgr.reload(); err != nil {
		return nil, err
	}

	v.WatchConfig()
	v.OnConfigChange(func(event fsnotify.Event) {
		_ = mgr.reload()
	})

	return mgr, nil
}

func (m *Manager) Current() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) reload() error {
	var cfg Config
	if err := m.viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "ops-container")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "10s")
	v.SetDefault("server.shutdown_timeout", "10s")
	v.SetDefault("database.driver", "mysql")
	v.SetDefault("database.enable", false)
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 3600)
	v.SetDefault("auth.tokens", []string{})
	v.SetDefault("ops_api.url", "")
	v.SetDefault("ops_api.timeout", "10s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.filename", "logs/app.log")
	v.SetDefault("log.max_size_mb", 100)
	v.SetDefault("log.max_backups", 5)
	v.SetDefault("log.max_age_days", 30)
	v.SetDefault("log.compress", false)
	v.SetDefault("telemetry.service_name", "ops-container")
	v.SetDefault("telemetry.exporter", "otlp")
	v.SetDefault("telemetry.otlp_endpoint", "localhost:4317")
	v.SetDefault("telemetry.insecure", true)
	v.SetDefault("telemetry.sampling_percent", 100)
	v.SetDefault("prometheus.enable", true)
	v.SetDefault("prometheus.path", "/metrics")
	v.SetDefault("swagger.enable", true)
	v.SetDefault("swagger.path", "/swagger/*any")
	v.SetDefault("development.enable_pprof", true)
}
