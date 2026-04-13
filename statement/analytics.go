//go:build go1.25

package statement

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// groupColumn is the set of transaction columns that can be used as a
// grouping key in queryGroupedTotals. Using a named type prevents
// arbitrary strings from being interpolated into SQL.
type groupColumn string

const (
	groupByType     groupColumn = "type"
	groupByCategory groupColumn = "category"
)

// GetAnalyticsSummary returns aggregated spend data.
// periodFrom and periodTo are inclusive bounds in "YYYY-MM" format.
// Either (or both) may be empty, which removes that bound from the query.
func GetAnalyticsSummary(ctx context.Context, pool *pgxpool.Pool, periodFrom, periodTo string) (*AnalyticsSummary, error) {
	months, err := queryMonthlyBreakdown(ctx, pool, periodFrom, periodTo)
	if err != nil {
		return nil, fmt.Errorf("monthly breakdown: %w", err)
	}

	byType, err := queryGroupedTotals(ctx, pool, groupByType, periodFrom, periodTo)
	if err != nil {
		return nil, fmt.Errorf("totals by type: %w", err)
	}

	byCategory, err := queryGroupedTotals(ctx, pool, groupByCategory, periodFrom, periodTo)
	if err != nil {
		return nil, fmt.Errorf("totals by category: %w", err)
	}

	return &AnalyticsSummary{
		Months:     months,
		ByType:     byType,
		ByCategory: byCategory,
	}, nil
}

// queryMonthlyBreakdown returns one MonthlySummary row per calendar month
// that has at least one transaction within the requested period bounds.
func queryMonthlyBreakdown(ctx context.Context, pool *pgxpool.Pool, periodFrom, periodTo string) ([]MonthlySummary, error) {
	where, args := periodWhere("TO_CHAR(t.date, 'YYYY-MM')", periodFrom, periodTo)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(t.date, 'YYYY-MM')                                              AS period,
			SUM(CASE WHEN t.amount_cents < 0 THEN ABS(t.amount_cents) ELSE 0 END)  AS debit_cents,
			SUM(CASE WHEN t.amount_cents > 0 THEN t.amount_cents        ELSE 0 END) AS credit_cents,
			COUNT(*)                                                                 AS tx_count
		FROM transactions t
		%s
		GROUP BY 1
		ORDER BY 1
	`, where)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var results []MonthlySummary
	for rows.Next() {
		var m MonthlySummary
		if err := rows.Scan(&m.Period, &m.DebitCents, &m.CreditCents, &m.TxCount); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// queryGroupedTotals returns a map of column-value → total debit spend in cents.
// Only negative amounts (debits) are summed so that credits and reimbursements
// do not inflate the totals.
// col must be one of the declared groupColumn constants — the type prevents
// arbitrary SQL interpolation at compile time.
func queryGroupedTotals(ctx context.Context, pool *pgxpool.Pool, col groupColumn, periodFrom, periodTo string) (map[string]int64, error) {
	// t.amount_cents < 0 is always the base condition. Period bounds, if
	// present, are appended as additional AND clauses via periodWhere.
	periodClause, args := periodWhere("TO_CHAR(t.date, 'YYYY-MM')", periodFrom, periodTo)

	var where string
	if periodClause == "" {
		where = "WHERE t.amount_cents < 0"
	} else {
		// periodClause is "WHERE <cond> [AND <cond>]"; strip the keyword and
		// append to our own WHERE so there is exactly one WHERE in the query.
		where = "WHERE t.amount_cents < 0 AND " + strings.TrimPrefix(periodClause, "WHERE ")
	}

	// COALESCE maps NULL categories to the sentinel "uncategorized"
	// so the JSON map never has a null key.
	query := fmt.Sprintf(`
		SELECT
			COALESCE(t.%s::text, 'uncategorized') AS label,
			SUM(ABS(t.amount_cents))               AS total_cents
		FROM transactions t
		%s
		GROUP BY 1
		ORDER BY 2 DESC
	`, col, where)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var label string
		var total int64
		if err := rows.Scan(&label, &total); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result[label] = total
	}
	return result, rows.Err()
}

// periodWhere builds a WHERE clause (or empty string) and a pgx args slice
// for filtering by period bounds. expr is the SQL expression that produces
// a "YYYY-MM" string — e.g. TO_CHAR(t.date, 'YYYY-MM').
//
// Returned placeholder indices start at $1 and increment for each bound
// present, so the caller can safely append additional args if needed.
func periodWhere(expr, from, to string) (clause string, args []any) {
	var conditions []string

	if from != "" {
		args = append(args, from)
		conditions = append(conditions, fmt.Sprintf("%s >= $%d", expr, len(args)))
	}
	if to != "" {
		args = append(args, to)
		conditions = append(conditions, fmt.Sprintf("%s <= $%d", expr, len(args)))
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}
