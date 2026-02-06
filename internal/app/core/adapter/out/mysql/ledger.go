package mysql

import (
	"context"
	"fmt"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
	"github.com/JoeShih716/go-mem-ledger/internal/app/core/usecase"
	"github.com/JoeShih716/go-mem-ledger/pkg/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// sqlUser 對應資料庫的 users 表
type sqlUser struct {
	ID        int64 `gorm:"primaryKey"`
	Balance   int64
	UpdatedAt int64 `gorm:"autoUpdateTime:milli"` // 自動更新時間
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

func NewMySQLLedger(client *mysql.Client) *MySQLLedger {
	return &MySQLLedger{
		client: client,
	}
}

func (ledger *MySQLLedger) PostTransaction(ctx context.Context, tran *domain.Transaction) error {
	err := ledger.client.DB().Transaction(func(tx *gorm.DB) error {
		// 先檢查是否有這筆交易記錄
		var transaction sqlTransaction
		err := tx.Where("ref_id = ?", tran.TransactionID[:]).First(&transaction).Error
		if err == nil {
			fmt.Println("已有該紀錄 回傳成功")
			return nil
		}
		if err == gorm.ErrRecordNotFound {
			fmt.Println("沒有該紀錄 可以繼續執行")
		} else {
			fmt.Printf("查詢交易錯誤: %v\n", err)
			return domain.ErrSelectTransactionFailed
		}
		// 取得鎖定帳號 以及lockID 悲觀鎖
		lockIDs := tran.GetLockIDs()
		var users []sqlUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", lockIDs).
			Find(&users).Error; err != nil {
			return err
		}
		userMap := make(map[int64]*sqlUser)
		for i := range users {
			userMap[users[i].ID] = &users[i]
		}
		// 安全檢查：確保涉及的帳號都存在
		switch tran.Type {
		case domain.TransactionTypeTransfer:
			if _, ok := userMap[tran.From]; !ok {
				return domain.ErrAccountNotFound
			}
			if _, ok := userMap[tran.To]; !ok {
				return domain.ErrAccountNotFound
			}
		case domain.TransactionTypeWithdraw:
			if _, ok := userMap[tran.From]; !ok {
				return domain.ErrAccountNotFound
			}
		case domain.TransactionTypeDeposit:
			if _, ok := userMap[tran.To]; !ok {
				return domain.ErrAccountNotFound
			}
		}
		// 依照 Type 執行業務邏輯 (Deposit/Withdraw/Transfer) ，扣款的需檢查餘額
		switch tran.Type {
		case domain.TransactionTypeDeposit:
			userMap[tran.To].Balance += tran.Amount
		case domain.TransactionTypeWithdraw:
			if userMap[tran.From].Balance < tran.Amount {
				return domain.ErrInsufficientBalance
			}
			userMap[tran.From].Balance -= tran.Amount
		case domain.TransactionTypeTransfer:
			if userMap[tran.From].Balance < tran.Amount {
				return domain.ErrInsufficientBalance
			}
			userMap[tran.From].Balance -= tran.Amount
			userMap[tran.To].Balance += tran.Amount
		}
		// 更新資料庫
		for _, user := range users {
			if err := tx.Save(&user).Error; err != nil {
				return err
			}
		}
		// 建立交易紀錄
		transaction = sqlTransaction{
			RefID:         tran.TransactionID[:],
			Sequence:      tran.Sequence,
			FromAccountID: tran.From,
			ToAccountID:   tran.To,
			Amount:        tran.Amount,
			Type:          uint8(tran.Type),
		}
		if err := tx.Create(&transaction).Error; err != nil {
			return err
		}
		return nil
	})
	return err
}

// GetAccountBalance 取得帳戶餘額
func (ledger *MySQLLedger) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	var user sqlUser
	err := ledger.client.DB().Where("id = ?", accountID).First(&user).Error
	if err != nil {
		return 0, err
	}
	return user.Balance, nil
}

var _ usecase.Ledger = (*MySQLLedger)(nil)
