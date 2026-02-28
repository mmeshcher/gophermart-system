package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	custommiddleware "github.com/mmeshcher/gophermart-system/internal/middleware"
)

// SetupRouter настраивает HTTP-маршруты и middleware сервиса гофермарт.
func (h *Handler) SetupRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(custommiddleware.GzipMiddleware)
	r.Use(custommiddleware.Logger(h.logger))

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware.Middleware)

			r.Post("/orders", h.UploadOrder)
			r.Get("/orders", h.GetOrders)

			r.Get("/balance", h.GetBalance)
			r.Post("/balance/withdraw", h.Withdraw)

			r.Get("/withdrawals", h.GetWithdrawals)
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	return r
}
