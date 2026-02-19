package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_WithValidCookie(t *testing.T) {
	m := NewAuthMiddleware("test-secret")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		id, ok := GetUserIDFromContext(r.Context())
		if !ok {
			t.Fatalf("user id not in context")
		}
		if id != 42 {
			t.Fatalf("user id from context = %d, want 42", id)
		}
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/protected", nil)

	m.SetAuthCookie(w, 42)
	res := w.Result()
	resCookies := res.Cookies()
	if len(resCookies) == 0 {
		t.Fatalf("no cookies set by SetAuthCookie")
	}

	r.AddCookie(resCookies[0])

	handler := m.Middleware(next)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if !nextCalled {
		t.Fatalf("next handler was not called")
	}
}

func TestAuthMiddleware_WithoutCookie(t *testing.T) {
	m := NewAuthMiddleware("test-secret")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/protected", nil)

	handler := m.Middleware(next)
	handler.ServeHTTP(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
	}
}

