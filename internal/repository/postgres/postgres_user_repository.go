package repository

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/observability"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *models.User) error {
	var err error
	tracer := otel.Tracer("user-repository")
	ctx, span := tracer.Start(ctx, "Create")
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("Create", status).Inc()
		observability.RepositoryDuration.WithLabelValues("Create").Observe(time.Since(start).Seconds())
	}()

	if user == nil {
		err = pkgerrors.ErrNilUser
		slog.Error("failed to create user", "method", "Create", "error", err)
		return err
	}

	span.SetAttributes(attribute.String("username", user.Username))

	if err = user.Validate(); err != nil {
		slog.Error("failed to create user", "method", "Create", "error", err)
		return fmt.Errorf("invalid user: %w", err)
	}
	const defaultBalance = 1000

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "method", "Create", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
				slog.Error("rollback failed", "method", "Create", "error", rbErr)
			}
		} else {
			if commitErr := tx.Commit(); commitErr != nil {
				err = fmt.Errorf("failed to commit transaction: %w", commitErr)
				slog.Error("failed to commit transaction", "method", "Create", "error", commitErr)
			}
		}
	}()

	query := `INSERT INTO users (username, password_hash, balance) VALUES ($1, $2, $3) RETURNING id, created_at`
	err = tx.QueryRowContext(ctx, query, user.Username, user.PasswordHash, defaultBalance).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			slog.Error("username already exists", "method", "Create", "username", user.Username, "error", err)
			return pkgerrors.ErrUserAlreadyExists
		}
		slog.Error("failed to create user", "method", "Create", "username", user.Username, "error", err)
		return fmt.Errorf("failed to create user: %w", err)
	}

	slog.Info("user created", "method", "Create", "username", user.Username, "id", user.ID)
	return nil
}

func (r *PostgresUserRepository) ChangeBalance(ctx context.Context, userID, amount int32) (newBalance int32, err error) {
	tracer := otel.Tracer("user-repository")
	ctx, span := tracer.Start(ctx, "ChangeBalance")
	span.SetAttributes(attribute.Int("user_id", int(userID)), attribute.Int("amount", int(amount)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("ChangeBalance", status).Inc()
		observability.RepositoryDuration.WithLabelValues("ChangeBalance").Observe(time.Since(start).Seconds())
	}()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "method", "ChangeBalance", "user_id", userID, "error", err)
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
				slog.Error("rollback failed", "method", "ChangeBalance", "error", rbErr)
			}
		} else {
			if commitErr := tx.Commit(); commitErr != nil {
				err = fmt.Errorf("failed to commit transaction: %w", commitErr)
				slog.Error("failed to commit transaction", "method", "ChangeBalance", "error", commitErr)
			}
		}
	}()

	query := `UPDATE users SET balance = balance + $1 WHERE id = $2 AND (balance + $1) >= 0 RETURNING balance`
	err = tx.QueryRowContext(ctx, query, amount, userID).Scan(&newBalance)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("user not found or insufficient funds", "method", "ChangeBalance", "user_id", userID, "amount", amount, "error", err)
		if amount < 0 {
			return 0, pkgerrors.ErrInsufficientFunds
		}
		return 0, pkgerrors.ErrUserNotFound
	}
	if err != nil {
		slog.Error("failed to update balance", "method", "ChangeBalance", "user_id", userID, "error", err)
		return 0, fmt.Errorf("failed to update balance: %w", err)
	}

	slog.Info("balance updated", "method", "ChangeBalance", "user_id", userID, "amount", amount, "new_balance", newBalance)
	return newBalance, nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id int32) (*models.User, error) {
	var err error
	tracer := otel.Tracer("user-repository")
	ctx, span := tracer.Start(ctx, "GetByID")
	span.SetAttributes(attribute.Int("user_id", int(id)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetByID", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetByID").Observe(time.Since(start).Seconds())
	}()

	query := `SELECT id, username, balance FROM users WHERE id = $1`
	var user models.User
	err = r.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Username, &user.Balance)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("user not found", "method", "GetByID", "id", id, "error", err)
		return nil, pkgerrors.ErrUserNotFound
	}
	if err != nil {
		slog.Error("failed to get user by id", "method", "GetByID", "id", id, "error", err)
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	slog.Info("user retrieved by id", "method", "GetByID", "id", id, "username", user.Username)
	return &user, nil
}

func (r *PostgresUserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var err error
	tracer := otel.Tracer("user-repository")
	ctx, span := tracer.Start(ctx, "GetByUsername")
	span.SetAttributes(attribute.String("username", username))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetByUsername", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetByUsername").Observe(time.Since(start).Seconds())
	}()

	if username == "" {
		err = fmt.Errorf("username cannot be empty")
		slog.Error("failed to get user by username", "method", "GetByUsername", "error", err)
		return nil, err
	}

	query := `SELECT id, username, password_hash, balance, created_at FROM users WHERE username = $1`
	var user models.User
	err = r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Balance,
		&user.CreatedAt,
	)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("user not found", "method", "GetByUsername", "username", username, "error", err)
		return nil, pkgerrors.ErrUserNotFound
	}
	if err != nil {
		slog.Error("failed to get user by username", "method", "GetByUsername", "username", username, "error", err)
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}
	slog.Info("user retrieved by username", "method", "GetByUsername", "username", username, "id", user.ID)
	return &user, nil
}
