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
	"github.com/honeynil/MerchServiceTochka-main/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Response — структура для единообразных ответов
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

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

			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			status := fmt.Sprintf("%d", recorder.status)
			RequestCounter.WithLabelValues(method, endpoint, status).Inc()
			RequestDuration.WithLabelValues(method, endpoint).Observe(time.Since(start).Seconds())
		}
	}

	// Хелпер для отправки JSON-ответа
	sendResponse := func(w http.ResponseWriter, statusCode int, resp Response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}

	// Хелпер для обработки ошибок
	handleError := func(w http.ResponseWriter, err error, defaultMsg string, defaultStatus int) {
		var statusCode int
		var message string

		switch err {
		case errors.ErrUsernameExists:
			statusCode = http.StatusConflict
			message = "Username already exists"
		case errors.ErrInvalidCredentials:
			statusCode = http.StatusUnauthorized
			message = "Invalid username or password"
		case errors.ErrMerchNotFound:
			statusCode = http.StatusNotFound
			message = "Merch not found"
		case errors.ErrInsufficientFunds:
			statusCode = http.StatusBadRequest
			message = "Insufficient funds"
		case errors.ErrUserNotFound:
			statusCode = http.StatusNotFound
			message = "User not found"
		case errors.ErrRequestAlreadyProcessed:
			statusCode = http.StatusConflict
			message = "Request already processed"
		case errors.ErrBalanceLocked:
			statusCode = http.StatusTooManyRequests
			message = "User balance is being processed"
		default:
			statusCode = defaultStatus
			message = defaultMsg
		}

		sendResponse(w, statusCode, Response{
			Status:  "error",
			Message: message,
		})
	}

	// Роуты
	mux.HandleFunc("/register", metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid request body",
			})
			return
		}

		if req.Username == "" || req.Password == "" {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Username and password are required",
			})
			return
		}

		eventID, err := svc.Register(r.Context(), req.Username, req.Password)
		if err != nil {
			slog.Error("register failed", "username", req.Username, "error", err)
			handleError(w, err, "Failed to register user", http.StatusBadRequest)
			return
		}

		slog.Info("user registered", "username", req.Username, "event_id", eventID)
		sendResponse(w, http.StatusCreated, Response{
			Status:  "success",
			Message: "User created successfully",
			Data:    map[string]string{"event_id": eventID},
		})
	}))

	mux.HandleFunc("/login", metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid request body",
			})
			return
		}

		token, err := svc.Login(r.Context(), req.Username, req.Password)
		if err != nil {
			slog.Error("login failed", "username", req.Username, "error", err)
			handleError(w, err, "Failed to login", http.StatusUnauthorized)
			return
		}

		slog.Info("user logged in", "username", req.Username)
		sendResponse(w, http.StatusOK, Response{
			Status:  "success",
			Message: "Login successful",
			Data:    map[string]string{"token": token},
		})
	}))

	// Защищённые роуты с JWT
	authHandler := auth.AuthMiddleware(redisClient, jwtSecret)
	mux.Handle("/buy", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		userID := r.Context().Value("user_id").(int32)
		var req struct {
			MerchID   int32  `json:"merch_id"`
			RequestID string `json:"request_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid request body",
			})
			return
		}

		if req.MerchID <= 0 {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid merch ID",
			})
			return
		}

		if req.RequestID == "" {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Request ID is required",
			})
			return
		}

		if err := svc.BuyMerch(r.Context(), userID, req.MerchID, req.RequestID); err != nil {
			slog.Error("buy failed", "user_id", userID, "merch_id", req.MerchID, "request_id", req.RequestID, "error", err)
			handleError(w, err, "Failed to purchase merch", http.StatusBadRequest)
			return
		}

		slog.Info("merch purchased", "user_id", userID, "merch_id", req.MerchID, "request_id", req.RequestID)
		sendResponse(w, http.StatusAccepted, Response{
			Status:  "success",
			Message: "Merch purchase accepted",
		})
	})))

	mux.Handle("/transfer", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		userID := r.Context().Value("user_id").(int32)
		var req struct {
			ToUserID int32 `json:"to_user_id"`
			Amount   int32 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid request body",
			})
			return
		}

		if req.ToUserID <= 0 {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Invalid receiver ID",
			})
			return
		}
		if req.Amount <= 0 {
			sendResponse(w, http.StatusBadRequest, Response{
				Status:  "error",
				Message: "Amount must be positive",
			})
			return
		}

		if err := svc.Transfer(r.Context(), userID, req.ToUserID, req.Amount); err != nil {
			slog.Error("transfer failed", "from_user_id", userID, "to_user_id", req.ToUserID, "error", err)
			handleError(w, err, "Failed to transfer funds", http.StatusBadRequest)
			return
		}

		slog.Info("transfer completed", "from_user_id", userID, "to_user_id", req.ToUserID, "amount", req.Amount)
		sendResponse(w, http.StatusAccepted, Response{
			Status:  "success",
			Message: "Transfer completed successfully",
		})
	})))

	mux.Handle("/balance", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		userID := r.Context().Value("user_id").(int32)
		balance, err := svc.GetBalance(r.Context(), userID)
		if err != nil {
			slog.Error("get balance failed", "user_id", userID, "error", err)
			handleError(w, err, "Failed to get balance", http.StatusInternalServerError)
			return
		}

		slog.Info("balance retrieved", "user_id", userID, "balance", balance)
		sendResponse(w, http.StatusOK, Response{
			Status: "success",
			Data:   map[string]int32{"balance": balance},
		})
	})))

	mux.Handle("/history", authHandler(metricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			sendResponse(w, http.StatusMethodNotAllowed, Response{
				Status:  "error",
				Message: "Method not allowed",
			})
			return
		}

		userID := r.Context().Value("user_id").(int32)
		history, err := svc.GetTransactionHistory(r.Context(), userID)
		if err != nil {
			slog.Error("get history failed", "user_id", userID, "error", err)
			handleError(w, err, "Failed to get transaction history", http.StatusInternalServerError)
			return
		}

		slog.Info("transaction history retrieved", "user_id", userID, "count", len(history))
		sendResponse(w, http.StatusOK, Response{
			Status: "success",
			Data:   history,
		})
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
