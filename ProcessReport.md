# 開發日記 / Dev Log

# 前言
我工作大部分時間是遊戲商 (Game Provider)的角色，平常對遊戲分數的操作，只需要呼叫平台商 (Platform/Wallet Provider)提供的API，取得結果做後續遊戲內處理，這個side project想摸索當個平台商處理帳務的流程XD，順便加強自己對資料操作的觀念。[用存款提款轉帳當作例子]
因為沒有碰過交易中心之類的東西，上網稍微爬了一下文章想動手做個簡單的交易中心，
預計使用 微服務架構 (Gateway + Core) 來實作
Gateway 負責提供對外 HTTP 接口 以及
Core 負責處理業務邏輯
兩者之間使用grpc來溝通

##  開始動手做 ~ 2026-02-06

### 1. 定義domain / usecase跟一些基礎建設
 1. 建立domain (account, transaction struct)
 2. 建立usecase (ledger interface)
 3. 加入其他repo使用的golangci-lint / pkg / docker / docker-compose / Makefile
 4. 定義rpc proto


### 2. 決定預計實作usecase的三種層級

- **Level 0**: MySQL (預期最慢)
- **Level 1**: Memory Mutex (預期中速)
- **Level 2**: Memory LMAX (預期最快)

### 3. 資料庫設計
為了配合 Go 的 `int64` 極速運算，DB 欄位也全都改用 `BIGINT` (放大10000倍)。
雖然效能好，但 SELECT 出來看不懂，所以建了一個 View `human_readable_users`，把數字除回去，看起來順眼多了XD

### 4. 寫Code: MySQLLedger
研究網路文章 學習到操作資料庫要用**悲觀鎖**這個專有名詞 以及注意ID排序避免死鎖
另外定義了sqlUser struct 不要跟domain的struct 混在一起
domain不該知道使用MySQL 這樣才符合Clean Architecture


### 5. 伺服器入口與微服務架構 (Server Architecture)
為了驗證一條龍流程 (MySQLLedger -> UseCase -> gRPC)，原本想直接寫 `cmd/server/main.go`。
但考慮到未來會有 **Core** (帳務核心) 和 **Connector** (Gateway) 兩個服務，決定直接採用標準專案結構：
- `cmd/core/main.go`: 核心服務入口
- `cmd/connector/main.go`: 連接器入口 (預留)
- `internal/app/core/...`: 將 domain/usecase/adapter 全部封裝進去，避免 Connector 誤用。

### 6. Clean Architecture 加強觀念 (CoreUseCase)
在實作 gRPC Server 時，本來是直接依賴 `usecase.Ledger` (介面)。
後來為了加強 Clean Architecture 的觀念，把結構改成：
- **Repo Interface**: `usecase.Ledger` (定義存取介面)
- **Interactor**: `usecase.CoreUseCase` (實作 application logic)
- **Driver**: `grpc.NewServer` 依賴 `CoreUseCase` Struct

##  ~ 2026-02-09

### 1. 新增Pkg WAL (Write-Ahead Logging)
啟動時會先創建/讀取WAL檔案，然後將交易記錄寫入File中。

### 2. Main.go 調整
- 定義 LedgerType 常數 (MySQL, Mutex, LMAX) 跟 全域變數UsedLedgerType
- 全域變數UsedLedgerType 預設為 Memory_Mutex

### 3. 實作MemoryMutexLedger
- 從MySQL載入帳戶資料帶入Ledger做暫存(記憶體)
- 實作Ledger介面(記憶體+Mutex)
- 將 WAL 整合至 `NewMutexLedger` 與 `PostTransaction` 流程中。
- Postman grpc 測試 :
  - 轉帳 存款 提款 成功 (有寫入wal.log)
  - 關掉core重開 => 成功 (有從wal.log恢復資料) GetBlance驗證 帳號餘額正確

心得：用記憶體還是怕怕的 然後目前還沒將資料寫回MySQL 之後做LMAX完再一起整理XD
應該是額外定期將wal.log的交易資料 跟memory的餘額資料 寫回MySQL 然後再做刪除wal.log