//go:build go1.25

package statement

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStatementExists = errors.New("statement already exists for this period")

// ImportResult is returned by ImportStatement and carries the statement ID
// together with any soft warnings that did not prevent the import.
type ImportResult struct {
	StatementID     int64
	SkippedTxBlocks int // number of transaction blocks the parser could not parse
}

// ImportStatement inserts the statement row and all its transactions in a
// single database transaction, so the database is never left with a statement
// row that has no (or partial) transactions.
//
// If a statement with the same (period, iban) already exists, the function
// returns ErrStatementExists without modifying any data.
func ImportStatement(ctx context.Context, pool *pgxpool.Pool, rec *StatementRecord, txRecords []TransactionRecord) (ImportResult, error) {
	var result ImportResult

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := insertStatementTx(ctx, tx, rec)
		if err != nil {
			return err // includes ErrStatementExists; tx rolls back automatically
		}
		result.StatementID = id

		// Stamp every record with the real DB id now that we have it.
		for i := range txRecords {
			txRecords[i].StatementID = id
		}

		return insertTransactionsTx(ctx, tx, txRecords)
	})
	if err != nil {
		return ImportResult{}, err
	}

	return result, nil
}

// insertStatementTx runs the statement upsert inside an existing transaction.
func insertStatementTx(ctx context.Context, tx pgx.Tx, rec *StatementRecord) (int64, error) {
	const insertSQL = `
		INSERT INTO statements (period, iban, uploaded_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (period, iban) DO NOTHING
		RETURNING id
	`

	var id int64
	err := tx.QueryRow(ctx, insertSQL, rec.Period, rec.IBAN, rec.UploadedAt).Scan(&id)
	if err == nil {
		return id, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict: a statement for this period+IBAN already exists.
		// Fetch its id so the caller can surface a meaningful error.
		const selectSQL = `SELECT id FROM statements WHERE period = $1 AND iban = $2`
		if err := tx.QueryRow(ctx, selectSQL, rec.Period, rec.IBAN).Scan(&id); err != nil {
			return 0, fmt.Errorf("fetch existing statement id: %w", err)
		}
		return 0, fmt.Errorf("%w (id %d)", ErrStatementExists, id)
	}

	return 0, fmt.Errorf("insert statement: %w", err)
}

// insertTransactionsTx bulk-inserts transaction records inside an existing
// transaction using a pgx pipeline batch.
// Conflicts on (statement_id, transaction_number) are silently ignored so
// that re-importing an already-stored statement is idempotent.
func insertTransactionsTx(ctx context.Context, tx pgx.Tx, records []TransactionRecord) error {
	if len(records) == 0 {
		return nil
	}

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

	results := tx.SendBatch(ctx, batch)

	for range records {
		if _, err := results.Exec(); err != nil {
			closeErr := results.Close()
			return errors.Join(fmt.Errorf("batch insert transaction: %w", err), closeErr)
		}
	}

	if err := results.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	return nil
}
