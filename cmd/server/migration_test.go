//go:build go1.25

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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

	cleanupSQL := []string{
		`DROP TABLE IF EXISTS transactions CASCADE`,
		`DROP TABLE IF EXISTS statements CASCADE`,
		`DROP TABLE IF EXISTS schema_migrations CASCADE`,
		`DROP FUNCTION IF EXISTS update_updated_at_column CASCADE`,
	}

	for _, sql := range cleanupSQL {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("cleanup failed for %q: %v", sql, err)
		}
	}

	t.Run("first_run_applies_all_migrations", func(t *testing.T) {
		if err := runMigrations(ctx, pool, logger); err != nil {
			t.Fatalf("first migration run failed: %v", err)
		}

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
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate table rows: %v", err)
		}

		expectedTables := []string{"schema_migrations", "statements", "transactions"}
		if diff := cmpDiff(expectedTables, tables); diff != "" {
			t.Errorf("tables mismatch (-want +got):\n%s", diff)
		}

		versions := readMigrationVersions(t, ctx, pool)
		expectedVersions := []int{1, 2}
		if diff := cmpDiffInts(expectedVersions, versions); diff != "" {
			t.Errorf("migration versions mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("second_run_is_idempotent", func(t *testing.T) {
		beforeVersions := readMigrationVersions(t, ctx, pool)

		if err := runMigrations(ctx, pool, logger); err != nil {
			t.Fatalf("second migration run failed: %v", err)
		}

		afterVersions := readMigrationVersions(t, ctx, pool)

		if diff := cmpDiffInts(beforeVersions, afterVersions); diff != "" {
			t.Errorf("schema_migrations changed after second run (-before +after):\n%s", diff)
		}
	})
}

func readMigrationVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []int {
	t.Helper()

	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query versions: %v", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate version rows: %v", err)
	}

	return versions
}

// cmpDiff is a simple diff helper for string slices.
func cmpDiff(want, got []string) string {
	if slices.Equal(want, got) {
		return ""
	}
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

func cmpDiffInts(want, got []int) string {
	if slices.EqualFunc(want, got, func(a, b int) bool { return a == b }) {
		return ""
	}
	if len(want) != len(got) {
		return fmt.Sprintf("length mismatch: want %d, got %d", len(want), len(got))
	}

	for i := range want {
		if want[i] != got[i] {
			return fmt.Sprintf("at index %d: want %d, got %d", i, want[i], got[i])
		}
	}

	return ""
}
