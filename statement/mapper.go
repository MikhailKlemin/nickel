//go:build go1.25

package statement

import (
	"fmt"
	"time"
)

// MapToStatementRecord converts a ParsedStatement to a StatementRecord for storage.
// It normalizes the statement period to "YYYY-MM" format using the PeriodFrom field.
func MapToStatementRecord(ps *ParsedStatement, uploadedAt time.Time) (*StatementRecord, error) {
	if ps.PeriodFrom == "" {
		return nil, fmt.Errorf("cannot determine period: PeriodFrom is empty")
	}

	// Parse the date in "DD/MM/YYYY" format to extract year and month
	date, err := time.Parse("02/01/2006", ps.PeriodFrom)
	if err != nil {
		return nil, fmt.Errorf("invalid period format %q: %w", ps.PeriodFrom, err)
	}

	period := date.Format("2006-01") // "YYYY-MM"

	return &StatementRecord{
		Period:     period,
		IBAN:       ps.IBAN,
		UploadedAt: uploadedAt,
	}, nil
}

// MapToTransactionRecords converts a slice of ParsedTransaction to TransactionRecord
// for storage, associating them with the given statementID.
func MapToTransactionRecords(statementID int64, transactions []ParsedTransaction) []TransactionRecord {
	records := make([]TransactionRecord, 0, len(transactions))
	for _, pt := range transactions {
		records = append(records, TransactionRecord{
			StatementID:       statementID,
			TransactionNumber: pt.Number,
			Date:              pt.Date,
			Type:              pt.Type,
			Description:       pt.Description,
			AmountCents:       pt.AmountCents,
			Category:          nil,
		})
	}
	return records
}
