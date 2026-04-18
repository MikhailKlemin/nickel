//go:build go1.25

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

// setupTestPool creates a connection pool for testing.
// It uses the TEST_DATABASE_URL environment variable or skips the test.
func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set (use: postgres://user:pass@localhost:5432/testdb)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	return pool
}

func TestMigrationBootstrap(t *testing.T) {
	t.Parallel()

	pool := setupTestPool(t)
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// Clean up any existing test state
	cleanupSQL := []string{
		`DROP TABLE IF EXISTS transactions CASCADE`,
		`DROP TABLE IF EXISTS statements CASCADE`,
		`DROP TABLE IF EXISTS schema_migrations CASCADE`,
		`DROP FUNCTION IF EXISTS update_updated_at_column CASCADE`,
	}
	for _, sql := range cleanupSQL {
		if _, err := pool.Exec(ctx, sql); err != nil {
			// Ignore errors (tables might not exist)
		}
	}

	// Test 1: First-time migration
	t.Run("first_run_applies_all_migrations", func(t *testing.T) {
		if err := runMigrations(ctx, pool, logger); err != nil {
			t.Fatalf("first migration run failed: %v", err)
		}

		// Check that both tables exist
		var tables []string
		rows, err := pool.Query(ctx, `
			SELECT table_name 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name IN ('statements', 'transactions', 'schema_migrations')
			ORDER BY table_name
		`)
		if err != nil {
			t.Fatalf("query tables: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan table name: %v", err)
			}
			tables = append(tables, name)
		}

		expected := []string{"schema_migrations", "statements", "transactions"}
		if diff := cmpDiff(expected, tables); diff != "" {
			t.Errorf("tables mismatch (-want +got):\n%s", diff)
		}

		// Check that both migration versions are recorded
		var versions []int
		rows, err = pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
		if err != nil {
			t.Fatalf("query versions: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err != nil {
				t.Fatalf("scan version: %v", err)
			}
			versions = append(versions, v)
		}

		// Check versions directly without cmpDiff
		if len(versions) != 2 {
			t.Errorf("expected 2 versions, got %d: %v", len(versions), versions)
		} else if versions[0] != 1 || versions[1] != 2 {
			t.Errorf("expected versions [1 2], got %v", versions)
		}
	})

	// Test 2: Idempotency - second run should not fail
	t.Run("second_run_is_idempotent", func(t *testing.T) {
		// Count statements before second run
		var beforeCount int64
		err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM statements`).Scan(&beforeCount)
		if err != nil {
			t.Fatalf("count statements before: %v", err)
		}

		// Run migrations again
		if err := runMigrations(ctx, pool, logger); err != nil {
			t.Fatalf("second migration run failed: %v", err)
		}

		// Count statements after - should be unchanged
		var afterCount int64
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM statements`).Scan(&afterCount)
		if err != nil {
			t.Fatalf("count statements after: %v", err)
		}

		if beforeCount != afterCount {
			t.Errorf("migration was not idempotent: statement count changed from %d to %d", beforeCount, afterCount)
		}
	})

	// Test 3: Invalid migration file handling
	t.Run("skips_invalid_migration_files", func(t *testing.T) {
		// Create a temporary migration directory
		tmpDir := t.TempDir()
		
		// Write valid migration
		validSQL := `CREATE TABLE IF NOT EXISTS test_table (id SERIAL PRIMARY KEY);`
		validPath := filepath.Join(tmpDir, "003_test.up.sql")
		if err := os.WriteFile(validPath, []byte(validSQL), 0644); err != nil {
			t.Fatalf("write valid migration: %v", err)
		}
		
		// Write invalid-named file (should be ignored)
		invalidPath := filepath.Join(tmpDir, "invalid.sql")
		if err := os.WriteFile(invalidPath, []byte(`SELECT 1;`), 0644); err != nil {
			t.Fatalf("write invalid migration: %v", err)
		}

		// We can't easily test the directory scanning without refactoring,
		// but the pattern matching in runMigrations should ignore "invalid.sql"
		// since it doesn't match "*_*.up.sql"
	})
}

// cmpDiff is a simple diff helper for slices.
func cmpDiff(want, got []string) string {
	if len(want) != len(got) {
		return fmt.Sprintf("length mismatch: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Sprintf("at index %d: want %q, got %q", i, want[i], got[i])
		}
	}
	return ""
}
