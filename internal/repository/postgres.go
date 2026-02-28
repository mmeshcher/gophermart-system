// Package repository содержит реализацию доступа к данным в PostgreSQL.
package repository

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mmeshcher/gophermart-system/internal/model"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrUserExists возвращается при попытке создать пользователя с уже существующим логином.
var (
	ErrUserExists = errors.New("user already exists")
	// ErrUserNotFound возвращается, если пользователь не найден.
	ErrUserNotFound = errors.New("user not found")
	// ErrOrderOwnedByAnother возвращается, если номер заказа принадлежит другому пользователю.
	ErrOrderOwnedByAnother = errors.New("order already uploaded by another user")
	// ErrInsufficientBalance возвращается при попытке списания суммы, превышающей баланс.
	ErrInsufficientBalance = errors.New("insufficient balance")
)

// PostgresRepository предоставляет доступ к хранилищу данных в PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository создаёт новый репозиторий и инициализирует схему БД через миграции.
func NewPostgresRepository(dsn string) (*PostgresRepository, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	r := &PostgresRepository{pool: pool}

	if err := r.runMigrations(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return r, nil
}

func (r *PostgresRepository) runMigrations(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(r.pool)
	defer db.Close()

	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

func (r *PostgresRepository) withRetry(ctx context.Context, fn func() error) error {
	var err error
	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

	for i := 0; i <= len(delays); i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Если ошибка контекста — выходим сразу
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		// Проверяем, является ли ошибка временной (сетевой)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			// Можно добавить коды ошибок, которые стоит ретраить (например, Connection Failure)
			// Но обычно pgxpool сам справляется с переподключением.
			// Ретраи полезны для Serialization Failure или Deadlocks.
			if pgErr.Code == pgerrcode.SerializationFailure || pgErr.Code == pgerrcode.DeadlockDetected {
				if i < len(delays) {
					time.Sleep(delays[i])
					continue
				}
			}
		}

		// Если это не pg-ошибка, но сетевая
		if isConnectionError(err) {
			if i < len(delays) {
				time.Sleep(delays[i])
				continue
			}
		}

		break
	}
	return err
}

func isConnectionError(err error) bool {
	// Упрощенная проверка на ошибки соединения
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "connection reset by peer")
}

// Close закрывает пул соединений с БД.
func (r *PostgresRepository) Close() error {
	r.pool.Close()
	return nil
}

// CreateUser создаёт нового пользователя.
func (r *PostgresRepository) CreateUser(ctx context.Context, login string, passwordHash []byte) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
		login, passwordHash,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return 0, fmt.Errorf("%w: %s", ErrUserExists, login)
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// GetUserByLogin возвращает пользователя по логину.
func (r *PostgresRepository) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, login, password_hash, created_at FROM users WHERE login = $1`,
		login,
	)

	var u model.User
	err := row.Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &u, nil
}

// AddOrder сохраняет номер заказа и возвращает признак того, что он уже существовал у пользователя.
func (r *PostgresRepository) AddOrder(ctx context.Context, userID int64, number string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	cmdTag, err := tx.Exec(ctx,
		`INSERT INTO orders (number, user_id, status) VALUES ($1, $2, $3) ON CONFLICT (number) DO NOTHING`,
		number, userID, string(model.OrderStatusNew),
	)
	if err != nil {
		return false, fmt.Errorf("insert order: %w", err)
	}

	inserted := cmdTag.RowsAffected() == 1

	var existingUserID int64
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM orders WHERE number = $1`,
		number,
	).Scan(&existingUserID)
	if err != nil {
		return false, fmt.Errorf("select existing order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}

	if existingUserID == userID {
		return !inserted, nil
	}

	return false, ErrOrderOwnedByAnother
}

// GetOrdersByUser возвращает список заказов пользователя.
func (r *PostgresRepository) GetOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT number, status, accrual, uploaded_at 
		 FROM orders 
		 WHERE user_id = $1 
		 ORDER BY uploaded_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("select orders: %w", err)
	}
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var (
			number     string
			status     string
			accrualC   *int64
			uploadedAt time.Time
		)
		if err := rows.Scan(&number, &status, &accrualC, &uploadedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		var accrual *float64
		if accrualC != nil {
			v := float64(*accrualC) / 100
			accrual = &v
		}

		orders = append(orders, model.Order{
			Number:     number,
			Status:     model.OrderStatus(status),
			Accrual:    accrual,
			UploadedAt: uploadedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return orders, nil
}

// GetBalance возвращает доступный баланс и сумму всех списаний пользователя в копейках.
func (r *PostgresRepository) GetBalance(ctx context.Context, userID int64) (int64, int64, error) {
	var accrualTotal int64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(accrual), 0) 
		 FROM orders 
		 WHERE user_id = $1 AND status = $2`,
		userID, string(model.OrderStatusProcessed),
	).Scan(&accrualTotal)
	if err != nil {
		return 0, 0, fmt.Errorf("sum accruals: %w", err)
	}

	var withdrawnTotal int64
	err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(sum), 0) 
		 FROM withdrawals 
		 WHERE user_id = $1`,
		userID,
	).Scan(&withdrawnTotal)
	if err != nil {
		return 0, 0, fmt.Errorf("sum withdrawals: %w", err)
	}

	current := accrualTotal - withdrawnTotal
	if current < 0 {
		current = 0
	}

	return current, withdrawnTotal, nil
}

// CreateWithdrawal создаёт запись о списании средств. Использует блокировку строки пользователя для сериализации списаний.
func (r *PostgresRepository) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, sumCents int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Блокируем строку пользователя для предотвращения параллельных списаний, превышающих баланс.
	var dummy int
	err = tx.QueryRow(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&dummy)
	if err != nil {
		return fmt.Errorf("lock user for update: %w", err)
	}

	var accrualTotal int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(accrual), 0) 
		 FROM orders 
		 WHERE user_id = $1 AND status = $2`,
		userID, string(model.OrderStatusProcessed),
	).Scan(&accrualTotal)
	if err != nil {
		return fmt.Errorf("sum accruals: %w", err)
	}

	var withdrawnTotal int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(sum), 0) 
		 FROM withdrawals 
		 WHERE user_id = $1`,
		userID,
	).Scan(&withdrawnTotal)
	if err != nil {
		return fmt.Errorf("sum withdrawals: %w", err)
	}

	current := accrualTotal - withdrawnTotal
	if sumCents > current {
		return ErrInsufficientBalance
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO withdrawals (user_id, order_number, sum) VALUES ($1, $2, $3)`,
		userID, orderNumber, sumCents,
	)
	if err != nil {
		return fmt.Errorf("insert withdrawal: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// GetWithdrawalsByUser возвращает историю списаний пользователя.
func (r *PostgresRepository) GetWithdrawalsByUser(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT order_number, sum, processed_at 
		 FROM withdrawals 
		 WHERE user_id = $1 
		 ORDER BY processed_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("select withdrawals: %w", err)
	}
	defer rows.Close()

	var res []model.Withdrawal
	for rows.Next() {
		var (
			orderNumber string
			sumCents    int64
			processedAt time.Time
		)

		if err := rows.Scan(&orderNumber, &sumCents, &processedAt); err != nil {
			return nil, fmt.Errorf("scan withdrawal: %w", err)
		}

		res = append(res, model.Withdrawal{
			Order:       orderNumber,
			Sum:         float64(sumCents) / 100,
			ProcessedAt: processedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return res, nil
}

// OrderForAccrual описывает заказ, ожидающий получения начислений.
type OrderForAccrual struct {
	Number string
	Status model.OrderStatus
}

// GetOrdersForAccrual возвращает заказы, для которых нужно запросить начисления.
func (r *PostgresRepository) GetOrdersForAccrual(ctx context.Context, limit int) ([]OrderForAccrual, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT number, status 
		 FROM orders 
		 WHERE status IN ($1, $2)
		 ORDER BY uploaded_at
		 LIMIT $3`,
		string(model.OrderStatusNew),
		string(model.OrderStatusProcessing),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("select orders for accrual: %w", err)
	}
	defer rows.Close()

	var res []OrderForAccrual
	for rows.Next() {
		var number string
		var status string
		if err := rows.Scan(&number, &status); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		res = append(res, OrderForAccrual{
			Number: number,
			Status: model.OrderStatus(status),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return res, nil
}

// UpdateOrderAccrual обновляет статус заказа и сумму начисления.
func (r *PostgresRepository) UpdateOrderAccrual(ctx context.Context, number string, status model.OrderStatus, accrualCents *int64) error {
	if accrualCents == nil {
		_, err := r.pool.Exec(ctx,
			`UPDATE orders SET status = $2 WHERE number = $1`,
			number, string(status),
		)
		if err != nil {
			return fmt.Errorf("update order: %w", err)
		}
		return nil
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE orders SET status = $2, accrual = $3 WHERE number = $1`,
		number, string(status), *accrualCents,
	)
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}

	return nil
}
