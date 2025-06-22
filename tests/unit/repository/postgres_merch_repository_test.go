package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	repository "github.com/honeynil/MerchServiceTochka-main/internal/repository/postgres"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestPostgresMerchRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresMerchRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		merchID := int32(1)
		expectedMerch := &models.Merch{
			ID:    merchID,
			Price: 1000,
			Name:  "T-shirt",
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, price, name FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "price", "name"}).
				AddRow(expectedMerch.ID, expectedMerch.Price, expectedMerch.Name))

		merch, err := repo.GetByID(ctx, merchID)
		assert.NoError(t, err)
		assert.Equal(t, expectedMerch, merch)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("MerchNotFound", func(t *testing.T) {
		merchID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, price, name FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnError(sql.ErrNoRows)

		merch, err := repo.GetByID(ctx, merchID)
		assert.Nil(t, merch)
		assert.ErrorIs(t, err, pkgerrors.ErrMerchNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		merchID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, price, name FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnError(fmt.Errorf("database error"))

		merch, err := repo.GetByID(ctx, merchID)
		assert.Nil(t, merch)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get merch by id")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresMerchRepository_GetPrice(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresMerchRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		merchID := int32(1)
		expectedPrice := int32(1000)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT price FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnRows(sqlmock.NewRows([]string{"price"}).AddRow(expectedPrice))

		price, err := repo.GetPrice(ctx, merchID)
		assert.NoError(t, err)
		assert.Equal(t, expectedPrice, price)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("MerchNotFound", func(t *testing.T) {
		merchID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT price FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnError(sql.ErrNoRows)

		price, err := repo.GetPrice(ctx, merchID)
		assert.Equal(t, int32(0), price)
		assert.ErrorIs(t, err, pkgerrors.ErrMerchNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		merchID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT price FROM merch WHERE id = $1`)).
			WithArgs(merchID).
			WillReturnError(fmt.Errorf("database error"))

		price, err := repo.GetPrice(ctx, merchID)
		assert.Equal(t, int32(0), price)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get merch price")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
