package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	service "github.com/honeynil/MerchServiceTochka-main/internal/services"
	pkgerrors "github.com/honeynil/MerchServiceTochka-main/pkg/errors"
)

type Handler struct {
	service service.MerchService
}

func NewHandler(s service.MerchService) *Handler {
	return &Handler{service: s}
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
}

func (h *Handler) RegisterPublicRoutes(r *mux.Router) {
	r.HandleFunc("/login", h.Login).Methods("POST")
	r.HandleFunc("/register", h.Register).Methods("POST")
}

func (h *Handler) RegisterProtectedRoutes(r *mux.Router) {
	r.HandleFunc("/buy", h.BuyMerch).Methods("POST")
	r.HandleFunc("/transfer", h.Transfer).Methods("POST")
	r.HandleFunc("/balance", h.GetBalance).Methods("GET")
	r.HandleFunc("/history", h.GetTransactionHistory).Methods("GET")
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	token, err := h.service.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrInvalidCredentials) {
			h.writeError(w, http.StatusUnauthorized, err)
		} else {
			h.writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	eventID, err := h.service.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrUsernameExists) {
			h.writeError(w, http.StatusConflict, err)
		} else {
			h.writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"event_id": eventID})
}

func (h *Handler) BuyMerch(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(int32)
	if !ok {
		h.writeError(w, http.StatusUnauthorized, errors.New("user not authenticated"))
		return
	}

	var req struct {
		MerchID   int32  `json:"merch_id"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.RequestID == "" {
		h.writeError(w, http.StatusBadRequest, errors.New("request_id is required"))
		return
	}

	err := h.service.BuyMerch(r.Context(), userID, req.MerchID, req.RequestID)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRequestAlreadyProcessed) {
			h.writeError(w, http.StatusConflict, err)
		} else if errors.Is(err, pkgerrors.ErrInsufficientFunds) {
			h.writeError(w, http.StatusBadRequest, err)
		} else if errors.Is(err, pkgerrors.ErrMerchNotFound) {
			h.writeError(w, http.StatusNotFound, err)
		} else {
			h.writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(int32)
	if !ok {
		h.writeError(w, http.StatusUnauthorized, errors.New("user not authenticated"))
		return
	}

	var req struct {
		ToUserID int32 `json:"to_user_id"`
		Amount   int32 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	err := h.service.Transfer(r.Context(), userID, req.ToUserID, req.Amount)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrInsufficientFunds) {
			h.writeError(w, http.StatusBadRequest, err)
		} else if errors.Is(err, pkgerrors.ErrUserNotFound) {
			h.writeError(w, http.StatusNotFound, err)
		} else {
			h.writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(int32)
	if !ok {
		h.writeError(w, http.StatusUnauthorized, errors.New("user not authenticated"))
		return
	}

	balance, err := h.service.GetBalance(r.Context(), userID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int32{"balance": balance})
}

func (h *Handler) GetTransactionHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(int32)
	if !ok {
		h.writeError(w, http.StatusUnauthorized, errors.New("user not authenticated"))
		return
	}

	transactions, err := h.service.GetTransactionHistory(r.Context(), userID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transactions)
}
