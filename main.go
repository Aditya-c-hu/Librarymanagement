package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Aditya-c-hu/Librarymanagement/internal/config"
	"github.com/Aditya-c-hu/Librarymanagement/internal/database"
	"github.com/Aditya-c-hu/Librarymanagement/internal/handlers"
	"github.com/Aditya-c-hu/Librarymanagement/internal/middleware"
	"github.com/Aditya-c-hu/Librarymanagement/internal/models"
	"github.com/Aditya-c-hu/Librarymanagement/internal/services"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	// Services
	authSvc := services.NewAuthService(db, cfg.JWTSecret, cfg.TokenExpiry)
	bookSvc := services.NewBookService(db)
	checkoutSvc := services.NewCheckoutService(db, cfg)
	reservationSvc := services.NewReservationService(db, cfg)

	// Handlers
	authH := handlers.NewAuthHandler(authSvc)
	bookH := handlers.NewBookHandler(bookSvc)
	checkoutH := handlers.NewCheckoutHandler(checkoutSvc)
	reservationH := handlers.NewReservationHandler(reservationSvc)

	// Background goroutine to expire stale reservations every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			n, err := reservationSvc.ExpireStaleReservations()
			if err != nil {
				log.Printf("error expiring reservations: %v", err)
			} else if n > 0 {
				log.Printf("expired %d stale reservations", n)
			}
		}
	}()

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			next.ServeHTTP(w, r)
		})
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public routes
	r.Post("/api/auth/register", authH.Register)
	r.Post("/api/auth/login", authH.Login)

	// Public book browsing
	r.Get("/api/books", bookH.List)
	r.Get("/api/books/{id}", bookH.Get)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(authSvc))

		// Profile
		r.Get("/api/auth/me", authH.Me)

		// Student + Librarian: checkout operations
		r.Post("/api/checkouts", checkoutH.Checkout)
		r.Post("/api/checkouts/{id}/return", checkoutH.Return)
		r.Post("/api/checkouts/{id}/renew", checkoutH.Renew)
		r.Get("/api/checkouts/me", checkoutH.GetMyCheckouts)

		// Student + Librarian: reservation operations
		r.Post("/api/reservations", reservationH.Reserve)
		r.Delete("/api/reservations/{id}", reservationH.Cancel)
		r.Get("/api/reservations/me", reservationH.GetMyReservations)

		// Librarian-only routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole(models.RoleLibrarian))

			// Book management
			r.Post("/api/books", bookH.Create)
			r.Put("/api/books/{id}", bookH.Update)
			r.Delete("/api/books/{id}", bookH.Delete)
			r.Post("/api/books/{id}/copies", bookH.AddCopies)
			r.Get("/api/books/{id}/copies", bookH.GetCopies)
			r.Patch("/api/books/{id}/copies/{copyId}", bookH.UpdateCopyStatus)

			// Checkout management
			r.Get("/api/checkouts", checkoutH.GetAllCheckouts)
			r.Get("/api/checkouts/overdue", checkoutH.GetOverdue)
			r.Post("/api/checkouts/{id}/pay-fine", checkoutH.PayFine)

			// Reservation management
			r.Get("/api/reservations", reservationH.GetAll)
			r.Get("/api/books/{id}/reservations", reservationH.GetBookReservations)
		})
	})

	log.Printf("Library API server starting on %s", cfg.ServerAddr)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
