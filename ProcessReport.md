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


