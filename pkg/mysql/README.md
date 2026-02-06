# MySQL Package

`pkg/mysql` 提供了一個建置於 **GORM** 之上的標準化 MySQL 連線管理套件。它封裝了最佳實踐配置（連線池、Log、事務設定），讓業務邏輯層能專注於數據操作。

## 功能特性

-   **GORM 整合**: 完全支援 GORM 的所有強大功能 (Hooks, Preload, Associations)。
-   **最佳化配置**:
    -   預設 `SkipDefaultTransaction: true` 提升寫入效能 (約快 30%)。
    -   強制設定 `MaxOpenConns` 與 `MaxIdleConns` 避免連線洩漏。
-   **自動重連**: GORM 的 MySQL Driver 內建了斷線重連機制。
-   **啟動韌性 (Startup Resilience)**: 內建 Exponential Backoff 重試機制。當資料庫尚未就緒時 (例如 Docker 啟動順序)，應用程式會嘗試重連而非直接崩潰 (Panic)。
-   **型別安全**: 透過 Struct 定義 Schema，減少 SQL 拼寫錯誤。

## 使用範例

### 初始化

```go
cfg := mysql.Config{
    Host:            "localhost",
    Port:            3306,
    User:            "root",
    Password:        "password",
    DBName:          "game_db",
    MaxOpenConns:    100,
    MaxIdleConns:    10,
    ConnMaxLifetime: 1 * time.Hour,
    LogLevel:        "warn",
}

client, err := mysql.NewClient(cfg)
if err != nil {
    panic(err)
}
defer client.Close()
```

### 資料模型定義 (Model)

```go
type Player struct {
    ID        uint      `gorm:"primaryKey"`
    Username  string    `gorm:"size:64;uniqueIndex"`
    Level     int       `gorm:"default:1"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 基本操作 (CRUD)

```go
db := client.DB()

// 1. 自動遷移 Schema (Auto Migration)
// GORM 會自動建立或更新 Table 結構
db.AutoMigrate(&Player{})

// 2. 新增 (Create)
player := Player{Username: "Neo", Level: 1}
result := db.Create(&player) // result.Error 用於檢查錯誤

// 3. 查詢 (Read)
var p Player
db.First(&p, "username = ?", "Neo")

// 4. 更新 (Update)
db.Model(&p).Update("Level", 99)

// 5. 刪除 (Delete)
db.Delete(&p)
```
