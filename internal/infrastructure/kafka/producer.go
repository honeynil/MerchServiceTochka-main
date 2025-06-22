package kafka

import (
	"context"
	"log/slog"

	"github.com/segmentio/kafka-go"
)

// KafkaProducer defines the interface for Kafka producer operations.
type KafkaProducer interface {
	Send(ctx context.Context, topic string, key int64, value []byte) error
	Close() error
}

// Producer is the implementation of KafkaProducer.
type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &Producer{writer: writer}
}

func (p *Producer) Send(ctx context.Context, topic string, key int64, value []byte) error {
	msg := kafka.Message{
		Key:   []byte(string(key)),
		Value: value,
	}
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		slog.Error("failed to send Kafka message", "topic", topic, "error", err)
		return err
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
