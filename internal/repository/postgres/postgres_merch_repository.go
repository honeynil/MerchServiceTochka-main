package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type PostgresMerchRepository struct {
	db *sql.DB
}

func NewPostgresMerchRepository(db *sql.DB) *PostgresMerchRepository {
	return &PostgresMerchRepository{db: db}
}

func (r *PostgresMerchRepository) GetByID(ctx context.Context, id int64) (*models.Merch, error) {
	query := `
			SELECT id, price, name
			FROM merch
			WHERE id = $1
`
	var merch models.Merch
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&merch.ID,
		&merch.Price,
		&merch.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get merch: %w", err)
	}
	return &merch, nil

}

func (r *PostgresMerchRepository) GetPrice(ctx context.Context, id int64) (int64, error) {
	var price int64
	query := `SELECT price FROM merch WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&price)
	return price, err
}
