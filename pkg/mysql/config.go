package mysql

import (
	"fmt"
	"time"
)

// Config 定義 MySQL 連線與連線池的配置
type Config struct {
	Host     string // 資料庫主機地址
	Port     int    // 資料庫埠號 (預設 3306)
	User     string // 使用者名稱
	Password string // 密碼
	DBName   string // 資料庫名稱

	// 連線池設定 (Connection Pool)
	// 參考: https://github.com/go-sql-driver/mysql#important-settings
	MaxOpenConns    int           // 最大開啟連線數
	MaxIdleConns    int           // 最大閒置連線數
	ConnMaxLifetime time.Duration // 連線最大存活時間

	// GORM 設定
	LogLevel string // Log 等級: "silent", "error", "warn", "info"
}

// DSN (Data Source Name) 產生連線字串
// 格式: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.DBName,
	)
}
