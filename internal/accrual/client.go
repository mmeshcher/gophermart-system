// Package accrual предоставляет клиент для внешней системы начислений.
package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// ErrOrderNotFound возвращается, если информация о заказе не найдена.
var ErrOrderNotFound = errors.New("order not found in accrual system")

// TooManyRequestsError описывает ошибку превышения лимита запросов.
type TooManyRequestsError struct {
	RetryAfter time.Duration
}

func (e *TooManyRequestsError) Error() string {
	return fmt.Sprintf("too many requests, retry after %s", e.RetryAfter)
}

// Client инкапсулирует HTTP-взаимодействие с системой начислений.
type Client struct {
	baseURL    string
	httpClient *retryablehttp.Client
}

// OrderAccrual описывает ответ системы начислений по одному заказу.
type OrderAccrual struct {
	Order   string   `json:"order"`
	Status  string   `json:"status"`
	Accrual *float64 `json:"accrual,omitempty"`
}

// NewClient создаёт HTTP-клиент для обращения к системе начислений по указанному адресу.
func NewClient(baseURL string) *Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 2 * time.Second
	// Отключаем логгирование retryablehttp, чтобы оно не мешало основному логгеру
	retryClient.Logger = nil

	// Кастомная проверка для 429, чтобы не ретраить её автоматически, а вернуть ошибку сразу
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return false, nil
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: retryClient,
	}
}

// GetOrderAccrual запрашивает информацию о начислении баллов для указанного номера заказа.
func (c *Client) GetOrderAccrual(ctx context.Context, number string) (*OrderAccrual, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("accrual client not configured")
	}

	base := c.baseURL
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}

	url := fmt.Sprintf("%s/api/orders/%s", base, number)

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 0 * time.Second
		if v := resp.Header.Get("Retry-After"); v != "" {
			if seconds, parseErr := strconv.Atoi(v); parseErr == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return nil, &TooManyRequestsError{RetryAfter: retryAfter}
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, ErrOrderNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result OrderAccrual
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
