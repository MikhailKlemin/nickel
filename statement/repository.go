package statement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStatementExists = errors.New("statement already exists for this period")

// InsertStatement inserts a StatementRecord into the database and returns its ID.
// If a statement with the same period and IBAN already exists, it returns
// the existing statement's ID along with ErrStatementExists.
func InsertStatement(ctx context.Context, pool *pgxpool.Pool, rec *StatementRecord) (int64, error) {
	const insertSQL = `
		INSERT INTO statements (period, iban, uploaded_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (period, iban) DO NOTHING
		RETURNING id
	`

	var id int64
	err := pool.QueryRow(ctx, insertSQL, rec.Period, rec.IBAN, rec.UploadedAt).Scan(&id)
	if err == nil {
		return id, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict occurred, fetch existing ID
		const selectSQL = `SELECT id FROM statements WHERE period = $1 AND iban = $2`
		if err := pool.QueryRow(ctx, selectSQL, rec.Period, rec.IBAN).Scan(&id); err != nil {
			return 0, fmt.Errorf("fetch existing statement ID: %w", err)
		}
		return id, ErrStatementExists
	}

	return 0, fmt.Errorf("insert statement: %w", err)
}

// InsertTransactions inserts multiple TransactionRecords in a single batch.
// Conflicts on (statement_id, transaction_number) are ignored (DO NOTHING).
func InsertTransactions(ctx context.Context, pool *pgxpool.Pool, records []TransactionRecord) error {
	const insertSQL = `
		INSERT INTO transactions (
			statement_id, transaction_number, date, type, description, amount_cents, category
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (statement_id, transaction_number) DO NOTHING
	`

	batch := &pgx.Batch{}
	for _, rec := range records {
		batch.Queue(insertSQL,
			rec.StatementID,
			rec.TransactionNumber,
			rec.Date,
			rec.Type,
			rec.Description,
			rec.AmountCents,
			rec.Category,
		)
	}

	results := pool.SendBatch(ctx, batch)
	defer results.Close()

	// Use for range over records (not len(records))
	for range records {
		_, err := results.Exec()
		if err != nil {
			return fmt.Errorf("batch insert transaction: %w", err)
		}
	}

	if err := results.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	return nil
}
