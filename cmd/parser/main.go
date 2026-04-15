//go:build go1.25

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"nickel/statement"

	"github.com/jackc/pgx/v5/pgxpool"
)

func run(ctx context.Context, args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("nickel-import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	filePath := fs.String("file", "", "path to statement file (PDF or TXT)")
	dsn := fs.String("dsn", "", "PostgreSQL connection string (or use DATABASE_URL env var)")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	// Validate required flags
	if *filePath == "" {
		return fmt.Errorf("missing required flag -file")
	}

	if *dsn == "" {
		*dsn = os.Getenv("DATABASE_URL")
		if *dsn == "" {
			return fmt.Errorf("missing DSN: use -dsn flag or DATABASE_URL environment variable")
		}
	}

	// Parse statement file
	logger.Info("parsing statement file", "path", *filePath)
	parsedStmt, err := statement.ParseFile(ctx, *filePath, logger)
	if err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	// Open database connection
	logger.Info("connecting to database")
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Map to storage records
	rec, err := statement.MapToStatementRecord(&parsedStmt, time.Now())
	if err != nil {
		return fmt.Errorf("map statement record: %w", err)
	}
	txRecords := statement.MapToTransactionRecords(0, parsedStmt.Transactions) // StatementID stamped inside ImportStatement

	// Import atomically: statement row + transactions in one DB transaction.
	logger.Info("importing statement", "period", rec.Period, "iban", rec.IBAN, "transactions", len(txRecords))
	result, err := statement.ImportStatement(ctx, pool, rec, txRecords)
	if err != nil {
		if errors.Is(err, statement.ErrStatementExists) {
			logger.Info("statement already imported, skipping", "period", rec.Period, "iban", rec.IBAN)
			return nil
		}
		return fmt.Errorf("import statement: %w", err)
	}

	if parsedStmt.SkippedTxBlocks > 0 {
		logger.Warn("some transaction blocks could not be parsed and were skipped",
			"skipped", parsedStmt.SkippedTxBlocks,
			"imported", len(txRecords),
		)
	}

	logger.Info("import completed",
		"statement_id", result.StatementID,
		"transactions", len(txRecords),
		"skipped", parsedStmt.SkippedTxBlocks,
		"period", rec.Period,
		"iban", rec.IBAN,
	)

	return nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(context.Background(), os.Args, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}
