// Package accrual предоставляет клиент для внешней системы начислений.
package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client инкапсулирует HTTP-взаимодействие с системой начислений.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// OrderAccrual описывает ответ системы начислений по одному заказу.
type OrderAccrual struct {
	Order   string   `json:"order"`
	Status  string   `json:"status"`
	Accrual *float64 `json:"accrual,omitempty"`
}

// NewClient создаёт HTTP-клиент для обращения к системе начислений по указанному адресу.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetOrderAccrual запрашивает информацию о начислении баллов для указанного номера заказа.
func (c *Client) GetOrderAccrual(ctx context.Context, number string) (*OrderAccrual, int, time.Duration, error) {
	if c == nil || c.baseURL == "" {
		return nil, 0, 0, fmt.Errorf("accrual client not configured")
	}

	base := c.baseURL
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}

	url := fmt.Sprintf("%s/api/orders/%s", base, number)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := time.Duration(0)
		if v := resp.Header.Get("Retry-After"); v != "" {
			if seconds, parseErr := strconv.Atoi(v); parseErr == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return nil, resp.StatusCode, retryAfter, nil
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, resp.StatusCode, 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result OrderAccrual
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, 0, fmt.Errorf("decode response: %w", err)
	}

	return &result, resp.StatusCode, 0, nil
}
