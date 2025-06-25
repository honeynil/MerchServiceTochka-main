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
	ctx, span := tracer.Start(ctx, "CreateUser")
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("CreateUser", status).Inc()
		observability.RepositoryDuration.WithLabelValues("CreateUser").Observe(time.Since(start).Seconds())
	}()

	if user == nil {
		err = pkgerrors.ErrNilUser
		slog.Error("failed to create user", "method", "Create", "error", err)
		return err
	}

	if err = user.Validate(); err != nil {
		slog.Error("failed to create user", "method", "Create", "error", err)
		return err
	}

	span.SetAttributes(
		attribute.String("username", user.Username),
		attribute.Int("balance", int(user.Balance)),
	)

	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "method", "Create", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	query := `INSERT INTO users (username, password_hash, balance) VALUES ($1, $2, $3) RETURNING id`
	var userID int32
	err = dbTx.QueryRowContext(ctx, query, user.Username, user.PasswordHash, user.Balance).Scan(&userID)
	if err != nil {
		if rbErr := dbTx.Rollback(); rbErr != nil {
			err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
			slog.Error("rollback failed", "method", "Create", "error", rbErr)
		} else {
			slog.Error("failed to create user", "method", "Create", "username", user.Username, "error", err)
		}
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			err = pkgerrors.ErrUserAlreadyExists
			slog.Error("username already exists", "method", "Create", "username", user.Username, "error", err)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Создаём начальную транзакцию
	_, err = dbTx.ExecContext(ctx,
		"INSERT INTO transactions (user_id, type, status, amount, created_at) VALUES ($1, $2, $3, $4, $5)",
		userID, "initial", "completed", user.Balance, time.Now())
	if err != nil {
		if rbErr := dbTx.Rollback(); rbErr != nil {
			err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
			slog.Error("rollback failed", "method", "Create", "error", rbErr)
		}
		slog.Error("failed to create initial transaction", "method", "Create", "user_id", userID, "error", err)
		return fmt.Errorf("failed to create initial transaction: %w", err)
	}

	if err = dbTx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "method", "Create", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	user.ID = userID
	slog.Info("user created", "method", "Create", "username", user.Username, "id", user.ID)
	return nil
}

func (r *PostgresUserRepository) ChangeBalance(ctx context.Context, userID, amount int32) (int32, error) {
	var err error
	tracer := otel.Tracer("user-repository")
	ctx, span := tracer.Start(ctx, "ChangeBalance")
	span.SetAttributes(
		attribute.Int("user_id", int(userID)),
		attribute.Int("amount", int(amount)),
	)
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

	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "method", "ChangeBalance", "error", err)
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	query := `
		UPDATE users
		SET balance = balance + $1
		WHERE id = $2 AND (balance + $1) >= 0
		RETURNING balance
	`
	var newBalance int32
	err = dbTx.QueryRowContext(ctx, query, amount, userID).Scan(&newBalance)
	if err != nil {
		if rbErr := dbTx.Rollback(); rbErr != nil {
			err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
			slog.Error("rollback failed", "method", "ChangeBalance", "error", rbErr)
		} else {
			slog.Error("failed to update balance", "method", "ChangeBalance", "user_id", userID, "error", err)
		}
		if err == sql.ErrNoRows {
			err = pkgerrors.ErrUserNotFoundOrInsufficientFunds
			slog.Error("user not found or insufficient funds", "method", "ChangeBalance", "user_id", userID, "amount", amount, "error", err)
		}
		return 0, fmt.Errorf("failed to update balance: %w", err)
	}

	if err = dbTx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "method", "ChangeBalance", "error", err)
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("balance updated", "method", "ChangeBalance", "user_id", userID, "amount", amount, "new_balance", newBalance)
	return newBalance, nil
}

func (r *PostgresUserRepository) GetBalance(ctx context.Context, userID int32) (int32, error) {
	var balance int32
	query := `SELECT balance FROM users WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&balance)
	if err != nil {
		slog.Error("failed to get balance", "user_id", userID, "error", err)
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}
	slog.Info("balance retrieved", "user_id", userID, "balance", balance)
	return balance, nil
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
