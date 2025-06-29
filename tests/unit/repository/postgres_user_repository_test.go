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
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func TestPostgresUserRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresUserRepository(db)
	ctx := context.Background()

	t.Run("NilUser", func(t *testing.T) {
		err := repo.Create(ctx, nil)
		assert.ErrorIs(t, err, pkgerrors.ErrNilUser)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UserAlreadyExists", func(t *testing.T) {
		user := &models.User{
			Username:     "testuser",
			PasswordHash: "hash",
			Balance:      1000,
		}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO users`)).
			WithArgs(user.Username, user.PasswordHash, user.Balance).
			WillReturnError(&pq.Error{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(ctx, user)
		assert.ErrorIs(t, err, pkgerrors.ErrUserAlreadyExists)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success", func(t *testing.T) {
		user := &models.User{
			Username:     "testuser",
			PasswordHash: "hash",
			Balance:      1000,
		}
		userID := int32(1)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO users (username, password_hash, balance) VALUES ($1, $2, $3) RETURNING id`)).
			WithArgs(user.Username, user.PasswordHash, user.Balance).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
		mock.ExpectCommit()

		err := repo.Create(ctx, user)
		assert.NoError(t, err)
		assert.Equal(t, userID, user.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("InvalidUser", func(t *testing.T) {
		user := &models.User{
			Username:     "",
			PasswordHash: "hash",
			Balance:      1000,
		}
		err := repo.Create(ctx, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("InvalidUserLongUsername", func(t *testing.T) {
		user := &models.User{
			Username:     string(make([]byte, 51)),
			PasswordHash: "hash",
			Balance:      1000,
		}
		err := repo.Create(ctx, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username too long")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("InvalidUserEmptyPasswordHash", func(t *testing.T) {
		user := &models.User{
			Username:     "testuser",
			PasswordHash: "",
			Balance:      1000,
		}
		err := repo.Create(ctx, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password_hash is required")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RollbackError", func(t *testing.T) {
		user := &models.User{
			Username:     "testuser",
			PasswordHash: "hash",
			Balance:      1000,
		}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO users`)).
			WithArgs(user.Username, user.PasswordHash, user.Balance).
			WillReturnError(fmt.Errorf("database error"))
		mock.ExpectRollback().WillReturnError(fmt.Errorf("rollback error"))

		err := repo.Create(ctx, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rollback failed")
		assert.Contains(t, err.Error(), "database error")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CommitError", func(t *testing.T) {
		user := &models.User{
			Username:     "testuser",
			PasswordHash: "hash",
			Balance:      1000,
		}
		userID := int32(1)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO users`)).
			WithArgs(user.Username, user.PasswordHash, user.Balance).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
		mock.ExpectCommit().WillReturnError(fmt.Errorf("commit error"))

		err := repo.Create(ctx, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit transaction")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresUserRepository_ChangeBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresUserRepository(db)
	ctx := context.Background()

	t.Run("UserNotFound", func(t *testing.T) {
		userID := int32(1)
		amount := int32(100)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		newBalance, err := repo.ChangeBalance(ctx, userID, amount)
		assert.Equal(t, int32(0), newBalance)
		assert.ErrorIs(t, err, pkgerrors.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("InsufficientFunds", func(t *testing.T) {
		userID := int32(1)
		amount := int32(-1000)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		newBalance, err := repo.ChangeBalance(ctx, userID, amount)
		assert.Equal(t, int32(0), newBalance)
		assert.ErrorIs(t, err, pkgerrors.ErrInsufficientFunds)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success", func(t *testing.T) {
		userID := int32(1)
		amount := int32(-100)
		newBalance := int32(900)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(newBalance))
		mock.ExpectCommit()

		result, err := repo.ChangeBalance(ctx, userID, amount)
		assert.NoError(t, err)
		assert.Equal(t, newBalance, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ZeroAmount", func(t *testing.T) {
		userID := int32(1)
		amount := int32(0)
		newBalance := int32(1000)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(newBalance))
		mock.ExpectCommit()

		result, err := repo.ChangeBalance(ctx, userID, amount)
		assert.NoError(t, err)
		assert.Equal(t, newBalance, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RollbackError", func(t *testing.T) {
		userID := int32(1)
		amount := int32(-100)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnError(fmt.Errorf("database error"))
		mock.ExpectRollback().WillReturnError(fmt.Errorf("rollback error"))

		newBalance, err := repo.ChangeBalance(ctx, userID, amount)
		assert.Equal(t, int32(0), newBalance)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rollback failed")
		assert.Contains(t, err.Error(), "database error")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CommitError", func(t *testing.T) {
		userID := int32(1)
		amount := int32(-100)
		newBalance := int32(900)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE users`)).
			WithArgs(amount, userID).
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(newBalance))
		mock.ExpectCommit().WillReturnError(fmt.Errorf("commit error"))

		result, err := repo.ChangeBalance(ctx, userID, amount)
		assert.Equal(t, int32(0), result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit transaction")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresUserRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresUserRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		userID := int32(1)
		expectedUser := &models.User{
			ID:       userID,
			Username: "testuser",
			Balance:  1000,
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, balance FROM users WHERE id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "balance"}).
				AddRow(expectedUser.ID, expectedUser.Username, expectedUser.Balance))

		user, err := repo.GetByID(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, user)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UserNotFound", func(t *testing.T) {
		userID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, balance FROM users WHERE id = $1`)).
			WithArgs(userID).
			WillReturnError(sql.ErrNoRows)

		user, err := repo.GetByID(ctx, userID)
		assert.Nil(t, user)
		assert.ErrorIs(t, err, pkgerrors.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		userID := int32(1)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, balance FROM users WHERE id = $1`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("database error"))

		user, err := repo.GetByID(ctx, userID)
		assert.Nil(t, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user by id")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPostgresUserRepository_GetByUsername(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	repo := repository.NewPostgresUserRepository(db)
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		username := "testuser"
		createdAt := time.Now().UTC()
		expectedUser := &models.User{
			ID:           1,
			Username:     username,
			PasswordHash: "hash",
			Balance:      1000,
			CreatedAt:    createdAt,
		}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, password_hash, balance, created_at FROM users WHERE username = $1`)).
			WithArgs(username).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "balance", "created_at"}).
				AddRow(expectedUser.ID, expectedUser.Username, expectedUser.PasswordHash, expectedUser.Balance, expectedUser.CreatedAt))

		user, err := repo.GetByUsername(ctx, username)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, user)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyUsername", func(t *testing.T) {
		user, err := repo.GetByUsername(ctx, "")
		assert.Nil(t, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username cannot be empty")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("UserNotFound", func(t *testing.T) {
		username := "testuser"
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, password_hash, balance, created_at FROM users WHERE username = $1`)).
			WithArgs(username).
			WillReturnError(sql.ErrNoRows)

		user, err := repo.GetByUsername(ctx, username)
		assert.Nil(t, user)
		assert.ErrorIs(t, err, pkgerrors.ErrUserNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DatabaseError", func(t *testing.T) {
		username := "testuser"
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, password_hash, balance, created_at FROM users WHERE username = $1`)).
			WithArgs(username).
			WillReturnError(fmt.Errorf("database error"))

		user, err := repo.GetByUsername(ctx, username)
		assert.Nil(t, user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user by username")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
