//go:build go1.25

package statement

import (
	"testing"
	"time"
)

func TestMapToStatementRecord(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		ps := &ParsedStatement{
			PeriodFrom: "01/03/2024",
			IBAN:       "FR7630001007941234567890185",
		}
		uploadedAt := time.Date(2024, 3, 5, 10, 30, 0, 0, time.UTC)

		rec, err := MapToStatementRecord(ps, uploadedAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rec.Period != "2024-03" {
			t.Errorf("expected period 2024-03, got %s", rec.Period)
		}
		if rec.IBAN != ps.IBAN {
			t.Errorf("expected IBAN %s, got %s", ps.IBAN, rec.IBAN)
		}
		if !rec.UploadedAt.Equal(uploadedAt) {
			t.Errorf("expected uploadedAt %v, got %v", uploadedAt, rec.UploadedAt)
		}
	})

	t.Run("empty PeriodFrom returns error", func(t *testing.T) {
		t.Parallel()

		ps := &ParsedStatement{
			PeriodFrom: "",
			IBAN:       "FR7630001007941234567890185",
		}
		_, err := MapToStatementRecord(ps, time.Now())
		if err == nil {
			t.Fatal("expected error for empty PeriodFrom")
		}
	})

	t.Run("invalid date format returns error", func(t *testing.T) {
		t.Parallel()

		ps := &ParsedStatement{
			PeriodFrom: "not-a-date",
			IBAN:       "FR7630001007941234567890185",
		}
		_, err := MapToStatementRecord(ps, time.Now())
		if err == nil {
			t.Fatal("expected error for invalid date format")
		}
	})
}

func TestMapToTransactionRecords(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns empty slice", func(t *testing.T) {
		t.Parallel()

		records := MapToTransactionRecords(123, []ParsedTransaction{})
		if records == nil {
			t.Error("expected non-nil empty slice")
		}
		if len(records) != 0 {
			t.Errorf("expected 0 records, got %d", len(records))
		}
	})

	t.Run("two transactions", func(t *testing.T) {
		t.Parallel()

		statementID := int64(456)
		date1 := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
		date2 := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)

		parsedTxs := []ParsedTransaction{
			{
				Number:      1,
				Date:        date1,
				RawDate:     "01/03/2024",
				Type:        "ACHAT",
				Description: "foo   bar   baz", // extra spaces
				AmountCents: -1234,
				RawAmount:   "-12,34",
			},
			{
				Number:      2,
				Date:        date2,
				RawDate:     "02/03/2024",
				Type:        "RETRAIT DAB",
				Description: "  leading and trailing  ",
				AmountCents: 5000,
				RawAmount:   "50,00",
			},
		}

		records := MapToTransactionRecords(statementID, parsedTxs)
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}

		// Check first transaction
		if records[0].StatementID != statementID {
			t.Errorf("record[0]: expected StatementID %d, got %d", statementID, records[0].StatementID)
		}
		if records[0].TransactionNumber != 1 {
			t.Errorf("record[0]: expected TransactionNumber 1, got %d", records[0].TransactionNumber)
		}
		if !records[0].Date.Equal(date1) {
			t.Errorf("record[0]: date mismatch")
		}
		if records[0].Type != "ACHAT" {
			t.Errorf("record[0]: expected Type ACHAT, got %s", records[0].Type)
		}
		if records[0].Description != "foo bar baz" {
			t.Errorf("record[0]: expected normalized description, got %q", records[0].Description)
		}
		if records[0].AmountCents != -1234 {
			t.Errorf("record[0]: expected AmountCents -1234, got %d", records[0].AmountCents)
		}
		if records[0].Category != nil {
			t.Errorf("record[0]: expected Category nil, got %v", records[0].Category)
		}

		// Check second transaction
		if records[1].StatementID != statementID {
			t.Errorf("record[1]: expected StatementID %d, got %d", statementID, records[1].StatementID)
		}
		if records[1].TransactionNumber != 2 {
			t.Errorf("record[1]: expected TransactionNumber 2, got %d", records[1].TransactionNumber)
		}
		if !records[1].Date.Equal(date2) {
			t.Errorf("record[1]: date mismatch")
		}
		if records[1].Type != "RETRAIT DAB" {
			t.Errorf("record[1]: expected Type RETRAIT DAB, got %s", records[1].Type)
		}
		if records[1].Description != "leading and trailing" {
			t.Errorf("record[1]: expected trimmed description, got %q", records[1].Description)
		}
		if records[1].AmountCents != 5000 {
			t.Errorf("record[1]: expected AmountCents 5000, got %d", records[1].AmountCents)
		}
		if records[1].Category != nil {
			t.Errorf("record[1]: expected Category nil, got %v", records[1].Category)
		}
	})
}
