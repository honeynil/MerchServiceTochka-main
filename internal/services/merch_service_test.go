package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	kafkamocks "github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/kafka/mocks"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
	redismocks "github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis/mocks"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	repositorymocks "github.com/honeynil/MerchServiceTochka-main/internal/repository/mocks"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestMerchService_Login(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful login", func(t *testing.T) {
		username := "testuser"
		password := "testpass"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		user := &models.User{
			ID:           1,
			Username:     username,
			PasswordHash: string(hashedPassword),
		}

		userRepo.EXPECT().GetByUsername(gomock.Any(), username).Return(user, nil)
		redisClient.EXPECT().Set(gomock.Any(), "user:1:token", gomock.Any(), time.Hour).Return(nil)

		token, err := service.Login(ctx, username, password)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("invalid credentials", func(t *testing.T) {
		username := "testuser"
		password := "wrongpass"

		userRepo.EXPECT().GetByUsername(gomock.Any(), username).Return(nil, errors.New("user not found"))

		token, err := service.Login(ctx, username, password)
		assert.ErrorIs(t, err, pkgerrors.ErrInvalidCredentials)
		assert.Empty(t, token)
	})
}

func TestMerchService_BuyMerch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful purchase", func(t *testing.T) {
		userID := int32(1)
		merchID := int32(1)
		merch := &models.Merch{ID: merchID, Name: "T-shirt", Price: 500}
		balance := int32(1000)
		merchKey := "merch:1"
		balanceKey := "user:1:balance"

		redisClient.EXPECT().Get(gomock.Any(), merchKey).Return("", redis.ErrKeyNotFound)
		merchRepo.EXPECT().GetByID(gomock.Any(), merchID).Return(merch, nil)
		merchJSON, _ := json.Marshal(merch)
		redisClient.EXPECT().Set(gomock.Any(), merchKey, string(merchJSON), time.Hour).Return(nil)
		merchRepo.EXPECT().GetPrice(gomock.Any(), merchID).Return(int32(500), nil)
		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), userID).Return(balance, nil)
		redisClient.EXPECT().Set(gomock.Any(), balanceKey, balance, time.Minute).Return(nil)
		kafkaProducer.EXPECT().Send(gomock.Any(), "transactions", gomock.Any(), gomock.Any()).Return(nil)
		redisClient.EXPECT().Del(gomock.Any(), balanceKey).Return(nil)

		err := service.BuyMerch(ctx, userID, merchID)
		assert.NoError(t, err)
	})

	t.Run("insufficient funds", func(t *testing.T) {
		userID := int32(1)
		merchID := int32(1)
		merch := &models.Merch{ID: merchID, Name: "T-shirt", Price: 1500}
		balance := int32(1000)
		merchKey := "merch:1"
		balanceKey := "user:1:balance"

		redisClient.EXPECT().Get(gomock.Any(), merchKey).Return("", redis.ErrKeyNotFound)
		merchRepo.EXPECT().GetByID(gomock.Any(), merchID).Return(merch, nil)
		merchJSON, _ := json.Marshal(merch)
		redisClient.EXPECT().Set(gomock.Any(), merchKey, string(merchJSON), time.Hour).Return(nil)
		merchRepo.EXPECT().GetPrice(gomock.Any(), merchID).Return(int32(1500), nil)
		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), userID).Return(balance, nil)
		redisClient.EXPECT().Set(gomock.Any(), balanceKey, balance, time.Minute).Return(nil)

		err := service.BuyMerch(ctx, userID, merchID)
		assert.ErrorIs(t, err, pkgerrors.ErrInsufficientFunds)
	})
}
func TestMerchService_Register(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful register", func(t *testing.T) {
		username := "newuser"
		password := "newpass"

		userRepo.EXPECT().GetByUsername(gomock.Any(), username).Return(nil, errors.New("user not found"))
		kafkaProducer.EXPECT().Send(gomock.Any(), "users", gomock.Any(), gomock.Any()).Return(nil)

		eventID, err := service.Register(ctx, username, password)
		assert.NoError(t, err)
		assert.NotEmpty(t, eventID)
	})

	t.Run("username exists", func(t *testing.T) {
		username := "existinguser"
		password := "newpass"
		user := &models.User{ID: 1, Username: username}

		userRepo.EXPECT().GetByUsername(gomock.Any(), username).Return(user, nil)

		eventID, err := service.Register(ctx, username, password)
		assert.ErrorIs(t, err, pkgerrors.ErrUsernameExists)
		assert.Empty(t, eventID)
	})
}

func TestMerchService_Transfer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful transfer", func(t *testing.T) {
		fromUserID := int32(1)
		toUserID := int32(2)
		amount := int32(500)
		balanceKey := "user:1:balance"
		fromUser := &models.User{ID: fromUserID, Username: "fromuser"}
		toUser := &models.User{ID: toUserID, Username: "touser"}

		userRepo.EXPECT().GetByID(gomock.Any(), fromUserID).Return(fromUser, nil)
		userRepo.EXPECT().GetByID(gomock.Any(), toUserID).Return(toUser, nil)
		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), fromUserID).Return(int32(1000), nil)
		redisClient.EXPECT().Set(gomock.Any(), balanceKey, int32(1000), time.Minute).Return(nil)
		kafkaProducer.EXPECT().Send(gomock.Any(), "transactions", gomock.Any(), gomock.Any()).Return(nil)
		redisClient.EXPECT().Del(gomock.Any(), balanceKey).Return(nil)
		redisClient.EXPECT().Del(gomock.Any(), "user:2:balance").Return(nil)

		err := service.Transfer(ctx, fromUserID, toUserID, amount)
		assert.NoError(t, err)
	})

	t.Run("insufficient funds", func(t *testing.T) {
		fromUserID := int32(1)
		toUserID := int32(2)
		amount := int32(1500)
		balanceKey := "user:1:balance"
		fromUser := &models.User{ID: fromUserID, Username: "fromuser"}
		toUser := &models.User{ID: toUserID, Username: "touser"}

		userRepo.EXPECT().GetByID(gomock.Any(), fromUserID).Return(fromUser, nil)
		userRepo.EXPECT().GetByID(gomock.Any(), toUserID).Return(toUser, nil)
		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), fromUserID).Return(int32(1000), nil)
		redisClient.EXPECT().Set(gomock.Any(), balanceKey, int32(1000), time.Minute).Return(nil)

		err := service.Transfer(ctx, fromUserID, toUserID, amount)
		assert.ErrorIs(t, err, pkgerrors.ErrInsufficientFunds)
	})
}

func TestMerchService_GetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful get balance", func(t *testing.T) {
		userID := int32(1)
		balanceKey := "user:1:balance"

		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), userID).Return(int32(1000), nil)
		redisClient.EXPECT().Set(gomock.Any(), balanceKey, int32(1000), time.Minute).Return(nil)

		balance, err := service.GetBalance(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, int32(1000), balance)
	})

	t.Run("user not found", func(t *testing.T) {
		userID := int32(1)
		balanceKey := "user:1:balance"

		redisClient.EXPECT().Get(gomock.Any(), balanceKey).Return("", redis.ErrKeyNotFound)
		transactionRepo.EXPECT().GetBalance(gomock.Any(), userID).Return(int32(0), errors.New("user not found"))

		balance, err := service.GetBalance(ctx, userID)
		assert.Error(t, err)
		assert.Equal(t, int32(0), balance)
	})
}

func TestMerchService_GetTransactionHistory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userRepo := repositorymocks.NewMockUserRepository(ctrl)
	merchRepo := repositorymocks.NewMockMerchRepository(ctrl)
	transactionRepo := repositorymocks.NewMockTransactionRepository(ctrl)
	redisClient := redismocks.NewMockRedisClient(ctrl)
	kafkaProducer := kafkamocks.NewMockKafkaProducer(ctrl)

	ctx := context.Background()
	jwtSecret := "secret"
	service := NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducer, jwtSecret)

	t.Run("successful get history", func(t *testing.T) {
		userID := int32(1)
		transactions := []models.Transaction{
			{ID: 1, UserID: userID, RelatedID: 1, Amount: 500, Type: "purchase", Status: "completed", CreatedAt: time.Now()},
			{ID: 2, UserID: userID, RelatedID: 2, Amount: 300, Type: "transfer", Status: "completed", CreatedAt: time.Now()},
		}

		transactionRepo.EXPECT().GetTransactionHistory(gomock.Any(), userID).Return(transactions, nil)

		result, err := service.GetTransactionHistory(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, transactions, result)
	})

	t.Run("no transactions", func(t *testing.T) {
		userID := int32(1)

		transactionRepo.EXPECT().GetTransactionHistory(gomock.Any(), userID).Return([]models.Transaction{}, nil)

		result, err := service.GetTransactionHistory(ctx, userID)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})
}
