# gRPC Package & Internal Communication

`pkg/grpc` 是我們微服務架構中的「內網高速公路」。它提供了一個高效的 Connection Pool，讓我們能以極低的延遲呼叫其他服務。

## 什麼是 RPC (Remote Procedure Call)?
簡單來說，就是「像呼叫本地函式一樣呼叫遠端伺服器」。
例如：`client.KickUser(...)`，你看起來像是在呼叫一個普通函式，但底層其實發送了一個網路請求給 Gateway Service。

## gRPC vs Redis Pub/Sub (通訊模式比較)

在我們的架構中，這兩種通訊方式各司其職：

| 特性 | gRPC (本套件) | Redis Pub/Sub (`pkg/redis`) |
| :--- | :--- | :--- |
| **通訊模式** | **單一目標 (Unicast)**<br>像打電話，一對一。 | **廣播 (Broadcast)**<br>像用大聲公，一對多。 |
| **典型場景** | **精準操作**：<br>- 踢掉特定玩家 (Kick)<br>- 查詢特定房間狀態<br>- 私訊 (Whisper) | **批量通知**：<br>- 全服公告<br>- 停機維護通知<br>- 跨服聊天室 |
| **可靠性** | **高** (Request/Response 確認) | **低** (射後不理，沒聽到就算了) |

## Client Pool (連線池)

為了追求極致效能，我們不應該對每個請求都建立新的 TCP 連線 (那樣太慢了)。
`pkg/grpc/pool.go` 幫我們管理了一組長連線 (Connection Pool)。

### 功能特性
-   **複用連線**: 針對相同目標 (Target) 複用底層連線。
-   **Lazy Connect**: 第一次呼叫才建立連線。
-   **Interceptor**: 支援中間件 (Middleware)，可用於統一的 Log, Metrics 或 Auth。

### 使用範例

```go
// 1. 初始化 Pool (通常在 main.go 做一次)
pool := grpcpool.NewPool(
    grpcpool.WithInterceptor(MyLoggingInterceptor), // 注入 Log
)

// 2. 獲取連線 (Target 通常來自 Router 計算結果)
target := "fishing-service-0.fishing-service.default.svc.cluster.local:8080"
conn, err := pool.GetConnection(target)
if err != nil {
    // 處理連線錯誤
}

// 3. 呼叫遠端方法
client := pb.NewGatewayClient(conn)
resp, err := client.KickUser(ctx, &pb.KickUserReq{UserId: "123"})
```
