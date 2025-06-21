package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/honeynil/MerchServiceTochka-main/internal/models"
)

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepositort(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *models.User) error {
	// impl here
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	if user.Username == "" || user.PasswordHash == "" {
		return fmt.Errorf("username and password are required")
	}
	const defaultBalance = 1000

	query := `
	INSERT INTO users (username, password_hash, balance)
	VALUES ($1, $2, $3)
	RETURNING id, created_at
	`
	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Username,
		user.PasswordHash,
		defaultBalance,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}
	return nil

}

func (r *PostgresUserRepository) ChangeBalance(ctx context.Context, userID, amount int32) (newBalance int32, err error) {
	query := `
		UPDATE users
		SET balance = balance + $1
		WHERE id = $2
		AND (balance +$1) >= 0
		RETURNING balance
		`

	err = r.db.QueryRowContext(ctx, query, amount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("user %d not found or insufficient funds (needed: %d)", userID, -amount)
	}
	return newBalance, nil

}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id int32) (*models.User, error) {
	query := `SELECT id, username, balance FROM users WHERE id = $1`
	var user models.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Username, &user.Balance)
	return &user, err

}

func (r *PostgresUserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	query := ` SELECT id, username, password_hash, balance, created_at FROM users WHERE username = $1`

	var user models.User

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Balance,
		&user.CreatedAt,
	)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("user '%s' not found", username)
	case err != nil:
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// func (r *PostgresUserRepository) InitBalance(ctx context.Context, userID, startBalance int32) {}
