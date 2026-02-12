package domain

import "github.com/google/uuid"

// amount 使用int64，並定義精度：小數點後 4 位
const (
	CurrencyScale = 10000
)

// TransactionType 交易類型
// 為了極致節省記憶體，使用 uint8
type TransactionType uint8

const (
	// 存款
	TransactionTypeDeposit TransactionType = 1
	// 提款
	TransactionTypeWithdraw TransactionType = 2
	// 轉帳
	TransactionTypeTransfer TransactionType = 3
)

// Transaction 交易 注意欄位排序以避免 Padding
type Transaction struct {
	// Sequence: 全局唯一的順序號 (由核心引擎分配，1, 2, 3...)
	// 用於 WAL 重放確保順序一致
	Sequence uint64
	// From, To: 帳戶 ID
	From int64
	To   int64
	// Amount: 金額
	Amount int64
	// CreatedAt: 交易時間
	CreatedAt int64
	// TransactionID: 外部追蹤號 (UUID)
	TransactionID uuid.UUID
	// Type: 放到最後面，利用 Padding 空間
	Type TransactionType
}

// GetLockIDs 回傳需要鎖定的帳號 ID，並確保順序以避免死鎖
func (t *Transaction) GetLockIDs() (ids []int64) {
	// 預先宣告一個容量為 2 的 slice，避免多次分配
	// make([]Type, len, cap)
	ids = make([]int64, 0, 2)
	switch t.Type {
	case TransactionTypeTransfer:
		if t.From < t.To {
			ids = append(ids, t.From, t.To)
		} else {
			ids = append(ids, t.To, t.From)
		}
	case TransactionTypeDeposit:
		ids = append(ids, t.To)
	case TransactionTypeWithdraw:
		ids = append(ids, t.From)
	}
	return ids
}
