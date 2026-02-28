package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetOrderAccrual_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/orders/123" {
			t.Fatalf("path = %s, want /api/orders/123", r.URL.Path)
		}

		resp := OrderAccrual{
			Order:   "123",
			Status:  "PROCESSED",
			Accrual: ptrFloat(10.5),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := client.GetOrderAccrual(ctx, "123")
	if err != nil {
		t.Fatalf("GetOrderAccrual error: %v", err)
	}
	if res == nil || res.Order != "123" || res.Status != "PROCESSED" {
		t.Fatalf("unexpected response: %+v", res)
	}
	if res.Accrual == nil || *res.Accrual != 10.5 {
		t.Fatalf("unexpected accrual: %v", res.Accrual)
	}
}

func TestGetOrderAccrual_TooManyRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetOrderAccrual(ctx, "123")
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}

	var tooManyErr *TooManyRequestsError
	if !errors.As(err, &tooManyErr) {
		t.Fatalf("expected TooManyRequestsError, got %T: %v", err, err)
	}
	if tooManyErr.RetryAfter != 1*time.Second {
		t.Fatalf("retryAfter = %v, want 1s", tooManyErr.RetryAfter)
	}
}

func TestGetOrderAccrual_NoContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := client.GetOrderAccrual(ctx, "123")
	if err == nil {
		t.Fatal("expected error for 204, got nil")
	}
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
