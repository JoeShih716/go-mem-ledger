package mysql

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/mysql"
)

// sqlUser 對應資料庫的 users 表
type sqlUser struct {
	ID        int64 `gorm:"primaryKey"`
	Balance   int64
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"` // 自動更新時間
}

// Deposit 存款
func (u *sqlUser) Deposit(amount int64) error {
	if amount < 0 {
		return domain.ErrAmountMustBePositive
	}
	u.Balance += amount
	return nil
}

// Withdraw 提款
func (u *sqlUser) Withdraw(amount int64) error {
	if amount < 0 {
		return domain.ErrAmountMustBePositive
	}
	if u.Balance < amount {
		return domain.ErrInsufficientBalance
	}
	u.Balance -= amount
	return nil
}

func (*sqlUser) TableName() string {
	return "users"
}

// sqlTransaction 對應資料庫的 transactions 表
type sqlTransaction struct {
	ID            int64  `gorm:"primaryKey;autoIncrement"`
	RefID         []byte `gorm:"column:ref_id;type:binary(16);uniqueIndex"` // 對應 domain.TransactionID
	Sequence      uint64 `gorm:"index"`
	FromAccountID int64
	ToAccountID   int64
	Amount        int64
	Type          uint8
	CreatedAt     int64 `gorm:"autoCreateTime:milli"` // 自動寫入時間
}

func (*sqlTransaction) TableName() string {
	return "transactions"
}

type MySQLLedger struct {
	client *mysql.Client
}

// NewMySQLLedger 建立一個新的 MySQLLedger 實例
//
// 參數:
//
//	client: MySQL 客戶端連線
//
// 回傳:
//
//	*MySQLLedger: MySQLLedger 實例
func NewMySQLLedger(client *mysql.Client) *MySQLLedger {
	return &MySQLLedger{
		client: client,
	}
}

// PostTransaction 處理交易請求 (Level 0: MySQL Transaction)
//
// 參數:
//
//	ctx: 上下文 (Context)
//	tran: 交易請求物件 (Transaction)
//
// 回傳:
//
//	error: 處理錯誤，若成功則為 nil
func (ledger *MySQLLedger) PostTransaction(ctx context.Context, tran *domain.Transaction) error {
	return ledger.client.DB().Transaction(func(tx *gorm.DB) error {
		// 1. Idempotency Check 冪等性檢查
		if exists, err := ledger.checkTransactionExists(tx, tran); err != nil {
			return err
		} else if exists {
			return nil
		}

		// 2. Lock & Load Accounts 悲觀鎖載入
		users, userMap, err := ledger.lockAccounts(tx, tran)
		if err != nil {
			return err
		}

		// 3. Business Logic
		if err := ledger.processTransactionLogic(tran, userMap); err != nil {
			return err
		}

		// 4. Update Accounts 更新帳戶
		if err := ledger.saveUsers(tx, users); err != nil {
			return err
		}

		// 5. Create Transaction Record 建立交易記錄
		return ledger.createTransactionLog(tx, tran)
	})
}

// checkTransactionExists 檢查交易是否已經存在 (冪等性檢查)
//
// 參數:
//
//	tx: GORM 資料庫事務 (Transaction)
//	tran: 交易請求物件
//
// 回傳:
//
//	bool: 是否已存在
//	error: 查詢錯誤
func (ledger *MySQLLedger) checkTransactionExists(tx *gorm.DB, tran *domain.Transaction) (bool, error) {
	var count int64
	err := tx.Model(&sqlTransaction{}).Where("ref_id = ?", tran.TransactionID[:]).Count(&count).Error
	if err != nil {
		return false, domain.ErrSelectTransactionFailed
	}
	return count > 0, nil
}

// lockAccounts 鎖定並載入涉及的帳戶 (悲觀鎖 FOR UPDATE)
//
// 參數:
//
//	tx: GORM 資料庫事務
//	tran: 交易請求物件
//
// 回傳:
//
//	[]sqlUser: 鎖定的使用者列表
//	map[int64]*sqlUser: 使用者 ID 對應的指標 Map
//	error: 查詢或鎖定錯誤
func (ledger *MySQLLedger) lockAccounts(tx *gorm.DB, tran *domain.Transaction) ([]sqlUser, map[int64]*sqlUser, error) {
	lockIDs := tran.GetLockIDs()
	var users []sqlUser
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", lockIDs).
		Find(&users).Error; err != nil {
		return nil, nil, err
	}

	userMap := make(map[int64]*sqlUser)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	return users, userMap, nil
}

// processTransactionLogic 執行核心交易業務邏輯
//
// 參數:
//
//	tran: 交易請求物件
//	userMap: 已鎖定的使用者 Map
//
// 回傳:
//
//	error: 業務邏輯驗證錯誤 (如餘額不足)
func (ledger *MySQLLedger) processTransactionLogic(tran *domain.Transaction, userMap map[int64]*sqlUser) error {
	switch tran.Type {
	case domain.TransactionTypeDeposit:
		return ledger.handleDeposit(tran, userMap)
	case domain.TransactionTypeWithdraw:
		return ledger.handleWithdraw(tran, userMap)
	case domain.TransactionTypeTransfer:
		return ledger.handleTransfer(tran, userMap)
	default:
		return nil
	}
}

// handleDeposit 處理存款邏輯
//
// 參數:
//
//	tran: 交易請求物件
//	userMap: 已鎖定的使用者 Map
//
// 回傳:
//
//	error: 處理錯誤
func (ledger *MySQLLedger) handleDeposit(tran *domain.Transaction, userMap map[int64]*sqlUser) error {
	toUser, ok := userMap[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	return toUser.Deposit(tran.Amount)
}

// handleWithdraw 處理提款邏輯
//
// 參數:
//
//	tran: 交易請求物件
//	userMap: 已鎖定的使用者 Map
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (ledger *MySQLLedger) handleWithdraw(tran *domain.Transaction, userMap map[int64]*sqlUser) error {
	fromUser, ok := userMap[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	return fromUser.Withdraw(tran.Amount)
}

// handleTransfer 處理轉帳邏輯
//
// 參數:
//
//	tran: 交易請求物件
//	userMap: 已鎖定的使用者 Map
//
// 回傳:
//
//	error: 處理錯誤 (如餘額不足)
func (ledger *MySQLLedger) handleTransfer(tran *domain.Transaction, userMap map[int64]*sqlUser) error {
	fromUser, ok := userMap[tran.From]
	if !ok {
		return domain.ErrAccountNotFound
	}
	toUser, ok := userMap[tran.To]
	if !ok {
		return domain.ErrAccountNotFound
	}
	// 先扣再加款
	if err := fromUser.Withdraw(tran.Amount); err != nil {
		return err
	}
	if err := toUser.Deposit(tran.Amount); err != nil {
		return err
	}
	return nil
}

// saveUsers 將更新後的帳戶資料寫回資料庫
//
// 參數:
//
//	tx: GORM 資料庫事務
//	users: 待更新的使用者列表
//
// 回傳:
//
//	error: 資料庫寫入錯誤
func (ledger *MySQLLedger) saveUsers(tx *gorm.DB, users []sqlUser) error {
	for _, user := range users {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
	}
	return nil
}

// createTransactionLog 建立交易流水紀錄
//
// 參數:
//
//	tx: GORM 資料庫事務
//	tran: 交易請求物件
//
// 回傳:
//
//	error: 資料庫寫入錯誤
func (ledger *MySQLLedger) createTransactionLog(tx *gorm.DB, tran *domain.Transaction) error {
	transaction := sqlTransaction{
		RefID:         tran.TransactionID[:],
		Sequence:      tran.Sequence,
		FromAccountID: tran.From,
		ToAccountID:   tran.To,
		Amount:        tran.Amount,
		Type:          uint8(tran.Type),
	}
	return tx.Create(&transaction).Error
}

// GetAccountBalance 取得指定帳戶的當前餘額
//
// 參數:
//
//	ctx: 上下文 (Context)
//	accountID: 帳戶 ID
//
// 回傳:
//
//	int64: 帳戶餘額
//	error: 查詢錯誤
func (ledger *MySQLLedger) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	var user sqlUser
	err := ledger.client.DB().Where("id = ?", accountID).First(&user).Error
	if err != nil {
		return 0, err
	}
	return user.Balance, nil
}

// LoadAllAccounts 載入系統所有帳戶資料 (用於初始化 Memory Ledger)
//
// 參數:
//
//	ctx: 上下文 (Context)
//
// 回傳:
//
//	map[int64]*domain.Account: 帳戶 ID 對應的 Domain Account 物件
//	error: 查詢錯誤
func (ledger *MySQLLedger) LoadAllAccounts(ctx context.Context) (map[int64]*domain.Account, error) {
	var users []sqlUser
	if err := ledger.client.DB().Find(&users).Error; err != nil {
		return nil, err
	}

	accountMap := make(map[int64]*domain.Account, len(users))
	for _, u := range users {
		accountMap[u.ID] = &domain.Account{
			ID:      u.ID,
			Balance: u.Balance,
		}
	}
	return accountMap, nil
}

var _ usecase.Ledger = (*MySQLLedger)(nil)
