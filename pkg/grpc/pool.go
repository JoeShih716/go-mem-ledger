package grpc

import (
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Pool 管理通往多個目標的 gRPC 客戶端連線。
// 它是執行緒安全的 (Thread-safe)，並確保每個目標地址只會維護一個連線實例。
type Pool struct {
	conns       sync.Map // map[string]*grpc.ClientConn
	mu          sync.Mutex
	interceptor grpc.UnaryClientInterceptor // 全局的單一請求攔截器 (Optional)
}

// PoolOption 定義了 Pool 的配置選項函數
type PoolOption func(*Pool)

// WithInterceptor 設定 Pool 的全局 UnaryClientInterceptor
// 用於統一處理 Logging, Metrics, 或 Auth Token 注入。
func WithInterceptor(interceptor grpc.UnaryClientInterceptor) PoolOption {
	return func(p *Pool) {
		p.interceptor = interceptor
	}
}

// NewPool 建立並回傳一個新的 gRPC 連線池。
// 可以傳入多個 PoolOption 來配置連線池。
func NewPool(opts ...PoolOption) *Pool {
	p := &Pool{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// GetConnection 獲取現有的連線，或為指定目標建立新連線。
// 此方法會使用通用的預設值來設定 keepalive 與連線超時機制。
//
// 參數:
//
//	target: string - 目標伺服器地址 (e.g., "localhost:50051" 或 K8s DNS)
//	opts: ...grpc.DialOption - 可選的額外 gRPC 連線選項
//
// 回傳值:
//
//	*grpc.ClientConn: gRPC 客戶端連線物件
//	error: 若建立連線失敗則回傳錯誤
func (p *Pool) GetConnection(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// 1. 嘗試讀取現有連線 (Fast path)
	if v, ok := p.conns.Load(target); ok {
		conn := v.(*grpc.ClientConn)
		// 檢查連線是否處於健康狀態 (或正在連線中)
		// 如果連線已處於 Shutdown (已關閉) 狀態，我們需要建立新的連線。
		if conn.GetState() != connectivity.Shutdown {
			return conn, nil
		}
		// 如果已關閉，從 map 中移除並繼續建立流程
		p.conns.Delete(target)
	}

	// 2. 加鎖以防止並發時的重複建立 (Double-check locking)
	p.mu.Lock()
	defer p.mu.Unlock()

	// 3. 再次檢查 (以防在加鎖期間其他 goroutine 已經建立了連線)
	if v, ok := p.conns.Load(target); ok {
		conn := v.(*grpc.ClientConn)
		if conn.GetState() != connectivity.Shutdown {
			return conn, nil
		}
		p.conns.Delete(target)
	}

	// 4. 建立新連線
	// 設定預設的彈性選項 (Resilience options)
	defaultOpts := []grpc.DialOption{
		// 預設使用不加密連線 (Insecure)
		// 因內部服務通訊通常在私有網路 (Cluster) 或搭配 Service Mesh，不需 TLS 加密。
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // 若無活動，每 10 秒發送一次 Ping
			Timeout:             time.Second,      // 等待 Ping 回應的超時時間為 1 秒
			PermitWithoutStream: true,             // 即使沒有活躍的 Stream 也允許發送 Ping (保持連線活著)
		}),
	}

	// 如果有設定攔截器，則加入選項
	if p.interceptor != nil {
		defaultOpts = append(defaultOpts, grpc.WithUnaryInterceptor(p.interceptor))
	}

	finalOpts := append(defaultOpts, opts...)

	// 注意: 在新版 gRPC 中，grpc.NewClient 取代了 DialContext
	// 這裡建立的是一個「虛擬連線」，真正的網路連線會在第一次呼叫時才建立 (Lazy connection)
	conn, err := grpc.NewClient(target, finalOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client for target %s: %w", target, err)
	}

	// 將新連線存入 map
	p.conns.Store(target, conn)
	return conn, nil
}

// Close 關閉連線池中的所有連線。
// 通常在應用程式關閉時呼叫。
func (p *Pool) Close() error {
	var firstErr error
	// 遍歷所有連線並關閉
	p.conns.Range(func(key, value any) bool {
		conn := value.(*grpc.ClientConn)
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = err // 記錄第一個發生的錯誤
		}
		p.conns.Delete(key)
		return true
	})
	return firstErr
}
