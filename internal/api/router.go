package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/auth"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
	service "github.com/honeynil/MerchServiceTochka-main/internal/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(RequestCounter, RequestDuration)
}

func SetupRouter(svc service.MerchService, redisClient redis.RedisClient, jwtSecret string) *http.ServeMux {
	mux := http.NewServeMux()

	// Middleware для метрик
	metricsMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			endpoint := r.URL.Path
			method := r.Method

			// Записываем ответ для получения статуса
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			status := fmt.Sprintf("%d", recorder.status)
			RequestCounter.WithLabelValues(method, endpoint, status).Inc()
			RequestDuration.WithLabelValues(method, endpoint).Observe(time.Since(start).Seconds())
		}
	}

	// Роуты
	mux.HandleFunc("/register", metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		eventID, err := svc.Register(r.Context(), req.Username, req.Password)
		if err != nil {
			slog.Error("register failed", "username", req.Username, "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `"%s"`, eventID)
	}))

	mux.HandleFunc("/login", metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		token, err := svc.Login(r.Context(), req.Username, req.Password)
		if err != nil {
			slog.Error("login failed", "username", req.Username, "error", err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `"%s"`, token)
	}))

	// Защищённые роуты с JWT
	authHandler := auth.AuthMiddleware(redisClient, jwtSecret)
	mux.Handle("/buy", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID := r.Context().Value("user_id").(int32)
		var req struct {
			MerchID int32 `json:"merch_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := svc.BuyMerch(r.Context(), userID, req.MerchID); err != nil {
			slog.Error("buy failed", "user_id", userID, "merch_id", req.MerchID, "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("{}"))
	})))

	mux.Handle("/transfer", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID := r.Context().Value("user_id").(int32)
		var req struct {
			ToUserID int32 `json:"to_user_id"`
			Amount   int32 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := svc.Transfer(r.Context(), userID, req.ToUserID, req.Amount); err != nil {
			slog.Error("transfer failed", "from_user_id", userID, "to_user_id", req.ToUserID, "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("{}"))
	})))

	mux.Handle("/balance", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID := r.Context().Value("user_id").(int32)
		balance, err := svc.GetBalance(r.Context(), userID)
		if err != nil {
			slog.Error("get balance failed", "user_id", userID, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]int32{"balance": balance})
	})))

	mux.Handle("/history", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID := r.Context().Value("user_id").(int32)
		history, err := svc.GetTransactionHistory(r.Context(), userID)
		if err != nil {
			slog.Error("get history failed", "user_id", userID, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(history)
	})))

	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// statusRecorder для захвата статуса ответа
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}
