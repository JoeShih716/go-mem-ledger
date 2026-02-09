package memory

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/wal"
)

type MutexLedger struct {
	accounts map[int64]*domain.Account
	mu       sync.RWMutex
	// 已處理過的交易
	processedTransactions map[[16]byte]bool
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
		processedTransactions: make(map[[16]byte]bool),
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

	for _, tran := range tranHistory {
		if err := m.applyTransaction(&tran); err != nil {
			return err
		}
	}
	return nil
}

// applyTransaction 應用單筆交易至記憶體 (不寫入 WAL)
// 用於 Recovery 階段或測試
//
// 參數:
//
//	tran: 交易物件
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) applyTransaction(tran *domain.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.postTransactionInternal(tran, false) // false = don't write WAL
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
	return m.postTransactionInternal(tran, true)
}

// postTransactionInternal 執行交易核心邏輯 (內部方法)
//
// 參數:
//
//	tran: 交易物件
//	writeWAL: 是否寫入 WAL (Recovery 時傳 false，正常交易傳 true)
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) postTransactionInternal(tran *domain.Transaction, writeWAL bool) error {
	_, ok := m.processedTransactions[tran.TransactionID]
	if ok {
		return nil
	}
	// 核心交易分發
	var err error
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		err = m.handleDeposit(tran, writeWAL)
	case domain.TransactionTypeWithdraw:
		err = m.handleWithdraw(tran, writeWAL)
	case domain.TransactionTypeTransfer:
		err = m.handleTransfer(tran, writeWAL)
	default:
		return nil // Unknown type, ignore or error
	}

	if err == nil {
		m.processedTransactions[tran.TransactionID] = true
	}
	return err
}

// handleDeposit 處理存款邏輯
//
// 參數:
//
//	tran: 交易物件
//	writeWAL: 是否寫入 WAL
//
// 回傳:
//
//	error: 處理錯誤
func (m *MutexLedger) handleDeposit(tran *domain.Transaction, writeWAL bool) error {
	toAccount, ok := m.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if writeWAL {
		if err := m.wal.Write(tran); err != nil {
			return domain.ErrWALWriteFailed
		}
	}
	return toAccount.Deposit(tran.Amount)
}

// handleWithdraw 處理提款邏輯
//
// 參數:
//
//	tran: 交易物件
//	writeWAL: 是否寫入 WAL
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (m *MutexLedger) handleWithdraw(tran *domain.Transaction, writeWAL bool) error {
	fromAccount, ok := m.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if fromAccount.Balance < tran.Amount {
		return domain.ErrInsufficientBalance
	}
	if writeWAL {
		if err := m.wal.Write(tran); err != nil {
			return domain.ErrWALWriteFailed
		}
	}
	return fromAccount.Withdraw(tran.Amount)
}

// handleTransfer 處理轉帳邏輯
//
// 參數:
//
//	tran: 交易物件
//	writeWAL: 是否寫入 WAL
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (m *MutexLedger) handleTransfer(tran *domain.Transaction, writeWAL bool) error {
	fromAccount, ok := m.accounts[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	toAccount, ok := m.accounts[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if fromAccount.Balance < tran.Amount {
		return domain.ErrInsufficientBalance
	}
	if writeWAL {
		if err := m.wal.Write(tran); err != nil {
			return domain.ErrWALWriteFailed
		}
	}
	if err := fromAccount.Withdraw(tran.Amount); err != nil {
		return err
	}
	return toAccount.Deposit(tran.Amount)
}

var _ usecase.Ledger = (*MutexLedger)(nil)
