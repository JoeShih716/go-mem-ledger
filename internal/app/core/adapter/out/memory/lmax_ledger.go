package memory

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/wal"
)

// transactionRequest 交易請求包裝channel，讓PostTransaction可以等待結果
type transactionRequest struct {
	Tx     *domain.Transaction
	Result chan error // 讓 PostTransaction 等這個 channel
}

type LMAXLedger struct {
	accounts map[int64]*domain.Account
	// 已處理過的交易
	processedTransactions map[uuid.UUID]bool
	// Write-Ahead Logging
	wal *wal.WAL
	// 輸送帶 負責接收交易
	transactionChan chan *transactionRequest
	// Pool 減少 GC 壓力
	requestPool sync.Pool
}

// NewLMAXLedger 建立一個新的 LMAXLedger 實例
//
// 參數:
//
//	wal: Write-Ahead Log 實例
//
// 回傳:
//
//	*LMAXLedger: LMAXLedger 實例
//	error: 初始化錯誤
func NewLMAXLedger(accounts map[int64]*domain.Account, wal *wal.WAL) (*LMAXLedger, error) {
	ledger := &LMAXLedger{
		accounts:              accounts, // 直接引用傳入的 Map
		processedTransactions: make(map[uuid.UUID]bool),
		wal:                   wal,
		transactionChan:       make(chan *transactionRequest, 1000), // Buffer 1000
		requestPool: sync.Pool{
			New: func() interface{} {
				return &transactionRequest{
					Result: make(chan error, 1),
				}
			},
		},
	}

	// 在啟動前先恢復資料
	if err := ledger.recoverFromWAL(); err != nil {
		return nil, err
	}

	return ledger, nil
}

// recoverFromWAL 從 WAL 檔案恢復帳本狀態
//
// 回傳:
//
//	error: 恢復過程錯誤
func (l *LMAXLedger) recoverFromWAL() error {
	tranHistory := make([]domain.Transaction, 0)

	err := l.wal.ReadAll(func(jsonRaw []byte) error {
		var tran domain.Transaction
		if err := json.Unmarshal(jsonRaw, &tran); err != nil {
			return err
		}
		tranHistory = append(tranHistory, tran)
		return nil
	})
	if err != nil {
		return err
	}

	for _, tran := range tranHistory {
		if err := l.applyRecoverTransaction(&tran); err != nil {
			return err
		}
	}
	return nil
}

// applyRecoverTransaction 恢復單筆交易 (不寫 WAL，不透過 Channel)
func (l *LMAXLedger) applyRecoverTransaction(tran *domain.Transaction) error {
	// 直接更新 State，不需要 Lock 因為這是在 NewLMAXLedger 裡跑的 (單執行緒)
	var err error
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		err = l.handleDeposit(tran)
	case domain.TransactionTypeWithdraw:
		err = l.handleWithdraw(tran)
	case domain.TransactionTypeTransfer:
		err = l.handleTransfer(tran)
	}

	if err == nil {
		l.processedTransactions[tran.TransactionID] = true
	}
	return err
}

// GetAccountBalance 取得指定帳戶的當前餘額
//
// 參數:
//
//	ctx: 上下文
//	accountID: 帳戶 ID
//
// 回傳:
//
//	int64: 帳戶餘額
//	error: 查詢錯誤 (如帳戶不存在)
func (l *LMAXLedger) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	account, ok := l.accounts[accountID]
	if !ok {
		return 0, domain.ErrAccountNotFound
	}
	return account.Balance, nil
}

// LoadAllAccounts implements usecase.Ledger.
func (l *LMAXLedger) LoadAllAccounts(ctx context.Context) (map[int64]*domain.Account, error) {
	return l.accounts, nil
}

// PostTransaction 接收交易請求
//
// 參數:
//
//	ctx: 上下文
//	tran: 交易請求物件
//
// 回傳:
//
//	error: 處理錯誤
//
// PostTransaction(等待) -> Channel -> Run Loop (核心) -> WAL -> Map Update -> Result Channel -> PostTransaction(收到結果)
func (l *LMAXLedger) PostTransaction(ctx context.Context, tran *domain.Transaction) error {
	return l.postTransactionInternal(tran)
}

func (l *LMAXLedger) postTransactionInternal(tran *domain.Transaction) error {
	// 1. 放入輸送帶 (使用 sync.Pool 減少 GC)
	req := l.requestPool.Get().(*transactionRequest)
	req.Tx = tran
	// 清空 Channel (雖然理論上應該是空的，但保險起見)
	select {
	case <-req.Result:
	default:
	}

	l.transactionChan <- req
	err := <-req.Result
	l.requestPool.Put(req)
	return err
}

// Start 啟動核心引擎 (非同步)
func (l *LMAXLedger) Start(ctx context.Context) {
	go l.run(ctx)
}

func (l *LMAXLedger) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// 收到關閉信號，把剩下的交易處理完
			l.drain()
			return
		case req := <-l.transactionChan:
			l.processTransaction(req)
		}
	}
}

func (l *LMAXLedger) drain() {
	for {
		select {
		case req := <-l.transactionChan:
			l.processTransaction(req)
		default:
			return
		}
	}
}

// processTransaction 處理單筆交易並回傳結果
func (l *LMAXLedger) processTransaction(req *transactionRequest) {
	tran := req.Tx

	// 0. Idempotency Check (Thread Safe in Loop)
	if _, ok := l.processedTransactions[tran.TransactionID]; ok {
		req.Result <- nil
		return
	}

	// 1. 寫入 WAL (Critical Path)
	if l.wal != nil {
		if err := l.wal.Write(tran); err != nil {
			req.Result <- domain.ErrWALWriteFailed
			return
		}
	}

	// 2. 執行業務邏輯 (Deposit/Withdraw)
	var err error
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		err = l.handleDeposit(tran)
	case domain.TransactionTypeWithdraw:
		err = l.handleWithdraw(tran)
	case domain.TransactionTypeTransfer:
		err = l.handleTransfer(tran)
	default:
		err = nil // Unknown type or no-op
	}

	// 3. 更新 Idempotency
	if err == nil {
		l.processedTransactions[tran.TransactionID] = true
	}

	// 4. 回傳結果
	req.Result <- err
}

func (l *LMAXLedger) handleDeposit(tran *domain.Transaction) error {
	toAccount, ok := l.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	return toAccount.Deposit(tran.Amount)
}

func (l *LMAXLedger) handleWithdraw(tran *domain.Transaction) error {
	fromAccount, ok := l.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}

	return fromAccount.Withdraw(tran.Amount)
}

func (l *LMAXLedger) handleTransfer(tran *domain.Transaction) error {
	fromAccount, ok := l.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	toAccount, ok := l.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}

	if err := fromAccount.Withdraw(tran.Amount); err != nil {
		return err
	}
	return toAccount.Deposit(tran.Amount)
}

var _ usecase.Ledger = (*LMAXLedger)(nil)
