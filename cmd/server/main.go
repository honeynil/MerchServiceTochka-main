package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/honeynil/MerchServiceTochka-main/internal/api"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/kafka"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
	"github.com/honeynil/MerchServiceTochka-main/internal/observability"
	core "github.com/honeynil/MerchServiceTochka-main/internal/repository/postgres"
	service "github.com/honeynil/MerchServiceTochka-main/internal/services"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Загружаем .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default env vars")
	}

	// Инициализируем логи, метрики, трейсы
	shutdown, _ := observability.Setup("merch-service")
	defer shutdown(context.Background())

	// Подключаемся к Postgres
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_DSN"))
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()

	// Инициализируем зависимости
	userRepo := core.NewPostgresUserRepository(db)
	merchRepo := core.NewPostgresMerchRepository(db)
	transactionRepo := core.NewPostgresTransactionRepository(db)
	redisClient := redis.NewClient(os.Getenv("REDIS_ADDR"))
	kafkaProducerTransactions := kafka.NewProducer([]string{os.Getenv("KAFKA_BROKER")}, "transactions")
	kafkaProducerUsers := kafka.NewProducer([]string{os.Getenv("KAFKA_BROKER")}, "users")
	defer kafkaProducerTransactions.Close()
	defer kafkaProducerUsers.Close()
	jwtSecret := os.Getenv("JWT_SECRET")

	// Инициализируем сервис
	svc := service.NewMerchService(userRepo, merchRepo, transactionRepo, redisClient, kafkaProducerUsers, kafkaProducerTransactions, jwtSecret)

	// Настраиваем Kafka-консьюмеры
	transactionConsumer := kafka.NewConsumer([]string{os.Getenv("KAFKA_BROKER")}, "transactions", "merch-service-group", userRepo, transactionRepo, redisClient)
	userConsumer := kafka.NewConsumer([]string{os.Getenv("KAFKA_BROKER")}, "users", "merch-service-group-users", userRepo, transactionRepo, redisClient)
	go transactionConsumer.Consume(context.Background())
	go userConsumer.Consume(context.Background())
	defer transactionConsumer.Close()
	defer userConsumer.Close()

	// Настраиваем роутер
	mux := api.SetupRouter(svc, redisClient, jwtSecret)

	// Запускаем сервер
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	go func() {
		log.Printf("Starting server on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server stopped")
}
