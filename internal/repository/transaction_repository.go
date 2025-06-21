package repository

import (
	"context"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type TransactionRepository interface {
	Create(ctx context.Context, tx *models.Transaction) (int32, error)
	GetByID(ctx context.Context, id int32) (*models.Transaction, error)
	GetBalance(ctx context.Context, userID int32) (int32, error)
}
