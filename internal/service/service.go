// Package service реализует бизнес-логику сервиса гофермарт.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/mmeshcher/gophermart-system/internal/accrual"
	"github.com/mmeshcher/gophermart-system/internal/model"
	"github.com/mmeshcher/gophermart-system/internal/repository"
)

// Repository описывает контракт доступа к данным, используемый сервисом.
type Repository interface {
	Close() error
	CreateUser(ctx context.Context, login string, passwordHash []byte) (int64, error)
	GetUserByLogin(ctx context.Context, login string) (*model.User, error)
	AddOrder(ctx context.Context, userID int64, number string) (bool, error)
	GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error)
	GetBalance(ctx context.Context, userID int64) (int64, int64, error)
	CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sumCents int64) error
	GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error)
	GetOrdersForAccrual(ctx context.Context, limit int) ([]repository.OrderForAccrual, error)
	UpdateOrderAccrual(ctx context.Context, number string, status model.OrderStatus, accrualCents *int64) error
}

// Service содержит бизнес-логику сервиса гофермарт.
type Service struct {
	repo          Repository
	accrualClient *accrual.Client
}

// NewService создаёт новый сервис с указанным репозиторием и клиентом системы начислений.
func NewService(repo Repository, accrualClient *accrual.Client) *Service {
	return &Service{
		repo:          repo,
		accrualClient: accrualClient,
	}
}

// Close закрывает ресурсы сервиса.
func (s *Service) Close() error {
	if s.repo != nil {
		return s.repo.Close()
	}
	return nil
}

// RegisterUser регистрирует нового пользователя.
func (s *Service) RegisterUser(ctx context.Context, login, password string) (int64, error) {
	hashed := hashPassword(login, password)
	id, err := s.repo.CreateUser(ctx, login, hashed)
	if err != nil {
		if errors.Is(err, repository.ErrUserExists) {
			return 0, repository.ErrUserExists
		}
		return 0, err
	}
	return id, nil
}

// AuthenticateUser проверяет логин и пароль пользователя и возвращает его идентификатор.
func (s *Service) AuthenticateUser(ctx context.Context, login, password string) (int64, error) {
	u, err := s.repo.GetUserByLogin(ctx, login)
	if err != nil {
		return 0, err
	}

	hashed := hashPassword(login, password)
	if hex.EncodeToString(hashed) != hex.EncodeToString(u.PasswordHash) {
		return 0, errors.New("invalid credentials")
	}

	return u.ID, nil
}

func hashPassword(login, password string) []byte {
	sum := sha256.Sum256([]byte(login + ":" + password))
	return sum[:]
}

// AddOrder добавляет номер заказа пользователю.
func (s *Service) AddOrder(ctx context.Context, userID int64, number string) (bool, error) {
	return s.repo.AddOrder(ctx, userID, number)
}

// GetOrdersByUser возвращает список заказов пользователя.
func (s *Service) GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.repo.GetOrdersByUser(ctx, userID)
}

// GetBalance возвращает баланс пользователя в виде структуры model.Balance.
func (s *Service) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	current, withdrawn, err := s.repo.GetBalance(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.Balance{
		Current:   float64(current) / 100,
		Withdrawn: float64(withdrawn) / 100,
	}, nil
}

// CreateWithdrawal создаёт запрос на списание средств пользователя.
func (s *Service) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	sumCents := int64(sum * 100)
	if sumCents <= 0 {
		return errors.New("withdraw sum must be positive")
	}
	return s.repo.CreateWithdrawal(ctx, userID, order, sumCents)
}

// GetWithdrawalsByUser возвращает историю списаний пользователя.
func (s *Service) GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return s.repo.GetWithdrawalsByUser(ctx, userID)
}

// StartAccrualUpdates запускает фоновый процесс обновления статусов заказов из системы начислений.
func (s *Service) StartAccrualUpdates(ctx context.Context) {
	if s.accrualClient == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processAccrualBatch(ctx)
			}
		}
	}()
}

func (s *Service) processAccrualBatch(ctx context.Context) {
	orders, err := s.repo.GetOrdersForAccrual(ctx, 100)
	if err != nil {
		return
	}

	for _, o := range orders {
		resp, statusCode, retryAfter, err := s.accrualClient.GetOrderAccrual(ctx, o.Number)
		if err != nil {
			continue
		}

		if statusCode == 0 {
			continue
		}

		if statusCode == 429 {
			if retryAfter > 0 {
				timer := time.NewTimer(retryAfter)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			continue
		}

		if resp == nil {
			continue
		}

		var status model.OrderStatus
		var accrualCents *int64

		switch resp.Status {
		case "REGISTERED", "PROCESSING":
			status = model.OrderStatusProcessing
		case "INVALID":
			status = model.OrderStatusInvalid
		case "PROCESSED":
			status = model.OrderStatusProcessed
			if resp.Accrual != nil {
				v := int64(*resp.Accrual * 100)
				accrualCents = &v
			}
		default:
			continue
		}

		_ = s.repo.UpdateOrderAccrual(ctx, o.Number, status, accrualCents)
	}
}
