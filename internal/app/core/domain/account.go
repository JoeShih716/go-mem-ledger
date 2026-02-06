package domain

type Account struct {
	ID      int64
	Balance int64
}

func NewAccount(id int64, balance int64) *Account {
	return &Account{
		ID:      id,
		Balance: balance,
	}
}

// Deposit 存款
func (a *Account) Deposit(amount int64) error {
	if amount < 0 {
		return ErrAmountMustBePositive
	}

	a.Balance = a.Balance + amount
	return nil
}

// Withdraw 提款
func (a *Account) Withdraw(amount int64) error {
	if amount < 0 {
		return ErrAmountMustBePositive
	}

	if a.Balance < amount {
		return ErrInsufficientBalance
	}

	a.Balance = a.Balance - amount
	return nil
}
