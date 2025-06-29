package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	stderrors "errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/kafka"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	"github.com/honeynil/MerchServiceTochka-main/internal/repository"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"
)

type MerchService interface {
	Login(ctx context.Context, username, password string) (string, error)
	Register(ctx context.Context, username, password string) (string, error)
	BuyMerch(ctx context.Context, userID, merchID int32, requestID string) error
	Transfer(ctx context.Context, fromUserID, toUserID, amount int32) error
	GetBalance(ctx context.Context, userID int32) (int32, error)
	GetTransactionHistory(ctx context.Context, userID int32) ([]models.Transaction, error)
}

type merchService struct {
	userRepo                  repository.UserRepository
	merchRepo                 repository.MerchRepository
	transactionRepo           repository.TransactionRepository
	redisClient               redis.RedisClient
	kafkaProducerUsers        *kafka.Producer
	kafkaProducerTransactions *kafka.Producer
	jwtSecret                 string
}

func NewMerchService(
	userRepo repository.UserRepository,
	merchRepo repository.MerchRepository,
	transactionRepo repository.TransactionRepository,
	redisClient redis.RedisClient,
	kafkaProducerUsers *kafka.Producer,
	kafkaProducerTransactions *kafka.Producer,
	jwtSecret string,
) *merchService {
	return &merchService{
		userRepo:                  userRepo,
		merchRepo:                 merchRepo,
		transactionRepo:           transactionRepo,
		redisClient:               redisClient,
		kafkaProducerUsers:        kafkaProducerUsers,
		kafkaProducerTransactions: kafkaProducerTransactions,
		jwtSecret:                 jwtSecret,
	}
}

func (s *merchService) Register(ctx context.Context, username, password string) (string, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "Register")
	defer span.End()

	if username == "" || password == "" {
		span.SetStatus(codes.Error, "empty username or password")
		return "", pkgerrors.ErrInvalidInput
	}

	existingUser, err := s.userRepo.GetByUsername(ctx, username)
	if existingUser != nil {
		span.SetStatus(codes.Error, "username already exists")
		slog.Warn("username already exists",
			"username", username,
			"existing_id", existingUser.ID)
		return "", pkgerrors.ErrUsernameExists
	}
	if err != nil && !stderrors.Is(err, pkgerrors.ErrUserNotFound) {
		span.RecordError(err)
		span.SetStatus(codes.Error, "user check failed")
		slog.Error("failed to check user existence",
			"username", username,
			"error", err)
		return "", fmt.Errorf("%w: failed to check user existence", pkgerrors.ErrInternal)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "password hashing failed")
		slog.Error("failed to hash password",
			"username", username,
			"error", err)
		return "", fmt.Errorf("%w: failed to hash password", pkgerrors.ErrInternal)
	}

	user := &models.User{
		Username:     username,
		PasswordHash: string(hash),
		Balance:      5000,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "user creation failed")
		slog.Error("failed to create user in DB",
			"username", username,
			"error", err)
		return "", fmt.Errorf("%w: failed to create user", pkgerrors.ErrInternal)
	}

	event := map[string]interface{}{
		"event_type":    "user_registered",
		"user_id":       user.ID,
		"username":      username,
		"password_hash": string(hash),
		"created_at":    time.Now().UTC().Format(time.RFC3339),
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		slog.Error("failed to marshal kafka event",
			"user_id", user.ID,
			"error", err)
	} else {
		go func() {
			retries := 3
			for i := 0; i < retries; i++ {
				if err := s.kafkaProducerUsers.Send(context.Background(), "users", int64(user.ID), eventBytes); err == nil {
					slog.Info("user registration event sent",
						"user_id", user.ID,
						"username", username)
					return
				}
				time.Sleep(time.Second * time.Duration(i+1))
			}
			slog.Error("failed to send user registration event after retries",
				"user_id", user.ID,
				"username", username)
		}()
	}

	slog.Info("user registered successfully",
		"user_id", user.ID,
		"username", username)

	return fmt.Sprintf("%d", user.ID), nil
}

func (s *merchService) Login(ctx context.Context, username, password string) (string, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "Login")
	defer span.End()

	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		slog.Error("failed to login", "username", username, "error", err)
		return "", pkgerrors.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		slog.Error("invalid password", "username", username)
		return "", pkgerrors.ErrInvalidCredentials
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		slog.Error("failed to generate JWT", "error", err)
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	if err := s.redisClient.Set(ctx, fmt.Sprintf("user:%d:token", user.ID), tokenString, time.Hour); err != nil {
		slog.Error("failed to cache JWT", "user_id", user.ID, "error", err)
	}

	slog.Info("user logged in", "username", username, "user_id", user.ID)
	return tokenString, nil
}

func (s *merchService) BuyMerch(ctx context.Context, userID, merchID int32, requestID string) error {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "BuyMerch")
	defer span.End()

	requestKey := fmt.Sprintf("request:%s", requestID)
	if val, err := s.redisClient.Get(ctx, requestKey); err == nil {
		slog.Error("request already processed", "request_id", requestID, "user_id", userID, "status", val)
		span.SetStatus(codes.Error, "request already processed")
		return pkgerrors.ErrRequestAlreadyProcessed
	}

	if err := s.redisClient.Set(ctx, requestKey, "pending", 24*time.Hour); err != nil {
		slog.Error("failed to set request key", "request_id", requestID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set request key")
		return err
	}

	lockKey := fmt.Sprintf("user:%d:lock", userID)
	ok, err := s.redisClient.SetNX(ctx, lockKey, "locked", 3*time.Second)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		slog.Error("failed to acquire lock", "user_id", userID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to acquire lock")
		return pkgerrors.ErrBalanceLocked
	}
	if !ok {
		s.redisClient.Del(ctx, requestKey)
		slog.Error("balance is locked", "user_id", userID)
		span.SetStatus(codes.Error, "balance is locked")
		return pkgerrors.ErrBalanceLocked
	}

	merchKey := fmt.Sprintf("merch:%d", merchID)
	var merch *models.Merch
	merchJSON, err := s.redisClient.Get(ctx, merchKey)
	if err != nil && !stderrors.Is(err, redis.ErrKeyNotFound) {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("failed to get merch from Redis", "merch_id", merchID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get merch from Redis")
		return err
	}
	if stderrors.Is(err, redis.ErrKeyNotFound) {
		merch, err = s.merchRepo.GetByID(ctx, merchID)
		if err != nil {
			s.redisClient.Del(ctx, requestKey)
			s.redisClient.Del(ctx, lockKey)
			slog.Error("merch not found", "merch_id", merchID, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "merch not found")
			return err
		}
		merchBytes, err := json.Marshal(merch)
		if err != nil {
			s.redisClient.Del(ctx, requestKey)
			s.redisClient.Del(ctx, lockKey)
			slog.Error("failed to marshal merch", "merch_id", merchID, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to marshal merch")
			return err
		}
		if err := s.redisClient.Set(ctx, merchKey, string(merchBytes), 24*time.Hour); err != nil {
			slog.Error("failed to cache merch", "merch_id", merchID, "error", err)
			span.RecordError(err)
		}
	} else {
		if err := json.Unmarshal([]byte(merchJSON), &merch); err != nil {
			s.redisClient.Del(ctx, requestKey)
			s.redisClient.Del(ctx, lockKey)
			slog.Error("failed to unmarshal merch", "merch_id", merchID, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to unmarshal merch")
			return err
		}
	}

	price, err := s.merchRepo.GetPrice(ctx, merchID)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("failed to get merch price", "merch_id", merchID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get merch price")
		return err
	}

	balance, err := s.userRepo.GetBalance(ctx, userID)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("failed to get balance from Postgres", "user_id", userID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get balance")
		return err
	}

	if balance < price {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("insufficient funds", "user_id", userID, "balance", balance, "price", price)
		span.SetStatus(codes.Error, "insufficient funds")
		return pkgerrors.ErrInsufficientFunds
	}

	event := map[string]interface{}{
		"user_id":    userID,
		"merch_id":   merchID,
		"amount":     -price,
		"type":       "purchase",
		"status":     "pending",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"request_id": requestID,
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("failed to marshal Kafka event", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal event")
		return fmt.Errorf("failed to marshal Kafka event: %w", err)
	}
	eventID := time.Now().UnixNano()
	if err := s.kafkaProducerTransactions.Send(ctx, "transactions", eventID, eventBytes); err != nil {
		s.redisClient.Del(ctx, requestKey)
		s.redisClient.Del(ctx, lockKey)
		slog.Error("failed to send Kafka event", "event_id", eventID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send Kafka event")
		return fmt.Errorf("failed to send Kafka event: %w", err)
	}

	slog.Info("buy event sent", "user_id", userID, "merch_id", merchID, "event_id", eventID, "request_id", requestID)
	return nil
}

func (s *merchService) Transfer(ctx context.Context, fromUserID, toUserID, amount int32) error {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "Transfer")
	defer span.End()

	if amount <= 0 {
		slog.Error("invalid transfer amount", "amount", amount)
		span.SetStatus(codes.Error, "invalid amount")
		return fmt.Errorf("amount must be positive")
	}

	_, err := s.userRepo.GetByID(ctx, fromUserID)
	if err != nil {
		slog.Error("sender not found", "user_id", fromUserID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "sender not found")
		return err
	}
	_, err = s.userRepo.GetByID(ctx, toUserID)
	if err != nil {
		slog.Error("receiver not found", "user_id", toUserID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "receiver not found")
		return err
	}

	requestID := fmt.Sprintf("%d-%d-%d-%d", fromUserID, toUserID, amount, time.Now().UnixNano())
	requestKey := fmt.Sprintf("request:%s", requestID)
	if _, err := s.redisClient.Get(ctx, requestKey); err == nil {
		slog.Error("transfer already processed", "request_id", requestID, "from_user_id", fromUserID)
		span.SetStatus(codes.Error, "transfer already processed")
		return pkgerrors.ErrRequestAlreadyProcessed
	}

	if err := s.redisClient.Set(ctx, requestKey, "pending", 24*time.Hour); err != nil {
		slog.Error("failed to set request key", "request_id", requestID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set request key")
		return err
	}

	lockKeys := []string{
		fmt.Sprintf("user:%d:lock", fromUserID),
		fmt.Sprintf("user:%d:lock", toUserID),
	}
	for _, lockKey := range lockKeys {
		ok, err := s.redisClient.SetNX(ctx, lockKey, "locked", 3*time.Second)
		if err != nil {
			s.redisClient.Del(ctx, requestKey)
			for _, lk := range lockKeys {
				s.redisClient.Del(ctx, lk)
			}
			slog.Error("failed to acquire lock", "lock_key", lockKey, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to acquire lock")
			return pkgerrors.ErrBalanceLocked
		}
		if !ok {
			s.redisClient.Del(ctx, requestKey)
			for _, lk := range lockKeys {
				s.redisClient.Del(ctx, lk)
			}
			slog.Error("balance is locked", "lock_key", lockKey)
			span.SetStatus(codes.Error, "balance is locked")
			return pkgerrors.ErrBalanceLocked
		}
	}

	balance, err := s.userRepo.GetBalance(ctx, fromUserID)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		for _, lk := range lockKeys {
			s.redisClient.Del(ctx, lk)
		}
		slog.Error("failed to get balance from Postgres", "user_id", fromUserID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get balance")
		return err
	}

	if balance < amount {
		s.redisClient.Del(ctx, requestKey)
		for _, lk := range lockKeys {
			s.redisClient.Del(ctx, lk)
		}
		slog.Error("insufficient funds", "user_id", fromUserID, "balance", balance, "amount", amount)
		span.SetStatus(codes.Error, "insufficient funds")
		return pkgerrors.ErrInsufficientFunds
	}

	event := map[string]interface{}{
		"from_user_id": fromUserID,
		"to_user_id":   toUserID,
		"amount":       amount,
		"type":         "transfer",
		"status":       "pending",
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"request_id":   requestID,
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		s.redisClient.Del(ctx, requestKey)
		for _, lk := range lockKeys {
			s.redisClient.Del(ctx, lk)
		}
		slog.Error("failed to marshal Kafka event", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal event")
		return fmt.Errorf("failed to marshal Kafka event: %w", err)
	}
	eventID := time.Now().UnixNano()
	if err := s.kafkaProducerTransactions.Send(ctx, "transactions", eventID, eventBytes); err != nil {
		s.redisClient.Del(ctx, requestKey)
		for _, lk := range lockKeys {
			s.redisClient.Del(ctx, lk)
		}
		slog.Error("failed to send Kafka event", "event_id", eventID, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send Kafka event")
		return fmt.Errorf("failed to send Kafka event: %w", err)
	}

	slog.Info("transfer event sent", "from_user_id", fromUserID, "to_user_id", toUserID, "event_id", eventID, "request_id", requestID)
	return nil
}

func (s *merchService) GetBalance(ctx context.Context, userID int32) (int32, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "GetBalance")
	defer span.End()

	slog.Info("Getting balance", "user_id", userID)
	balanceKey := fmt.Sprintf("user:%d:balance", userID)
	balanceStr, err := s.redisClient.Get(ctx, balanceKey)
	if err == nil {
		var balance int32
		if err := json.Unmarshal([]byte(balanceStr), &balance); err != nil {
			slog.Error("failed to unmarshal balance", "user_id", userID, "error", err)
		} else {
			slog.Info("Balance fetched from Redis", "user_id", userID, "balance", balance)
			return balance, nil
		}
	}

	balance, err := s.userRepo.GetBalance(ctx, userID)
	if err != nil {
		slog.Error("failed to get balance from Postgres", "user_id", userID, "error", err)
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	if err := s.redisClient.Set(ctx, balanceKey, balance, 5*time.Minute); err != nil {
		slog.Error("failed to cache balance", "user_id", userID, "error", err)
	}

	slog.Info("Balance fetched from Postgres", "user_id", userID, "balance", balance)
	return balance, nil
}

func (s *merchService) GetTransactionHistory(ctx context.Context, userID int32) ([]models.Transaction, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "GetTransactionHistory")
	defer span.End()

	transactions, err := s.transactionRepo.GetTransactionHistory(ctx, userID)
	if err != nil {
		slog.Error("failed to get transaction history", "user_id", userID, "error", err)
		return nil, err
	}

	slog.Info("transaction history retrieved", "user_id", userID, "count", len(transactions))
	return transactions, nil
}
