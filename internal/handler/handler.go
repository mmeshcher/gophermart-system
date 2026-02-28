// Package handler содержит HTTP-обработчики API сервиса гофермарт.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mmeshcher/gophermart-system/internal/middleware"
	"github.com/mmeshcher/gophermart-system/internal/model"
	"github.com/mmeshcher/gophermart-system/internal/repository"
	"github.com/mmeshcher/gophermart-system/internal/validation"
)

// Service определяет контракт бизнес-логики, используемой HTTP-обработчиками.
type Service interface {
	RegisterUser(ctx context.Context, login, password string) (int64, error)
	AuthenticateUser(ctx context.Context, login, password string) (int64, error)
	AddOrder(ctx context.Context, userID int64, number string) (bool, error)
	GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error)
	GetBalance(ctx context.Context, userID int64) (*model.Balance, error)
	CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error
	GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

// Handler реализует HTTP-обработчики API сервиса гофермарт.
type Handler struct {
	service        Service
	logger         *zap.Logger
	authMiddleware *middleware.AuthMiddleware
}

// NewHandler создаёт новый экземпляр обработчика HTTP-запросов.
func NewHandler(s Service, logger *zap.Logger, auth *middleware.AuthMiddleware) *Handler {
	return &Handler{
		service:        s,
		logger:         logger,
		authMiddleware: auth,
	}
}

type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Register обрабатывает регистрацию нового пользователя.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	userID, err := h.service.RegisterUser(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, repository.ErrUserExists) {
			http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
			return
		}
		h.logger.Error("register user error", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h.authMiddleware.SetAuthCookie(w, userID)
	w.WriteHeader(http.StatusOK)
}

// Login выполняет аутентификацию пользователя и установка cookie.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	userID, err := h.service.AuthenticateUser(r.Context(), req.Login, req.Password)
	if err != nil {
		if err == repository.ErrUserNotFound || err.Error() == "invalid credentials" {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		h.logger.Error("login user error", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h.authMiddleware.SetAuthCookie(w, userID)
	w.WriteHeader(http.StatusOK)
}

// UploadOrder принимает номер заказа для начислений от текущего пользователя.
func (h *Handler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	number := strings.TrimSpace(string(body))

	if !validation.IsValidOrderNumber(number) {
		http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
		return
	}

	alreadyExists, err := h.service.AddOrder(r.Context(), userID, number)
	if err != nil {
		if err == repository.ErrOrderOwnedByAnother {
			http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
			return
		}
		h.logger.Error("upload order error", zap.Error(err), zap.String("order", number))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if alreadyExists {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

type orderResponse struct {
	Number     string   `json:"number"`
	Status     string   `json:"status"`
	Accrual    *float64 `json:"accrual,omitempty"`
	UploadedAt string   `json:"uploaded_at"`
}

// GetOrders возвращает список заказов текущего пользователя.
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	orders, err := h.service.GetOrdersByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("get orders error", zap.Error(err), zap.Int64("userID", userID))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, orderResponse{
			Number:     o.Number,
			Status:     string(o.Status),
			Accrual:    o.Accrual,
			UploadedAt: o.UploadedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// GetBalance возвращает баланс текущего пользователя.
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	balance, err := h.service.GetBalance(r.Context(), userID)
	if err != nil {
		h.logger.Error("get balance error", zap.Error(err), zap.Int64("userID", userID))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(balance); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

type withdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

// Withdraw создаёт операцию списания средств для текущего пользователя.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var req withdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !validation.IsValidOrderNumber(req.Order) {
		http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
		return
	}

	if req.Sum <= 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	err := h.service.CreateWithdrawal(r.Context(), userID, req.Order, req.Sum)
	if err != nil {
		if err == repository.ErrInsufficientBalance {
			http.Error(w, http.StatusText(http.StatusPaymentRequired), http.StatusPaymentRequired)
			return
		}
		h.logger.Error("withdraw error", zap.Error(err), zap.Int64("userID", userID), zap.String("order", req.Order))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type withdrawalResponse struct {
	Order       string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

// GetWithdrawals возвращает историю списаний текущего пользователя.
func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	withdrawals, err := h.service.GetWithdrawalsByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("get withdrawals error", zap.Error(err), zap.Int64("userID", userID))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]withdrawalResponse, 0, len(withdrawals))
	for _, wth := range withdrawals {
		resp = append(resp, withdrawalResponse{
			Order:       wth.Order,
			Sum:         wth.Sum,
			ProcessedAt: wth.ProcessedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
