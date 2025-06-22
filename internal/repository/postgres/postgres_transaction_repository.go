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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type PostgresTransactionRepository struct {
	db *sql.DB
}

func NewPostgresTransactionRepository(db *sql.DB) *PostgresTransactionRepository {
	return &PostgresTransactionRepository{db: db}
}

func (r *PostgresTransactionRepository) Create(ctx context.Context, tx *models.Transaction) (int32, error) {
	var err error
	tracer := otel.Tracer("transaction-repository")
	ctx, span := tracer.Start(ctx, "CreateTransaction")
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("CreateTransaction", status).Inc()
		observability.RepositoryDuration.WithLabelValues("CreateTransaction").Observe(time.Since(start).Seconds())
	}()

	if tx == nil {
		err = pkgerrors.ErrNilTransaction
		slog.Error("failed to create transaction", "method", "Create", "error", err)
		return 0, err
	}

	if tx.Type != models.TypePurchase && tx.Type != models.TypeTransfer {
		err = pkgerrors.ErrInvalidTransactionType
		slog.Error("invalid transaction type", "method", "Create", "type", tx.Type, "error", err)
		return 0, err
	}

	if tx.Status != models.StatusPending && tx.Status != models.StatusCompleted && tx.Status != models.StatusFailed {
		err = pkgerrors.ErrInvalidTransactionStatus
		slog.Error("invalid transaction status", "method", "Create", "status", tx.Status, "error", err)
		return 0, err
	}

	if tx.Amount <= 0 {
		err = fmt.Errorf("amount must be positive")
		slog.Error("amount must be positive", "method", "Create", "amount", tx.Amount, "error", err)
		return 0, err
	}

	span.SetAttributes(
		attribute.Int("user_id", int(tx.UserID)),
		attribute.Int("related_id", int(tx.RelatedID)),
		attribute.Int("amount", int(tx.Amount)),
		attribute.String("type", string(tx.Type)),
		attribute.String("status", string(tx.Status)),
	)

	dbTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "method", "Create", "error", err)
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	query := `INSERT INTO transactions (user_id, related_id, amount, type, status) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	var txID int32
	var createdAt time.Time
	err = dbTx.QueryRowContext(ctx, query, tx.UserID, tx.RelatedID, tx.Amount, tx.Type, tx.Status).Scan(&txID, &createdAt)
	if err != nil {
		if rbErr := dbTx.Rollback(); rbErr != nil {
			err = fmt.Errorf("rollback failed: %v; original error: %w", rbErr, err)
			slog.Error("rollback failed", "method", "Create", "error", rbErr)
		} else {
			slog.Error("failed to create transaction", "method", "Create", "user_id", tx.UserID, "related_id", tx.RelatedID, "type", tx.Type, "status", tx.Status, "error", err)
		}
		return 0, fmt.Errorf("failed to create transaction: %w", err)
	}

	if err = dbTx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "method", "Create", "error", err)
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	tx.ID = txID
	tx.CreatedAt = createdAt
	slog.Info("transaction created", "method", "Create", "id", tx.ID, "user_id", tx.UserID, "related_id", tx.RelatedID, "type", tx.Type, "status", tx.Status)
	return txID, nil
}

func (r *PostgresTransactionRepository) GetByID(ctx context.Context, id int32) (*models.Transaction, error) {
	var err error
	tracer := otel.Tracer("transaction-repository")
	ctx, span := tracer.Start(ctx, "GetTransactionByID")
	span.SetAttributes(attribute.Int("transaction_id", int(id)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetTransactionByID", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetTransactionByID").Observe(time.Since(start).Seconds())
	}()

	var tx models.Transaction
	query := `SELECT id, user_id, related_id, amount, type, status, created_at FROM transactions WHERE id = $1`
	err = r.db.QueryRowContext(ctx, query, id).Scan(&tx.ID, &tx.UserID, &tx.RelatedID, &tx.Amount, &tx.Type, &tx.Status, &tx.CreatedAt)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("transaction not found", "method", "GetByID", "transaction_id", id, "error", err)
		return nil, pkgerrors.ErrTransactionNotFound
	}
	if err != nil {
		slog.Error("failed to get transaction by id", "method", "GetByID", "transaction_id", id, "error", err)
		return nil, fmt.Errorf("failed to get transaction by id: %w", err)
	}

	slog.Info("transaction retrieved", "method", "GetByID", "transaction_id", id, "user_id", tx.UserID, "type", tx.Type)
	return &tx, nil
}

func (r *PostgresTransactionRepository) GetBalance(ctx context.Context, userID int32) (int32, error) {
	var err error
	tracer := otel.Tracer("transaction-repository")
	ctx, span := tracer.Start(ctx, "GetBalance")
	span.SetAttributes(attribute.Int("user_id", int(userID)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetBalance", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetBalance").Observe(time.Since(start).Seconds())
	}()

	var balance int32
	query := `
		SELECT COALESCE(SUM(
			CASE
				WHEN type = 'purchase' AND status = 'completed' THEN -amount
				WHEN type = 'transfer' AND status = 'completed' AND user_id = $1 THEN -amount
				WHEN type = 'transfer' AND status = 'completed' AND related_id = $1 THEN amount
				ELSE 0
			END
		), 0) as balance
		FROM transactions
		WHERE user_id = $1 OR related_id = $1
	`
	err = r.db.QueryRowContext(ctx, query, userID).Scan(&balance)
	if err != nil {
		slog.Error("failed to get balance", "method", "GetBalance", "user_id", userID, "error", err)
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	slog.Info("balance retrieved", "method", "GetBalance", "user_id", userID, "balance", balance)
	return balance, nil
}
