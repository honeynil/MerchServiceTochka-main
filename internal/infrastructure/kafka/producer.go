package kafka

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/segmentio/kafka-go"
)

type KafkaProducer interface {
	Send(ctx context.Context, topic string, key int64, value []byte) error
	Close() error
}

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.LeastBytes{},
		Async:        true,
		RequiredAcks: kafka.RequireOne,
	}
	return &Producer{writer: writer}
}

func (p *Producer) Send(ctx context.Context, topic string, key int64, value []byte) error {
	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(fmt.Sprintf("%d", key)),
		Value: value,
	}
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		slog.Error("failed to send Kafka message", "topic", topic, "key", key, "error", err)
		return err
	}
	slog.Info("Kafka message sent", "topic", topic, "key", key)
	return nil
}

func (p *Producer) Close() error {
	if err := p.writer.Close(); err != nil {
		slog.Error("failed to close Kafka writer", "error", err)
		return err
	}
	slog.Info("Kafka writer closed")
	return nil
}
