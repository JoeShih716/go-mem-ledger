package domain

import "errors"

var (
	// ErrAmountMustBePositive 金額必須為正數
	ErrAmountMustBePositive = errors.New("amount must be positive")

	// ErrInsufficientBalance 餘額不足
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrAccountNotFound 找不到帳戶
	ErrAccountNotFound = errors.New("account not found")

	// ErrAccountAlreadyExists 帳戶已存在
	ErrAccountAlreadyExists = errors.New("account already exists")

	// ErrTransactionAlreadyProcessed 交易已處理
	ErrTransactionAlreadyProcessed = errors.New("transaction already processed")

	// ErrSelectTransactionFailed 查詢交易失敗
	ErrSelectTransactionFailed = errors.New("select transaction failed")
)
