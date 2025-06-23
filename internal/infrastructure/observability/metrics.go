package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Счётчик вызовов методов репозитория
	RepositoryCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "repository_calls_total",
			Help: "Total number of repository method calls",
		},
		[]string{"method", "status"},
	)

	// Гистограмма времени выполнения запросов
	RepositoryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "repository_duration_seconds",
			Help:    "Duration of repository method calls in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

func InitMetrics() {
	prometheus.MustRegister(RepositoryCalls, RepositoryDuration)
}
