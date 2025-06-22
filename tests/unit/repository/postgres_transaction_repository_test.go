package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	repository "github.com/honeynil/MerchServiceTochka-main/internal/repository/postgres"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestPostgresTransactionRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresTransactionRepository(db)
	ctx := context.Background()

	t.Run("NilTransaction", func(t *testing.T) {
		id, err := repo.Create(ctx, nil)
		assert.Equal(t, int32(0), id)
		assert.ErrorIs(t, err, pkgerrors.ErrNilTransaction)
	})

	t.Run("InvalidType", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      "invalid",
			Status:    models.StatusCompleted,
		}
		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.ErrorIs(t, err, pkgerrors.ErrInvalidTransactionType)
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    "invalid",
		}
		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.ErrorIs(t, err, pkgerrors.ErrInvalidTransactionStatus)
	})

	t.Run("InvalidAmount", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    0,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
		}
		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "amount must be positive")
	})

	t.Run("Success", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
		}
		createdAt := time.Now().UTC()
		txID := int32(1)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO transactions (user_id, related_id, amount, type, status) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`)).
			WithArgs(tx.UserID, tx.RelatedID, tx.Amount, tx.Type, tx.Status).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(txID, createdAt))
		mock.ExpectCommit()

		id, err := repo.Create(ctx, tx)
		assert.NoError(t, err)
		assert.Equal(t, txID, id)
		assert.Equal(t, txID, tx.ID)
		assert.WithinDuration(t, createdAt, tx.CreatedAt, time.Second)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
		}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO transactions`)).
			WithArgs(tx.UserID, tx.RelatedID, tx.Amount, tx.Type, tx.Status).
			WillReturnError(fmt.Errorf("database error"))
		mock.ExpectRollback()

		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create transaction")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RollbackError", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
		}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO transactions`)).
			WithArgs(tx.UserID, tx.RelatedID, tx.Amount, tx.Type, tx.Status).
			WillReturnError(fmt.Errorf("database error"))
		mock.ExpectRollback().WillReturnError(fmt.Errorf("rollback error"))

		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rollback failed")
		assert.Contains(t, err.Error(), "database error")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CommitError", func(t *testing.T) {
		tx := &models.Transaction{
			UserID:    1,
			RelatedID: 1,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
		}
		createdAt := time.Now().UTC()
		txID := int32(1)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO transactions`)).
			WithArgs(tx.UserID, tx.RelatedID, tx.Amount, tx.Type, tx.Status).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(txID, createdAt))
		mock.ExpectCommit().WillReturnError(fmt.Errorf("commit error"))

		id, err := repo.Create(ctx, tx)
		assert.Equal(t, int32(0), id)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit transaction")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresTransactionRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresTransactionRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		txID := int32(1)
		createdAt := time.Now().UTC()
		expectedTx := &models.Transaction{
			ID:        txID,
			UserID:    1,
			RelatedID: 2,
			Amount:    500,
			Type:      models.TypePurchase,
			Status:    models.StatusCompleted,
			CreatedAt: createdAt,
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, user_id, related_id, amount, type, status, created_at FROM transactions WHERE id = $1`)).
			WithArgs(txID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "related_id", "amount", "type", "status", "created_at"}).
				AddRow(expectedTx.ID, expectedTx.UserID, expectedTx.RelatedID, expectedTx.Amount, expectedTx.Type, expectedTx.Status, expectedTx.CreatedAt))

		tx, err := repo.GetByID(ctx, txID)
		assert.NoError(t, err)
		assert.Equal(t, expectedTx, tx)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("TransactionNotFound", func(t *testing.T) {
		txID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, user_id, related_id, amount, type, status, created_at FROM transactions WHERE id = $1`)).
			WithArgs(txID).
			WillReturnError(sql.ErrNoRows)

		tx, err := repo.GetByID(ctx, txID)
		assert.Nil(t, tx)
		assert.ErrorIs(t, err, pkgerrors.ErrTransactionNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		txID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, user_id, related_id, amount, type, status, created_at FROM transactions WHERE id = $1`)).
			WithArgs(txID).
			WillReturnError(fmt.Errorf("database error"))

		tx, err := repo.GetByID(ctx, txID)
		assert.Nil(t, tx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get transaction by id")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresTransactionRepository_GetBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresTransactionRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		userID := int32(1)
		expectedBalance := int32(1000)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(expectedBalance))

		balance, err := repo.GetBalance(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, expectedBalance, balance)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		userID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(SUM`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("database error"))

		balance, err := repo.GetBalance(ctx, userID)
		assert.Equal(t, int32(0), balance)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get balance")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
