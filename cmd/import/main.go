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

	// Map to storage record
	rec, err := statement.MapToStatementRecord(&parsedStmt, time.Now())
	if err != nil {
		return fmt.Errorf("map statement record: %w", err)
	}

	// Insert statement
	logger.Info("inserting statement record", "period", rec.Period, "iban", rec.IBAN)
	statementID, err := statement.InsertStatement(ctx, pool, rec)
	if err != nil {
		if errors.Is(err, statement.ErrStatementExists) {
			logger.Info("statement already imported, skipping", "period", rec.Period, "iban", rec.IBAN)
			return nil
		}
		return fmt.Errorf("insert statement: %w", err)
	}

	// Map and insert transactions
	transactionRecords := statement.MapToTransactionRecords(statementID, parsedStmt.Transactions)
	
	logger.Info("inserting transactions", "count", len(transactionRecords), "statement_id", statementID)
	if err := statement.InsertTransactions(ctx, pool, transactionRecords); err != nil {
		return fmt.Errorf("insert transactions: %w", err)
	}

	logger.Info("import completed",
		"statement_id", statementID,
		"transactions", len(transactionRecords),
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
