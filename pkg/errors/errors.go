package errors

import (
	"errors"
	"fmt"
)

var (
	ErrUserNotFound                    = errors.New("user not found")
	ErrUserAlreadyExists               = errors.New("user already exists")
	ErrInsufficientFunds               = errors.New("insufficient funds")
	ErrNilUser                         = errors.New("user is nil")
	ErrMerchNotFound                   = errors.New("merch not found")
	ErrNilMerch                        = errors.New("merch is nil")
	ErrNilTransaction                  = errors.New("transaction is nil")
	ErrInvalidTransactionType          = errors.New("invalid transaction type")
	ErrInvalidTransactionStatus        = errors.New("invalid transaction status")
	ErrTransactionNotFound             = errors.New("transaction not found")
	ErrUserNotFoundOrInsufficientFunds = errors.New("user not found or insufficient funds")
	ErrInvalidCredentials              = fmt.Errorf("invalid credentials")
	ErrUsernameExists                  = fmt.Errorf("username already exists")
	ErrInternal                        = fmt.Errorf("error to create user")
	ErrInvalidInput                    = fmt.Errorf("ErrInvalidInput")
)
