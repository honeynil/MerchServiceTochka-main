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

type PostgresMerchRepository struct {
	db *sql.DB
}

func NewPostgresMerchRepository(db *sql.DB) *PostgresMerchRepository {
	return &PostgresMerchRepository{db: db}
}

func (r *PostgresMerchRepository) GetByID(ctx context.Context, id int32) (*models.Merch, error) {
	var err error
	tracer := otel.Tracer("merch-repository")
	ctx, span := tracer.Start(ctx, "GetMerchByID")
	span.SetAttributes(attribute.Int("merch_id", int(id)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetMerchByID", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetMerchByID").Observe(time.Since(start).Seconds())
	}()

	query := `SELECT id, price, name FROM merch WHERE id = $1`
	var merch models.Merch
	err = r.db.QueryRowContext(ctx, query, id).Scan(&merch.ID, &merch.Price, &merch.Name)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("merch not found", "method", "GetByID", "merch_id", id, "error", err)
		return nil, pkgerrors.ErrMerchNotFound
	}
	if err != nil {
		slog.Error("failed to get merch by id", "method", "GetByID", "merch_id", id, "error", err)
		return nil, fmt.Errorf("failed to get merch by id: %w", err)
	}

	slog.Info("merch retrieved by id", "method", "GetByID", "merch_id", id, "name", merch.Name)
	return &merch, nil
}

func (r *PostgresMerchRepository) GetPrice(ctx context.Context, id int32) (int32, error) {
	var err error
	tracer := otel.Tracer("merch-repository")
	ctx, span := tracer.Start(ctx, "GetMerchPrice")
	span.SetAttributes(attribute.Int("merch_id", int(id)))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("GetMerchPrice", status).Inc()
		observability.RepositoryDuration.WithLabelValues("GetMerchPrice").Observe(time.Since(start).Seconds())
	}()

	var price int32
	query := `SELECT price FROM merch WHERE id = $1`
	err = r.db.QueryRowContext(ctx, query, id).Scan(&price)
	if stderrors.Is(err, sql.ErrNoRows) {
		slog.Error("merch not found", "method", "GetPrice", "merch_id", id, "error", err)
		return 0, pkgerrors.ErrMerchNotFound
	}
	if err != nil {
		slog.Error("failed to get merch price", "method", "GetPrice", "merch_id", id, "error", err)
		return 0, fmt.Errorf("failed to get merch price: %w", err)
	}

	slog.Info("merch price retrieved", "method", "GetPrice", "merch_id", id, "price", price)
	return price, nil
}
