package errors

import (
	"errors"
)

var (
	// users errors
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrNilUser           = errors.New("user is nil")
	ErrUsernameExists    = errors.New("username already exists")
	ErrInternal          = errors.New("error to create user")
	// merch errors
	ErrMerchNotFound = errors.New("merch not found")
	ErrNilMerch      = errors.New("merch is nil")
	// trasactions errors
	ErrNilTransaction           = errors.New("transaction is nil")
	ErrInvalidTransactionType   = errors.New("invalid transaction type")
	ErrInvalidTransactionStatus = errors.New("invalid transaction status")
	ErrTransactionNotFound      = errors.New("transaction not found")
	// async & other errors
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrInvalidInput            = errors.New("ErrInvalidInput")
	ErrRequestAlreadyProcessed = errors.New("request alredy processed")
	ErrBalanceLocked           = errors.New("user balance is being processed")
)
