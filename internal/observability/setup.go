package observability

import (
	"context"
	"net/http"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Setup инициализирует логи, метрики и трейсы, возвращает shutdown-функцию и HTTP-обработчик для /metrics
func Setup(serviceName string) (func(context.Context) error, http.Handler) {
	// Инициализация логов
	observability.InitLogger()

	// Инициализация метрик
	observability.InitMetrics()

	// Инициализация трейсов
	tracerShutdown := observability.InitTracing(serviceName)

	// Возвращаем shutdown-функцию для трейсинга и Prometheus-обработчик
	return tracerShutdown, promhttp.Handler()
}
