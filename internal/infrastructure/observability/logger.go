package observability

import (
	"context"
	"log/slog"
	"os"
)

func InitLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}

func WithContext(ctx context.Context, attrs ...any) *slog.Logger {
	return slog.With(attrs)
}
