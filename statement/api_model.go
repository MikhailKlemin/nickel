//go:build go1.25

package statement

import "time"

// ---------------------------------------------------------------------------
// Statement
// ---------------------------------------------------------------------------

type StatementResponse struct {
	ID         int64  `json:"id"`
	Period     string `json:"period"`
	IBAN       string `json:"iban"`
	UploadedAt string `json:"uploaded_at"` // RFC 3339
	TxCount    int64  `json:"transaction_count"`
}

func StatementRowToResponse(r *StatementRow) StatementResponse {
	return StatementResponse{
		ID:         r.ID,
		Period:     r.Period,
		IBAN:       r.IBAN,
		UploadedAt: r.UploadedAt.UTC().Format(time.RFC3339),
		TxCount:    r.TxCount,
	}
}

// ---------------------------------------------------------------------------
// Transaction
// ---------------------------------------------------------------------------

type TransactionResponse struct {
	ID          int64   `json:"id"`
	StatementID int64   `json:"statement_id"`
	Number      int     `json:"number"`
	Date        string  `json:"date"` // "YYYY-MM-DD"
	Type        string  `json:"type"`
	Description string  `json:"description"`
	AmountCents int64   `json:"amount_cents"`
	AmountEur   string  `json:"amount_eur"` // e.g. "-12.34"
	Category    *string `json:"category"`   // null when unset
}

type PagedTransactions struct {
	Data   []TransactionResponse `json:"data"`
	Total  int64                 `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

func TransactionRowToResponse(r *TransactionRow) TransactionResponse {
	return TransactionResponse{
		ID:          r.ID,
		StatementID: r.StatementID,
		Number:      r.TransactionNumber,
		Date:        r.Date.UTC().Format("2006-01-02"),
		Type:        r.Type,
		Description: r.Description,
		AmountCents: r.AmountCents,
		AmountEur:   formatAmountEuro(r.AmountCents), // reuses func from parsed_model.go
		Category:    r.Category,
	}
}

// ---------------------------------------------------------------------------
// Analytics
// ---------------------------------------------------------------------------

type MonthlySummary struct {
	Period      string `json:"period"` // "YYYY-MM"
	DebitCents  int64  `json:"debit_cents"`
	CreditCents int64  `json:"credit_cents"`
	TxCount     int64  `json:"transaction_count"`
}

type AnalyticsSummary struct {
	Months     []MonthlySummary `json:"months"`
	ByType     map[string]int64 `json:"by_type"`
	ByCategory map[string]int64 `json:"by_category"`
}
