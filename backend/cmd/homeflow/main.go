package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/documents"
	"homeflow/backend/internal/expenses"
	"homeflow/backend/internal/httpapi"
	"homeflow/backend/internal/links"
	"homeflow/backend/internal/observability"
	"homeflow/backend/internal/pets"
	"homeflow/backend/internal/postgres"
	"homeflow/backend/internal/receipts"
	"homeflow/backend/internal/reminders"
	"homeflow/backend/internal/shopping"
	"homeflow/backend/internal/vets"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("homeflow stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	environment := env("HOMEFLOW_ENV", "development")
	if environment != "development" && environment != "local" {
		return errors.New("HOMEFLOW_ENV must be development or local")
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	metrics := observability.New()
	pool, err := postgres.Open(startupCtx, os.Getenv("HOMEFLOW_DATABASE_URL"), metrics)
	if err != nil {
		return err
	}
	defer pool.Close()
	metrics.BindDatabase(pool)
	if err := postgres.Migrate(startupCtx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	bootstrapUsers := [2]auth.BootstrapUser{
		{Username: os.Getenv("HOMEFLOW_USER1_USERNAME"), DisplayName: os.Getenv("HOMEFLOW_USER1_DISPLAY_NAME"), Password: os.Getenv("HOMEFLOW_USER1_PASSWORD"), Role: "admin"},
		{Username: os.Getenv("HOMEFLOW_USER2_USERNAME"), DisplayName: os.Getenv("HOMEFLOW_USER2_DISPLAY_NAME"), Password: os.Getenv("HOMEFLOW_USER2_PASSWORD"), Role: "member"},
	}
	if err := auth.Bootstrap(startupCtx, pool, bootstrapUsers); err != nil {
		return fmt.Errorf("bootstrap users: %w", err)
	}

	authService := auth.NewPostgresService(pool)
	shoppingService := shopping.NewService(shopping.NewPostgresRepository(pool))
	expenseService := expenses.NewService(expenses.NewPostgresRepository(pool))
	receiptStorage, err := receipts.NewLocalStorage(env("HOMEFLOW_RECEIPT_STORAGE", "./storage/receipts"))
	if err != nil {
		return fmt.Errorf("initialize receipt storage: %w", err)
	}
	receiptService := receipts.NewService(receipts.NewPostgresRepository(pool), expenseService, receiptStorage)
	documentService := documents.NewService(documents.NewPostgresRepository(pool), receiptStorage)
	reminderService := reminders.NewService(reminders.NewPostgresRepository(pool))
	calendarService := calendar.NewService(calendar.NewPostgresRepository(pool))
	petService := pets.NewService(pets.NewPostgresRepository(pool))
	vetService := vets.NewService(vets.NewPostgresRepository(pool))
	linkService := links.NewService(links.NewPostgresRepository(pool))
	handler := httpapi.NewFullWithOptions(logger, authService, authService, shoppingService, expenseService, receiptService, httpapi.BrowserCookieConfig{
		Secure:    env("HOMEFLOW_COOKIE_SECURE", "false") == "true",
		WebRoot:   env("HOMEFLOW_WEB_ROOT", ""),
		Metrics:   metrics,
		Calendar:  calendarService,
		Pets:      petService,
		Documents: documentService,
		Reminders: reminderService,
		Links:     linkService,
		Vets:      vetService,
	})
	server := &http.Server{
		Addr:              env("HOMEFLOW_LISTEN_ADDR", ":8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("homeflow started", "address", server.Addr, "environment", environment)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
