package repository

import (
	"context"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type MerchRepository interface {
	GetByID(ctx context.Context, id int32) (*models.Merch, error)
	GetPrice(ctx context.Context, id int32) (int32, error)
}
