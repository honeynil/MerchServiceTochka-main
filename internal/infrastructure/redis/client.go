package redis

import (
	"context"
	"log/slog"
	"time"

	stderrors "errors"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/observability"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var ErrKeyNotFound = stderrors.New("key not found")

// RedisClient defines the interface for Redis operations.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

// Client is the implementation of RedisClient.
type Client struct {
	client *redis.Client
}

func NewClient(addr string) *Client {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		slog.Error("failed to connect to Redis", "addr", addr, "error", err)
		panic(err)
	}

	slog.Info("connected to Redis", "addr", addr)
	return &Client{client: client}
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	var err error
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisGet")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("RedisGet", status).Inc()
		observability.RepositoryDuration.WithLabelValues("RedisGet").Observe(time.Since(start).Seconds())
	}()

	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		err = ErrKeyNotFound
		slog.Error("key not found", "method", "Get", "key", key, "error", err)
		return "", err
	}
	if err != nil {
		slog.Error("failed to get key", "method", "Get", "key", key, "error", err)
		return "", err
	}
	slog.Info("key retrieved", "method", "Get", "key", key)
	return val, nil
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	var err error
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisSet")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("RedisSet", status).Inc()
		observability.RepositoryDuration.WithLabelValues("RedisSet").Observe(time.Since(start).Seconds())
	}()

	err = c.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		slog.Error("failed to set key", "method", "Set", "key", key, "error", err)
		return err
	}
	slog.Info("key set", "method", "Set", "key", key, "expiration", expiration)
	return nil
}

func (c *Client) Del(ctx context.Context, key string) error {
	var err error
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisDel")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		observability.RepositoryCalls.WithLabelValues("RedisDel", status).Inc()
		observability.RepositoryDuration.WithLabelValues("RedisDel").Observe(time.Since(start).Seconds())
	}()

	err = c.client.Del(ctx, key).Err()
	if err != nil {
		slog.Error("failed to delete key", "method", "Del", "key", key, "error", err)
		return err
	}
	slog.Info("key deleted", "method", "Del", "key", key)
	return nil
}

func (c *Client) Close() error {
	err := c.client.Close()
	if err != nil {
		slog.Error("failed to close Redis connection", "error", err)
		return err
	}
	slog.Info("Redis connection closed")
	return nil
}
