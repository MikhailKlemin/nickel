//go:build go1.25

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nickel/api"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
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
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL environment variable is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	logger.Info("database connection established")

	if err := runMigrations(ctx, pool, logger); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}

	server := api.NewServer(pool, logger)
	handler := server.Handler()

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case sig := <-shutdown:
		logger.Info("shutdown signal received", "signal", sig)
	case err := <-serverErr:
		logger.Error("server error", "err", err)
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}

type migrationFile struct {
	version int
	path    string
}

// runMigrations executes all SQL migration files in the migrations directory.
// Migrations are applied in version order (001_, 002_, etc.) and each version
// is recorded in the schema_migrations table to guarantee idempotency.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	const createTableSQL = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`

	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	migrations, err := collectMigrationFiles("migrations")
	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		logger.Warn("no migration files found in migrations/ directory")
		return nil
	}

	for _, mig := range migrations {
		applied, err := migrationApplied(ctx, pool, mig.version)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", mig.version, err)
		}
		if applied {
			logger.Info("migration already applied", "version", mig.version, "file", filepath.Base(mig.path))
			continue
		}

		if err := applyMigration(ctx, pool, mig); err != nil {
			return err
		}

		logger.Info("applied migration", "version", mig.version, "file", filepath.Base(mig.path))
	}

	return nil
}

func collectMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var migrations []migrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil || version <= 0 {
			continue
		}

		migrations = append(migrations, migrationFile{
			version: version,
			path:    filepath.Join(dir, name),
		})
	}

	slices.SortFunc(migrations, func(a, b migrationFile) int {
		return cmp.Compare(a.version, b.version)
	})

	return migrations, nil
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, version int) (bool, error) {
	const checkSQL = `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`

	var applied bool
	if err := pool.QueryRow(ctx, checkSQL, version).Scan(&applied); err != nil {
		return false, err
	}

	return applied, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, mig migrationFile) error {
	sqlBytes, err := os.ReadFile(mig.path)
	if err != nil {
		return fmt.Errorf("read migration file %q: %w", mig.path, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction for migration %d: %w", mig.version, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("execute migration %d (%s): %w", mig.version, filepath.Base(mig.path), err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, mig.version); err != nil {
		return fmt.Errorf("record migration %d: %w", mig.version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", mig.version, err)
	}

	return nil
}
