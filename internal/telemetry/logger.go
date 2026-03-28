package telemetry

import (
	"os"
	"path/filepath"

	"github.com/liuhaogui/ops-container/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func NewLogger(cfg config.LogConfig) (*zap.Logger, error) {
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level.SetLevel(zap.InfoLevel)
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	writers := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	if cfg.Filename != "" {
		_ = os.MkdirAll(filepath.Dir(cfg.Filename), 0o755)
		writers = append(writers, zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}))
	}

	core := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(writers...), level)
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)), nil
}
