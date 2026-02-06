package mysql

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Client 封裝 GORM DB 實例
type Client struct {
	db *gorm.DB
}

// NewClient 建立並回傳一個新的 MySQL 客戶端實例 (GORM)
//
// 參數:
//
//	cfg: Config - MySQL 連線配置
//
// 回傳值:
//
//	*Client: 封裝後的 MySQL 客戶端
//	error: 若連線失敗則回傳錯誤
func NewClient(cfg Config) (*Client, error) {
	gormConfig := &gorm.Config{
		// 預設跳過事務模式，顯著提升寫入效能 (除非業務邏輯明確需要 Transaction)
		// 對於遊戲 Log 或狀態更新這類高頻操作很有幫助
		SkipDefaultTransaction: true,
		Logger:                 newLogger(cfg.LogLevel),
	}

	var db *gorm.DB
	var err error

	// Retry mechanism for database connection
	maxRetries := 10
	retryInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(mysql.Open(cfg.DSN()), gormConfig)
		if err == nil {
			// Try pinging to ensure connection is actually alive
			rawDB, pingErr := db.DB()
			if pingErr == nil {
				if err = rawDB.Ping(); err == nil {
					break // Connection successful
				}
				err = pingErr
			}
		}

		if i < maxRetries-1 {
			fmt.Printf("Failed to connect to MySQL (attempt %d/%d): %v. Retrying in %v...\n", i+1, maxRetries, err, retryInterval)
			time.Sleep(retryInterval)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to mysql after %d attempts: %w", maxRetries, err)
	}

	// 取得底層 sql.DB 物件以設定連線池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.db: %w", err)
	}

	// 設定連線池參數 (Production Ready)
	// 這些設定對於防止資料庫連線耗盡至關重要
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 測試連線
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("mysql ping failed: %w", err)
	}

	return &Client{db: db}, nil
}

// DB 回傳底層的 *gorm.DB 實例，供業務邏輯層使用
func (c *Client) DB() *gorm.DB {
	return c.db
}

// Close 關閉資料庫連線
func (c *Client) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// newLogger 根據配置建立 GORM Logger
func newLogger(level string) logger.Interface {
	var logLevel logger.LogLevel
	switch level {
	case "info":
		logLevel = logger.Info
	case "warn":
		logLevel = logger.Warn
	case "error":
		logLevel = logger.Error
	case "silent":
		logLevel = logger.Silent
	default:
		logLevel = logger.Error // 預設只記錄錯誤
	}

	return logger.Default.LogMode(logLevel)
}
