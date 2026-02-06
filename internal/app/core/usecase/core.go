package usecase

import (
	"context"

	"github.com/JoeShih716/go-mem-ledger/internal/app/core/domain"
)

// CoreUseCase 是核心業務邏輯層
type CoreUseCase struct {
	ledger Ledger
}

func NewCoreUseCase(ledger Ledger) *CoreUseCase {
	return &CoreUseCase{
		ledger: ledger,
	}
}

// PostTransaction 處理交易
func (c *CoreUseCase) PostTransaction(ctx context.Context, tran *domain.Transaction) error {
	return c.ledger.PostTransaction(ctx, tran)
}

// GetAccountBalance 取得帳戶餘額
func (c *CoreUseCase) GetAccountBalance(ctx context.Context, accountID int64) (int64, error) {
	return c.ledger.GetAccountBalance(ctx, accountID)
}
