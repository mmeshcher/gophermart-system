package accrual

import (
	"context"
	"encoding/json"
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

	res, code, retry, err := client.GetOrderAccrual(ctx, "123")
	if err != nil {
		t.Fatalf("GetOrderAccrual error: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", code, http.StatusOK)
	}
	if retry != 0 {
		t.Fatalf("retryAfter = %v, want 0", retry)
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
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, code, retry, err := client.GetOrderAccrual(ctx, "123")
	if err != nil {
		t.Fatalf("GetOrderAccrual error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response for 429, got %+v", res)
	}
	if code != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", code, http.StatusTooManyRequests)
	}
	if retry < 5*time.Second {
		t.Fatalf("retryAfter = %v, want at least 5s", retry)
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

	res, code, retry, err := client.GetOrderAccrual(ctx, "123")
	if err != nil {
		t.Fatalf("GetOrderAccrual error: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response for 204, got %+v", res)
	}
	if code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", code, http.StatusNoContent)
	}
	if retry != 0 {
		t.Fatalf("retryAfter = %v, want 0", retry)
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}

