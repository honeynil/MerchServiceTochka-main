package redis

import (
	"context"
	"log/slog"
	"time"

	stderrors "errors"

	"github.com/redis/go-redis/v9"
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
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrKeyNotFound
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Del(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *Client) Close() error {
	return c.client.Close()
}
