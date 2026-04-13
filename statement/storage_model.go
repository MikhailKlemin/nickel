//go:build go1.25

package statement

import (
	"time"
)

type StatementRecord struct {
	Period     string // normalized period in "YYYY-MM" format
	IBAN       string
	UploadedAt time.Time
}

type TransactionRecord struct {
	StatementID       int64
	TransactionNumber int
	Date              time.Time
	Type              string
	Description       string
	AmountCents       int64
	Category          *string // nullable category
}
