//go:build go1.25

package statement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// ---------------------------------------------------------------------------
// Row types returned from DB queries
// ---------------------------------------------------------------------------

type StatementRow struct {
	ID         int64
	Period     string
	IBAN       string
	UploadedAt time.Time
	TxCount    int64
}

type TransactionRow struct {
	ID                int64
	StatementID       int64
	TransactionNumber int
	Date              time.Time
	Type              string
	Description       string
	AmountCents       int64
	Category          *string
}

// ListCategories returns all distinct non-null categories ordered alphabetically.
func ListCategories(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	const q = `
		SELECT DISTINCT category
		FROM transactions
		WHERE category IS NOT NULL
		ORDER BY category
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		results = append(results, cat)
	}
	return results, rows.Err()
}

// TransactionFilter holds optional narrowing criteria for ListTransactions
// and CountTransactions. Zero values mean "no filter on this field".
type TransactionFilter struct {
	StatementID *int64
	DateFrom    *time.Time
	DateTo      *time.Time
	Type        *string
	// "uncategorized" is a sentinel that maps to IS NULL.
	// Any other non-empty string filters by exact match.
	Category *string
	Limit    int // default 50, max 200 (enforced by the handler)
	Offset   int
}

// scanTxFields scans the 8 standard transaction columns into r.
// It is intentionally a free function so both pgx.Row and pgx.Rows
// call sites can use it without the pgx.Row interface mismatch.
func scanTxFields(r *TransactionRow, scan func(...any) error) error {
	return scan(
		&r.ID, &r.StatementID, &r.TransactionNumber,
		&r.Date, &r.Type, &r.Description, &r.AmountCents, &r.Category,
	)
}

// ---------------------------------------------------------------------------
// Statement queries
// ---------------------------------------------------------------------------

func ListStatements(ctx context.Context, pool *pgxpool.Pool) ([]StatementRow, error) {
	const q = `
		SELECT s.id, s.period, s.iban, s.uploaded_at, COUNT(t.id) AS tx_count
		FROM statements s
		LEFT JOIN transactions t ON t.statement_id = s.id
		GROUP BY s.id
		ORDER BY s.period DESC
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list statements: %w", err)
	}
	defer rows.Close()

	var results []StatementRow
	for rows.Next() {
		var r StatementRow
		if err := rows.Scan(&r.ID, &r.Period, &r.IBAN, &r.UploadedAt, &r.TxCount); err != nil {
			return nil, fmt.Errorf("scan statement row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func GetStatementByID(ctx context.Context, pool *pgxpool.Pool, id int64) (*StatementRow, error) {
	const q = `
		SELECT s.id, s.period, s.iban, s.uploaded_at, COUNT(t.id) AS tx_count
		FROM statements s
		LEFT JOIN transactions t ON t.statement_id = s.id
		WHERE s.id = $1
		GROUP BY s.id
	`
	var r StatementRow
	err := pool.QueryRow(ctx, q, id).Scan(&r.ID, &r.Period, &r.IBAN, &r.UploadedAt, &r.TxCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get statement %d: %w", id, err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Transaction queries
// ---------------------------------------------------------------------------

// transactionWhere builds the WHERE clause and args slice from a filter.
// Placeholder indices start at $1 and increment for each active condition.
func transactionWhere(f TransactionFilter) (clause string, args []any) {
	var conditions []string

	if f.StatementID != nil {
		args = append(args, *f.StatementID)
		conditions = append(conditions, fmt.Sprintf("t.statement_id = $%d", len(args)))
	}
	if f.DateFrom != nil {
		args = append(args, *f.DateFrom)
		conditions = append(conditions, fmt.Sprintf("t.date >= $%d", len(args)))
	}
	if f.DateTo != nil {
		args = append(args, *f.DateTo)
		conditions = append(conditions, fmt.Sprintf("t.date <= $%d", len(args)))
	}
	if f.Type != nil {
		args = append(args, *f.Type)
		conditions = append(conditions, fmt.Sprintf("t.type = $%d", len(args)))
	}
	if f.Category != nil {
		if *f.Category == "uncategorized" {
			conditions = append(conditions, "t.category IS NULL")
		} else {
			args = append(args, *f.Category)
			conditions = append(conditions, fmt.Sprintf("t.category = $%d", len(args)))
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

const transactionCols = `
	t.id, t.statement_id, t.transaction_number,
	t.date, t.type, t.description, t.amount_cents, t.category
`

func scanTransactionRow(row pgx.Row) (*TransactionRow, error) {
	var r TransactionRow
	if err := scanTxFields(&r, row.Scan); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("scan transaction: %w", err)
	}
	return &r, nil
}

func CountTransactions(ctx context.Context, pool *pgxpool.Pool, f TransactionFilter) (int64, error) {
	where, args := transactionWhere(f)
	q := fmt.Sprintf("SELECT COUNT(*) FROM transactions t %s", where)

	var n int64
	if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count transactions: %w", err)
	}
	return n, nil
}

func ListTransactions(ctx context.Context, pool *pgxpool.Pool, f TransactionFilter) ([]TransactionRow, error) {
	where, args := transactionWhere(f)

	// LIMIT and OFFSET placeholders follow any WHERE args.
	args = append(args, f.Limit, f.Offset)
	limitPlaceholder := fmt.Sprintf("$%d", len(args)-1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args))

	q := fmt.Sprintf(`
		SELECT %s
		FROM transactions t
		%s
		ORDER BY t.date DESC, t.transaction_number ASC
		LIMIT %s OFFSET %s
	`, transactionCols, where, limitPlaceholder, offsetPlaceholder)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var results []TransactionRow
	for rows.Next() {
		var r TransactionRow
		if err := scanTxFields(&r, rows.Scan); err != nil {
			return nil, fmt.Errorf("scan transaction row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func GetTransactionByID(ctx context.Context, pool *pgxpool.Pool, id int64) (*TransactionRow, error) {
	q := fmt.Sprintf("SELECT %s FROM transactions t WHERE t.id = $1", transactionCols)
	return scanTransactionRow(pool.QueryRow(ctx, q, id))
}

func UpdateTransactionCategory(ctx context.Context, pool *pgxpool.Pool, id int64, category *string) error {
	const q = `UPDATE transactions SET category = $1 WHERE id = $2`
	tag, err := pool.Exec(ctx, q, category, id)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
