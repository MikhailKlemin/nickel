//go:build go1.25

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

	"nickel/api"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Use structured logging with text output to stderr
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := context.Background()
	if err := run(ctx, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	// Read configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL environment variable is required")
	}

	// Create database connection pool
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}
	defer pool.Close()

	// Verify database connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	logger.Info("database connection established")

	// Run migrations
	if err := runMigrations(ctx, pool, logger); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}

	// Initialize API server
	server := api.NewServer(pool, logger)
	handler := server.Handler()

	// Configure HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown handling
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-shutdown:
		logger.Info("shutdown signal received", "signal", sig)
	case err := <-serverErr:
		logger.Error("server error", "err", err)
		return err
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}

// runMigrations executes SQL migration files.
// For simplicity, we're using a basic approach. In production,
// consider using a dedicated migration tool like golang-migrate.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	// Check if migrations table exists
	const checkTableSQL = `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'schema_migrations'
		)`
	
	var migrationsTableExists bool
	err := pool.QueryRow(ctx, checkTableSQL).Scan(&migrationsTableExists)
	if err != nil {
		return fmt.Errorf("failed to check migrations table: %w", err)
	}

	// Create migrations table if it doesn't exist
	if !migrationsTableExists {
		const createMigrationsTableSQL = `
			CREATE TABLE schema_migrations (
				version BIGINT PRIMARY KEY,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`
		if _, err := pool.Exec(ctx, createMigrationsTableSQL); err != nil {
			return fmt.Errorf("failed to create migrations table: %w", err)
		}
		logger.Info("created schema_migrations table")
	}

	// Check if initial migration has been applied
	const checkMigrationSQL = `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 1)`
	var migrationApplied bool
	err = pool.QueryRow(ctx, checkMigrationSQL).Scan(&migrationApplied)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if migrationApplied {
		logger.Info("migration 001 already applied")
		return nil
	}

	// Read and execute migration file
	migrationSQL, err := os.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	// Execute migration within a transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(migrationSQL)); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record migration version
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (1)`); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	logger.Info("applied migration 001")
	return nil
}
