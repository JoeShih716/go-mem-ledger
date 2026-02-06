package usecase

import (
	"context"

	"github.com/JoeShih716/go-mem-ledger/internal/domain"
)

type Ledger interface {
	// 不再分 Deposit/Withdraw，直接看 tran.Type 決定
	PostTransaction(ctx context.Context, tran *domain.Transaction) error
	// GetAccountBalance 取得帳戶餘額
	GetAccountBalance(ctx context.Context, accountID int64) (int64, error)
}
