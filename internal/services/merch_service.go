package service

import (
	"context"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type MerchService interface {
	Login(ctx context.Context, username, password string) (string, error)   // JWT, 1 час
	Register(ctx context.Context, username, password string) (int32, error) // UserID
	BuyMerch(ctx context.Context, userID, merchID int32) error
	Transfer(ctx context.Context, fromUserID, toUserID, amount int32) error
	GetBalance(ctx context.Context, userID int32) (int32, error)
	GetTransactionHistory(ctx context.Context, userID int32) ([]models.Transaction, error)
}
