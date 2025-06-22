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
	"golang.org/x/crypto/bcrypt"
)

type MerchService interface {
	Login(ctx context.Context, username, password string) (string, error)
	Register(ctx context.Context, username, password string) (string, error)
	BuyMerch(ctx context.Context, userID, merchID int32) error
	Transfer(ctx context.Context, fromUserID, toUserID, amount int32) error
	GetBalance(ctx context.Context, userID int32) (int32, error)
	GetTransactionHistory(ctx context.Context, userID int32) ([]models.Transaction, error)
}

type merchService struct {
	userRepo        repository.UserRepository
	merchRepo       repository.MerchRepository
	transactionRepo repository.TransactionRepository
	redisClient     redis.RedisClient
	kafkaProducer   kafka.KafkaProducer
	jwtSecret       string
}

func NewMerchService(
	userRepo repository.UserRepository,
	merchRepo repository.MerchRepository,
	transactionRepo repository.TransactionRepository,
	redisClient redis.RedisClient,
	kafkaProducer kafka.KafkaProducer,
	jwtSecret string,
) MerchService {
	return &merchService{
		userRepo:        userRepo,
		merchRepo:       merchRepo,
		transactionRepo: transactionRepo,
		redisClient:     redisClient,
		kafkaProducer:   kafkaProducer,
		jwtSecret:       jwtSecret,
	}
}

// Остальной код остаётся без изменений (Login, Register, BuyMerch, Transfer, GetBalance, GetTransactionHistory)

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

func (s *merchService) Register(ctx context.Context, username, password string) (string, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "Register")
	defer span.End()

	if username == "" || password == "" {
		return "", fmt.Errorf("username and password are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.userRepo.GetByUsername(ctx, username)
	if err == nil {
		slog.Error("username already exists", "username", username)
		return "", pkgerrors.ErrUsernameExists
	}

	event := map[string]interface{}{
		"username":      username,
		"password_hash": string(hash),
		"balance":       0,
		"created_at":    time.Now().UTC().Format(time.RFC3339),
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal Kafka event", "error", err)
		return "", fmt.Errorf("failed to marshal Kafka event: %w", err)
	}
	eventID := time.Now().UnixNano()
	if err := s.kafkaProducer.Send(ctx, "users", eventID, eventBytes); err != nil {
		slog.Error("failed to send Kafka event", "event_id", eventID, "error", err)
		return "", fmt.Errorf("failed to send Kafka event: %w", err)
	}

	slog.Info("register event sent", "username", username, "event_id", eventID)
	return fmt.Sprintf("%d", eventID), nil
}

func (s *merchService) BuyMerch(ctx context.Context, userID, merchID int32) error {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "BuyMerch")
	defer span.End()

	merchKey := fmt.Sprintf("merch:%d", merchID)
	var merch *models.Merch
	merchJSON, err := s.redisClient.Get(ctx, merchKey)
	if err != nil && !stderrors.Is(err, redis.ErrKeyNotFound) {
		slog.Error("failed to get merch from Redis", "merch_id", merchID, "error", err)
		return err
	}
	if stderrors.Is(err, redis.ErrKeyNotFound) {
		merch, err = s.merchRepo.GetByID(ctx, merchID)
		if err != nil {
			slog.Error("merch not found", "merch_id", merchID, "error", err)
			return err
		}
		merchBytes, err := json.Marshal(merch)
		if err != nil {
			slog.Error("failed to marshal merch", "merch_id", merchID, "error", err)
			return err
		}
		if err := s.redisClient.Set(ctx, merchKey, string(merchBytes), time.Hour); err != nil {
			slog.Error("failed to cache merch", "merch_id", merchID, "error", err)
		}
	} else {
		if err := json.Unmarshal([]byte(merchJSON), &merch); err != nil {
			slog.Error("failed to unmarshal merch", "merch_id", merchID, "error", err)
			return err
		}
	}

	price, err := s.merchRepo.GetPrice(ctx, merchID)
	if err != nil {
		slog.Error("failed to get merch price", "merch_id", merchID, "error", err)
		return err
	}

	balanceKey := fmt.Sprintf("user:%d:balance", userID)
	balanceStr, err := s.redisClient.Get(ctx, balanceKey)
	var balance int32
	if err != nil && !stderrors.Is(err, redis.ErrKeyNotFound) {
		slog.Error("failed to get balance from Redis", "user_id", userID, "error", err)
		return err
	}
	if stderrors.Is(err, redis.ErrKeyNotFound) {
		balance, err = s.transactionRepo.GetBalance(ctx, userID)
		if err != nil {
			slog.Error("failed to get balance", "user_id", userID, "error", err)
			return err
		}
		if err := s.redisClient.Set(ctx, balanceKey, balance, time.Minute); err != nil {
			slog.Error("failed to cache balance", "user_id", userID, "error", err)
		}
	} else {
		if err := json.Unmarshal([]byte(balanceStr), &balance); err != nil {
			slog.Error("failed to unmarshal balance", "user_id", userID, "error", err)
			return err
		}
	}

	if balance < price {
		slog.Error("insufficient funds", "user_id", userID, "balance", balance, "price", price)
		return pkgerrors.ErrInsufficientFunds
	}

	event := map[string]interface{}{
		"user_id":    userID,
		"merch_id":   merchID,
		"amount":     -price,
		"type":       "purchase",
		"status":     "pending",
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal Kafka event", "error", err)
		return fmt.Errorf("failed to marshal Kafka event: %w", err)
	}
	eventID := time.Now().UnixNano()
	if err := s.kafkaProducer.Send(ctx, "transactions", eventID, eventBytes); err != nil {
		slog.Error("failed to send Kafka event", "event_id", eventID, "error", err)
		return fmt.Errorf("failed to send Kafka event: %w", err)
	}

	if err := s.redisClient.Del(ctx, balanceKey); err != nil {
		slog.Error("failed to invalidate balance cache", "user_id", userID, "error", err)
	}

	slog.Info("buy event sent", "user_id", userID, "merch_id", merchID, "event_id", eventID)
	return nil
}

func (s *merchService) Transfer(ctx context.Context, fromUserID, toUserID, amount int32) error {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "Transfer")
	defer span.End()

	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	_, err := s.userRepo.GetByID(ctx, fromUserID)
	if err != nil {
		slog.Error("sender not found", "user_id", fromUserID, "error", err)
		return err
	}
	_, err = s.userRepo.GetByID(ctx, toUserID)
	if err != nil {
		slog.Error("receiver not found", "user_id", toUserID, "error", err)
		return err
	}

	balanceKey := fmt.Sprintf("user:%d:balance", fromUserID)
	balanceStr, err := s.redisClient.Get(ctx, balanceKey)
	var balance int32
	if err != nil && !stderrors.Is(err, redis.ErrKeyNotFound) {
		slog.Error("failed to get balance from Redis", "user_id", fromUserID, "error", err)
		return err
	}
	if stderrors.Is(err, redis.ErrKeyNotFound) {
		balance, err = s.transactionRepo.GetBalance(ctx, fromUserID)
		if err != nil {
			slog.Error("failed to get balance", "user_id", fromUserID, "error", err)
			return err
		}
		if err := s.redisClient.Set(ctx, balanceKey, balance, time.Minute); err != nil {
			slog.Error("failed to cache balance", "user_id", fromUserID, "error", err)
		}
	} else {
		if err := json.Unmarshal([]byte(balanceStr), &balance); err != nil {
			slog.Error("failed to unmarshal balance", "user_id", fromUserID, "error", err)
			return err
		}
	}

	if balance < amount {
		slog.Error("insufficient funds", "user_id", fromUserID, "balance", balance, "amount", amount)
		return pkgerrors.ErrInsufficientFunds
	}

	event := map[string]interface{}{
		"from_user_id": fromUserID,
		"to_user_id":   toUserID,
		"amount":       amount,
		"type":         "transfer",
		"status":       "pending",
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal Kafka event", "error", err)
		return fmt.Errorf("failed to marshal Kafka event: %w", err)
	}
	eventID := time.Now().UnixNano()
	if err := s.kafkaProducer.Send(ctx, "transactions", eventID, eventBytes); err != nil {
		slog.Error("failed to send Kafka event", "event_id", eventID, "error", err)
		return fmt.Errorf("failed to send Kafka event: %w", err)
	}

	for _, userID := range []int32{fromUserID, toUserID} {
		if err := s.redisClient.Del(ctx, fmt.Sprintf("user:%d:balance", userID)); err != nil {
			slog.Error("failed to invalidate balance cache", "user_id", userID, "error", err)
		}
	}

	slog.Info("transfer event sent", "from_user_id", fromUserID, "to_user_id", toUserID, "event_id", eventID)
	return nil
}

func (s *merchService) GetBalance(ctx context.Context, userID int32) (int32, error) {
	tracer := otel.Tracer("merch-service")
	ctx, span := tracer.Start(ctx, "GetBalance")
	defer span.End()

	balanceKey := fmt.Sprintf("user:%d:balance", userID)
	balanceStr, err := s.redisClient.Get(ctx, balanceKey)
	if err != nil && !stderrors.Is(err, redis.ErrKeyNotFound) {
		slog.Error("failed to get balance from Redis", "user_id", userID, "error", err)
		return 0, err
	}
	if stderrors.Is(err, redis.ErrKeyNotFound) {
		balance, err := s.transactionRepo.GetBalance(ctx, userID)
		if err != nil {
			slog.Error("failed to get balance", "user_id", userID, "error", err)
			return 0, err
		}
		if err := s.redisClient.Set(ctx, balanceKey, balance, time.Minute); err != nil {
			slog.Error("failed to cache balance", "user_id", userID, "error", err)
		}
		return balance, nil
	}

	var balance int32
	if err := json.Unmarshal([]byte(balanceStr), &balance); err != nil {
		slog.Error("failed to unmarshal balance", "user_id", userID, "error", err)
		return 0, err
	}
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
