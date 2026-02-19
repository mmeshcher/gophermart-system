// Package middleware содержит HTTP middleware для сервиса гофермарт.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type contextKey string

const userIDKey contextKey = "userID"

const (
	authCookieName = "auth_token"
	authCookieTTL  = 365 * 24 * time.Hour
)

// AuthMiddleware выполняет проверку аутентификации пользователя по подписанному cookie.
type AuthMiddleware struct {
	secretKey []byte
}

// NewAuthMiddleware создаёт новый экземпляр AuthMiddleware с указанным секретным ключом.
func NewAuthMiddleware(secret string) *AuthMiddleware {
	key := []byte(secret)
	if len(key) == 0 {
		randomKey := make([]byte, 32)
		if _, err := rand.Read(randomKey); err == nil {
			key = randomKey
		} else {
			key = []byte("default-secret-key")
		}
	}

	return &AuthMiddleware{
		secretKey: key,
	}
}

// Middleware проверяет cookie авторизации и добавляет идентификатор пользователя в контекст запроса.
func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		userID, ok := a.parseCookie(cookie.Value)
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetAuthCookie устанавливает cookie авторизации для указанного идентификатора пользователя.
func (a *AuthMiddleware) SetAuthCookie(w http.ResponseWriter, userID int64) {
	value := a.signUserID(userID)

	cookie := &http.Cookie{
		Name:     authCookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Now().Add(authCookieTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
}

func (a *AuthMiddleware) signUserID(userID int64) string {
	mac := hmac.New(sha256.New, a.secretKey)
	idStr := strconv.FormatInt(userID, 10)
	mac.Write([]byte(idStr))
	signature := mac.Sum(nil)
	return idStr + "." + hex.EncodeToString(signature)
}

func (a *AuthMiddleware) parseCookie(cookieValue string) (int64, bool) {
	parts := strings.Split(cookieValue, ".")
	if len(parts) != 2 {
		return 0, false
	}

	idStr := parts[0]
	signature := parts[1]

	expected := a.signUserIDFromString(idStr)
	expectedParts := strings.Split(expected, ".")
	if len(expectedParts) != 2 {
		return 0, false
	}

	if !hmac.Equal([]byte(signature), []byte(expectedParts[1])) {
		return 0, false
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, false
	}

	return id, true
}

func (a *AuthMiddleware) signUserIDFromString(idStr string) string {
	mac := hmac.New(sha256.New, a.secretKey)
	mac.Write([]byte(idStr))
	signature := mac.Sum(nil)
	return idStr + "." + hex.EncodeToString(signature)
}

// GetUserIDFromContext извлекает идентификатор пользователя из контекста запроса.
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}
