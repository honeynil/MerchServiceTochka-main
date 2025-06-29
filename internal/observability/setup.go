package observability

import (
	"context"
	"net/http"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Setup(serviceName string) (func(context.Context) error, http.Handler) {
	observability.InitLogger()
	observability.InitMetrics()
	tracerShutdown := observability.InitTracing(serviceName)
	return tracerShutdown, promhttp.Handler()
}
