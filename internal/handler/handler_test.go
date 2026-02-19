package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mmeshcher/gophermart-system/internal/middleware"
	"github.com/mmeshcher/gophermart-system/internal/model"
)

type stubService struct {
	registerUserID int64
	registerErr    error

	authUserID int64
	authErr    error

	addOrderAlready bool
	addOrderErr     error

	ordersResp []model.Order
	ordersErr  error

	balanceResp *model.Balance
	balanceErr  error

	withdrawErr error

	withdrawalsResp []model.Withdrawal
	withdrawalsErr  error
}

func (s *stubService) RegisterUser(ctx context.Context, login, password string) (int64, error) {
	return s.registerUserID, s.registerErr
}

func (s *stubService) AuthenticateUser(ctx context.Context, login, password string) (int64, error) {
	return s.authUserID, s.authErr
}

func (s *stubService) AddOrder(ctx context.Context, userID int64, number string) (bool, error) {
	return s.addOrderAlready, s.addOrderErr
}

func (s *stubService) GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.ordersResp, s.ordersErr
}

func (s *stubService) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	return s.balanceResp, s.balanceErr
}

func (s *stubService) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	return s.withdrawErr
}

func (s *stubService) GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return s.withdrawalsResp, s.withdrawalsErr
}

func newTestHandler(t *testing.T, svc Service) *Handler {
	t.Helper()

	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	auth := middleware.NewAuthMiddleware("test-secret")

	return NewHandler(svc, logger, auth)
}

func TestRegister_Success(t *testing.T) {
	svc := &stubService{
		registerUserID: 42,
	}
	h := newTestHandler(t, svc)

	body, _ := json.Marshal(credentialsRequest{
		Login:    "user",
		Password: "pass",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
}

func TestLogin_UnauthorizedOnError(t *testing.T) {
	svc := &stubService{
		authErr: context.DeadlineExceeded,
	}
	h := newTestHandler(t, svc)

	body, _ := json.Marshal(credentialsRequest{
		Login:    "user",
		Password: "pass",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
	}
}

func TestGetOrders_NoContent(t *testing.T) {
	svc := &stubService{
		ordersResp: []model.Order{},
	}
	h := newTestHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	rec := httptest.NewRecorder()

	h.authMiddleware.SetAuthCookie(rec, 1)
	cookie := rec.Result().Cookies()[0]

	req.AddCookie(cookie)
	respRec := httptest.NewRecorder()

	handlerWithAuth := h.authMiddleware.Middleware(http.HandlerFunc(h.GetOrders))
	handlerWithAuth.ServeHTTP(respRec, req)

	res := respRec.Result()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNoContent)
	}
}

func TestGetWithdrawals_JSONResponse(t *testing.T) {
	now := time.Now().UTC()
	svc := &stubService{
		withdrawalsResp: []model.Withdrawal{
			{
				Order:       "123",
				Sum:         10.5,
				ProcessedAt: now,
			},
		},
	}
	h := newTestHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil)
	rec := httptest.NewRecorder()

	h.authMiddleware.SetAuthCookie(rec, 1)
	cookie := rec.Result().Cookies()[0]
	req.AddCookie(cookie)

	respRec := httptest.NewRecorder()
	handlerWithAuth := h.authMiddleware.Middleware(http.HandlerFunc(h.GetWithdrawals))
	handlerWithAuth.ServeHTTP(respRec, req)

	res := respRec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}
