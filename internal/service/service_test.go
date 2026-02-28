package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mmeshcher/gophermart-system/internal/model"
	"github.com/mmeshcher/gophermart-system/internal/repository"
)

func TestHashPasswordDeterministic(t *testing.T) {
	a := hashPassword("user", "pass")
	b := hashPassword("user", "pass")
	c := hashPassword("user", "other")

	if string(a) != string(b) {
		t.Fatalf("hashPassword must be deterministic, got %x and %x", a, b)
	}
	if string(a) == string(c) {
		t.Fatalf("different passwords must produce different hashes")
	}
}

func TestCreateWithdrawalValidation(t *testing.T) {
	svc := &Service{}

	err := svc.CreateWithdrawal(context.Background(), 1, "12345678903", -10)
	if err == nil {
		t.Fatalf("expected error for negative sum")
	}
}

type stubRepo struct {
	createUserID int64
	createUserErr error

	getUser     *model.User
	getUserErr  error

	balanceCurrent   int64
	balanceWithdrawn int64
	balanceErr       error

	withdrawals []model.Withdrawal
	withdrawalsErr error
}

func (s *stubRepo) Close() error { return nil }

func (s *stubRepo) CreateUser(ctx context.Context, login string, passwordHash []byte) (int64, error) {
	return s.createUserID, s.createUserErr
}

func (s *stubRepo) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	return s.getUser, s.getUserErr
}

func (s *stubRepo) AddOrder(ctx context.Context, userID int64, number string) (bool, error) {
	return false, nil
}

func (s *stubRepo) GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	return nil, nil
}

func (s *stubRepo) GetBalance(ctx context.Context, userID int64) (int64, int64, error) {
	return s.balanceCurrent, s.balanceWithdrawn, s.balanceErr
}

func (s *stubRepo) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sumCents int64) error {
	return nil
}

func (s *stubRepo) GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return s.withdrawals, s.withdrawalsErr
}

func (s *stubRepo) GetOrdersForAccrual(ctx context.Context, limit int) ([]repository.OrderForAccrual, error) {
	return nil, nil
}

func (s *stubRepo) UpdateOrderAccrual(ctx context.Context, number string, status model.OrderStatus, accrualCents *int64) error {
	return nil
}

func TestRegisterUser_PropagatesDuplicateError(t *testing.T) {
	repo := &stubRepo{
		createUserErr: repository.ErrUserExists,
	}
	svc := NewService(repo, nil)

	_, err := svc.RegisterUser(context.Background(), "login", "pass")
	if !errors.Is(err, repository.ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
}

func TestAuthenticateUser_InvalidCredentials(t *testing.T) {
	hashed := hashPassword("user", "correct")
	repo := &stubRepo{
		getUser: &model.User{
			ID:           1,
			Login:        "user",
			PasswordHash: hashed,
		},
	}

	svc := NewService(repo, nil)

	_, err := svc.AuthenticateUser(context.Background(), "user", "wrong")
	if err == nil {
		t.Fatalf("expected error for invalid credentials")
	}
}

func TestGetBalance_ConvertsToRubles(t *testing.T) {
	repo := &stubRepo{
		balanceCurrent:   150,
		balanceWithdrawn: 50,
	}
	svc := NewService(repo, nil)

	balance, err := svc.GetBalance(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if balance.Current != 1.5 {
		t.Fatalf("Current = %v, want 1.5", balance.Current)
	}
	if balance.Withdrawn != 0.5 {
		t.Fatalf("Withdrawn = %v, want 0.5", balance.Withdrawn)
	}
}

func TestGetWithdrawalsByUser_PassThrough(t *testing.T) {
	now := time.Now()
	repo := &stubRepo{
		withdrawals: []model.Withdrawal{
			{Order: "123", Sum: 10.5, ProcessedAt: now},
		},
	}
	svc := NewService(repo, nil)

	res, err := svc.GetWithdrawalsByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetWithdrawalsByUser error: %v", err)
	}
	if len(res) != 1 || res[0].Order != "123" {
		t.Fatalf("unexpected withdrawals: %+v", res)
	}
}

func TestStartAccrualUpdates_NoClient(t *testing.T) {
	svc := &Service{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	go func() {
		svc.StartAccrualUpdates(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("StartAccrualUpdates did not return without client")
	}
}
