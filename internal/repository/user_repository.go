package repository

import (
	"context"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type UserRepository interface {
	// Основные операции
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int32) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)

	// Управление балансом
	ChangeBalance(ctx context.Context, userID, delta int32) (newBalance int32, err error)
}
