package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/liuhaogui/ops-container/internal/config"
	"github.com/liuhaogui/ops-container/internal/model"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	Gorm *gorm.DB
	SQL  *sql.DB
}

func NewDatabase(cfg config.DatabaseConfig, log *zap.Logger) (*Database, error) {
	if !cfg.Enable {
		log.Info("database disabled, skipping initialization")
		return nil, nil
	}

	if cfg.DSN == "" {
		return nil, fmt.Errorf("database dsn is required")
	}

	gormDB, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := gormDB.AutoMigrate(&model.User{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	log.Info("database initialized")
	return &Database{Gorm: gormDB, SQL: sqlDB}, nil
}

func (d *Database) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}
