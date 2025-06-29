package redis

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
	Close() error
}

type Client struct {
	client *redis.Client
}

func NewClient(addr, password string) *Client {
	return &Client{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       0,
		}),
	}
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisGet")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrKeyNotFound
	}
	if err != nil {
		slog.Error("failed to get key", "method", "Get", "key", key, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	return val, nil
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisSet")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	var val string
	switch v := value.(type) {
	case string:
		val = v
	default:
		valBytes, err := json.Marshal(value)
		if err != nil {
			slog.Error("failed to marshal value", "method", "Set", "key", key, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		val = string(valBytes)
	}

	err := c.client.Set(ctx, key, val, expiration).Err()
	if err != nil {
		slog.Error("failed to set key", "method", "Set", "key", key, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	slog.Info("key set", "method", "Set", "key", key)
	return nil
}

func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisSetNX")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	var val string
	switch v := value.(type) {
	case string:
		val = v
	default:
		valBytes, err := json.Marshal(value)
		if err != nil {
			slog.Error("failed to marshal value", "method", "SetNX", "key", key, "error", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return false, err
		}
		val = string(valBytes)
	}

	ok, err := c.client.SetNX(ctx, key, val, expiration).Result()
	if err != nil {
		slog.Error("failed to set key with SetNX", "method", "SetNX", "key", key, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	slog.Info("key set with SetNX", "method", "SetNX", "key", key, "result", ok)
	return ok, nil
}

func (c *Client) Del(ctx context.Context, key string) error {
	tracer := otel.Tracer("redis-client")
	ctx, span := tracer.Start(ctx, "RedisDel")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	err := c.client.Del(ctx, key).Err()
	if err != nil {
		slog.Error("failed to delete key", "method", "Del", "key", key, "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	slog.Info("key deleted", "method", "Del", "key", key)
	return nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

var ErrKeyNotFound = redis.Nil
