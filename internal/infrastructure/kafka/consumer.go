package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
	"github.com/honeynil/MerchServiceTochka-main/internal/models"
	"github.com/honeynil/MerchServiceTochka-main/internal/repository"
	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader          *kafka.Reader
	userRepo        repository.UserRepository
	transactionRepo repository.TransactionRepository
	redisClient     redis.RedisClient
}

func NewConsumer(brokers []string, topic, groupID string, userRepo repository.UserRepository, transactionRepo repository.TransactionRepository, redisClient redis.RedisClient) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			Topic:    topic,
			GroupID:  groupID,
			MinBytes: 10e3,
			MaxBytes: 10e6,
		}),
		userRepo:        userRepo,
		transactionRepo: transactionRepo,
		redisClient:     redisClient,
	}
}

func (c *Consumer) Consume(ctx context.Context) {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			slog.Error("failed to read Kafka message", "topic", c.reader.Config().Topic, "error", err)
			continue
		}

		slog.Info("Kafka message received", "topic", msg.Topic, "key", string(msg.Key), "value", string(msg.Value))

		switch msg.Topic {
		case "users":
			var event struct {
				Username     string `json:"username"`
				PasswordHash string `json:"password_hash"`
				Balance      int32  `json:"balance"`
				CreatedAt    string `json:"created_at"`
			}
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				slog.Error("failed to unmarshal user event", "error", err)
				continue
			}

			createdAt, err := time.Parse(time.RFC3339, event.CreatedAt)
			if err != nil {
				slog.Error("invalid created_at format", "value", event.CreatedAt, "error", err)
				continue
			}

			user := &models.User{
				Username:     event.Username,
				PasswordHash: event.PasswordHash,
				Balance:      event.Balance,
				CreatedAt:    createdAt,
			}

			if err := c.userRepo.Create(ctx, user); err != nil {
				slog.Error("failed to create user", "username", user.Username, "error", err)
				continue
			}

			slog.Info("user created", "username", user.Username, "user_id", user.ID)

		case "transactions":
			var event struct {
				UserID     int32  `json:"user_id,omitempty"`
				FromUserID int32  `json:"from_user_id,omitempty"`
				ToUserID   int32  `json:"to_user_id,omitempty"`
				MerchID    int32  `json:"merch_id,omitempty"`
				Amount     int32  `json:"amount"`
				Type       string `json:"type"`
				CreatedAt  string `json:"created_at"`
				RequestID  string `json:"request_id,omitempty"`
			}
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				slog.Error("failed to unmarshal transaction event", "error", err)
				continue
			}

			createdAt, err := time.Parse(time.RFC3339, event.CreatedAt)
			if err != nil {
				slog.Error("invalid created_at format", "value", event.CreatedAt, "error", err)
				continue
			}

			switch event.Type {
			case string(models.TypePurchase):
				if event.UserID == 0 || event.MerchID == 0 || event.RequestID == "" {
					slog.Error("invalid purchase event: missing user_id, merch_id, or request_id")
					continue
				}

				// Проверяем идемпотентность
				requestKey := fmt.Sprintf("request:%s", event.RequestID)
				if _, err := c.redisClient.Get(ctx, requestKey); err == nil {
					slog.Info("purchase already processed", "request_id", event.RequestID, "user_id", event.UserID)
					continue
				}

				// Обновляем баланс
				_, err := c.userRepo.ChangeBalance(ctx, event.UserID, event.Amount)
				if err != nil {
					slog.Error("failed to update balance", "user_id", event.UserID, "error", err)
					continue
				}

				// Создаем транзакцию
				transaction := &models.Transaction{
					UserID:    event.UserID,
					RelatedID: event.MerchID,
					Amount:    event.Amount,
					Type:      models.TypePurchase,
					Status:    models.StatusCompleted,
					CreatedAt: createdAt,
				}

				transactionID, err := c.transactionRepo.Create(ctx, transaction)
				if err != nil {
					slog.Error("failed to create transaction", "user_id", event.UserID, "error", err)
					continue
				}

				// Сохраняем request_id
				if err := c.redisClient.Set(ctx, requestKey, "processed", 24*time.Hour); err != nil {
					slog.Error("failed to set request_id in Redis", "request_id", event.RequestID, "error", err)
				}

				// Снимаем блокировку
				lockKey := fmt.Sprintf("user:%d:lock", event.UserID)
				if err := c.redisClient.Del(ctx, lockKey); err != nil {
					slog.Error("failed to remove lock", "user_id", event.UserID, "error", err)
				}

				slog.Info("purchase processed", "user_id", event.UserID, "merch_id", event.MerchID, "transaction_id", transactionID, "request_id", event.RequestID)

			case string(models.TypeTransfer):
				if event.FromUserID == 0 || event.ToUserID == 0 {
					slog.Error("invalid transfer event: missing from_user_id or to_user_id")
					continue
				}

				// Вычитаем у отправителя
				_, err := c.userRepo.ChangeBalance(ctx, event.FromUserID, -event.Amount)
				if err != nil {
					slog.Error("failed to update sender balance", "user_id", event.FromUserID, "error", err)
					continue
				}

				// Добавляем получателю
				_, err = c.userRepo.ChangeBalance(ctx, event.ToUserID, event.Amount)
				if err != nil {
					slog.Error("failed to update receiver balance", "user_id", event.ToUserID, "error", err)
					continue
				}

				// Транзакция для отправителя
				senderTransaction := &models.Transaction{
					UserID:    event.FromUserID,
					RelatedID: event.ToUserID,
					Amount:    -event.Amount,
					Type:      models.TypeTransfer,
					Status:    models.StatusCompleted,
					CreatedAt: createdAt,
				}
				senderTransactionID, err := c.transactionRepo.Create(ctx, senderTransaction)
				if err != nil {
					slog.Error("failed to create sender transaction", "user_id", event.FromUserID, "error", err)
					continue
				}

				// Транзакция для получателя
				receiverTransaction := &models.Transaction{
					UserID:    event.ToUserID,
					RelatedID: event.FromUserID,
					Amount:    event.Amount,
					Type:      models.TypeTransfer,
					Status:    models.StatusCompleted,
					CreatedAt: createdAt,
				}
				receiverTransactionID, err := c.transactionRepo.Create(ctx, receiverTransaction)
				if err != nil {
					slog.Error("failed to create receiver transaction", "user_id", event.ToUserID, "error", err)
					continue
				}

				// Снимаем блокировку для обоих пользователей
				for _, userID := range []int32{event.FromUserID, event.ToUserID} {
					lockKey := fmt.Sprintf("user:%d:lock", userID)
					if err := c.redisClient.Del(ctx, lockKey); err != nil {
						slog.Error("failed to remove lock", "user_id", userID, "error", err)
					}
				}

				slog.Info("transfer processed", "from_user_id", event.FromUserID, "to_user_id", event.ToUserID, "sender_transaction_id", senderTransactionID, "receiver_transaction_id", receiverTransactionID)

			default:
				slog.Error("unknown transaction type", "type", event.Type)
				continue
			}
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
