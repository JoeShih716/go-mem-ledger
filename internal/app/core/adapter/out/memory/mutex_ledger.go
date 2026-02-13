package memory

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/wal"
)

// MutexLedger 是一個使用 Mutex 實現的帳本
//
// 結構:
//
//	accounts: 帳戶資料 Map
//	mu: Mutex 用於保護帳戶資料
//	processedTransactions: 已處理過的交易 Map
//	wal: Write-Ahead Log 實例
type MutexLedger struct {
	accounts map[int64]*domain.Account
	mu       sync.RWMutex
	// 已處理過的交易
	processedTransactions map[uuid.UUID]time.Time
	// Write-Ahead Logging
	wal *wal.WAL
}

// NewMutexLedger 建立一個新的 MutexLedger 實例
//
// 參數:
//
//	accounts: 初始帳戶資料 Map
//	wal: Write-Ahead Log 實例
//
// 回傳:
//
//	*MutexLedger: MutexLedger 實例
//	error: 初始化錯誤 (如 WAL 恢復失敗)
func NewMutexLedger(accounts map[int64]*domain.Account, wal *wal.WAL) (*MutexLedger, error) {
	ledger := &MutexLedger{
		accounts:              accounts,
		mu:                    sync.RWMutex{},
		processedTransactions: make(map[uuid.UUID]time.Time),
		wal:                   wal,
	}
	err := ledger.recoverFromWAL()
	if err != nil {
		return nil, err
	}
	return ledger, nil
}

// recoverFromWAL 從 WAL 檔案恢復帳本狀態
//
// 回傳:
//
//	error: 恢復過程錯誤
func (m *MutexLedger) recoverFromWAL() error {
	tranHistory := make([]domain.Transaction, 0)

	err := m.wal.ReadAll(func(jsonRaw []byte) error {
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
	now := time.Now()
	for _, tran := range tranHistory {
		if err := m.applyRecoverTransaction(&tran, now); err != nil {
			return err
		}
	}
	return nil
}

// applyRecoverTransaction 恢復單筆交易至記憶體 (不寫入 WAL)
// 只有 NewMutexLedger 呼叫，無需 Lock (單執行緒)
func (m *MutexLedger) applyRecoverTransaction(tran *domain.Transaction, now time.Time) error {
	var err error
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		err = m.handleDeposit(tran)
	case domain.TransactionTypeWithdraw:
		err = m.handleWithdraw(tran)
	case domain.TransactionTypeTransfer:
		err = m.handleTransfer(tran)
	}

	if err == nil {
		m.processedTransactions[tran.TransactionID] = now
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
func (m *MutexLedger) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	account, ok := m.accounts[accountID]
	if !ok {
		return 0, domain.ErrAccountNotFound
	}
	return account.Balance, nil
}

// LoadAllAccounts 載入系統所有帳戶資料 (Level 1 實作直接回傳當前 Map)
//
// 參數:
//
//	ctx: 上下文
//
// 回傳:
//
//	map[int64]*domain.Account: 帳戶 ID 對應的 Domain Account 物件
//	error: 查詢錯誤
func (m *MutexLedger) LoadAllAccounts(ctx context.Context) (map[int64]*domain.Account, error) {
	return m.accounts, nil
}

// PostTransaction 處理交易請求 (Level 1: Mutex Lock)
//
// 參數:
//
//	ctx: 上下文
//	tran: 交易請求物件
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) PostTransaction(ctx context.Context, tran *domain.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.postTransactionInternal(tran)
}

// postTransactionInternal 執行交易核心邏輯 (內部方法)
//
// 參數:
//
//	tran: 交易物件
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) postTransactionInternal(tran *domain.Transaction) error {
	_, ok := m.processedTransactions[tran.TransactionID]
	if ok {
		return nil
	}

	// 1. 寫入 WAL (Critical Path)
	if m.wal != nil {
		// 寫入記憶體
		if err := m.wal.Write(tran); err != nil {
			return domain.ErrWALWriteFailed
		}

		// 刷入硬碟
		if err := m.wal.Flush(); err != nil {
			return domain.ErrWALWriteFailed
		}
	}

	// 2. 核心交易分發
	var err error
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		err = m.handleDeposit(tran)
	case domain.TransactionTypeWithdraw:
		err = m.handleWithdraw(tran)
	case domain.TransactionTypeTransfer:
		err = m.handleTransfer(tran)
	default:
		return nil // Unknown type, ignore or error
	}

	if err == nil {
		m.processedTransactions[tran.TransactionID] = time.Now()
	}
	return err
}

// handleDeposit 處理存款邏輯
//
// 參數:
//
//	tran: 交易物件
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) handleDeposit(tran *domain.Transaction) error {
	toAccount, ok := m.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	return toAccount.Deposit(tran.Amount)
}

// handleWithdraw 處理提款邏輯
//
// 參數:
//
//	tran: 交易物件
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (m *MutexLedger) handleWithdraw(tran *domain.Transaction) error {
	fromAccount, ok := m.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}

	return fromAccount.Withdraw(tran.Amount)
}

// handleTransfer 處理轉帳邏輯
//
// 參數:
//
//	tran: 交易物件
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (m *MutexLedger) handleTransfer(tran *domain.Transaction) error {
	fromAccount, ok := m.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	toAccount, ok := m.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}

	if err := fromAccount.Withdraw(tran.Amount); err != nil {
		return err
	}
	return toAccount.Deposit(tran.Amount)
}

var _ usecase.Ledger = (*MutexLedger)(nil)
