//go:build go1.25

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nickel/statement"
)

func TestParseTransactionFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       string
		wantErr     bool
		errContains string
		checkFilter func(*testing.T, statement.TransactionFilter)
	}{
		{
			name:    "empty query returns defaults",
			query:   "",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Limit != 50 {
					t.Errorf("expected default limit 50, got %d", f.Limit)
				}
				if f.Offset != 0 {
					t.Errorf("expected default offset 0, got %d", f.Offset)
				}
				if f.DateFrom != nil || f.DateTo != nil || f.Type != nil || f.Category != nil {
					t.Errorf("expected nil filters, got %+v", f)
				}
			},
		},
		{
			name:    "valid limit and offset",
			query:   "limit=25&offset=100",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Limit != 25 {
					t.Errorf("expected limit 25, got %d", f.Limit)
				}
				if f.Offset != 100 {
					t.Errorf("expected offset 100, got %d", f.Offset)
				}
			},
		},
		{
			name:    "limit caps at 200",
			query:   "limit=500",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Limit != 200 {
					t.Errorf("expected limit capped at 200, got %d", f.Limit)
				}
			},
		},
		{
			name:        "invalid limit returns error",
			query:       "limit=invalid",
			wantErr:     true,
			errContains: "limit must be a positive integer",
		},
		{
			name:        "negative offset returns error",
			query:       "offset=-1",
			wantErr:     true,
			errContains: "offset must be a non-negative integer",
		},
		{
			name:    "valid date range",
			query:   "date_from=2024-01-01&date_to=2024-01-31",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.DateFrom == nil || f.DateTo == nil {
					t.Fatal("expected both dates to be set")
				}
				expectedFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				expectedTo := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
				if !f.DateFrom.Equal(expectedFrom) {
					t.Errorf("expected date_from %v, got %v", expectedFrom, f.DateFrom)
				}
				if !f.DateTo.Equal(expectedTo) {
					t.Errorf("expected date_to %v, got %v", expectedTo, f.DateTo)
				}
			},
		},
		{
			name:        "invalid date format returns error",
			query:       "date_from=01-01-2024",
			wantErr:     true,
			errContains: "date_from must be YYYY-MM-DD",
		},
		{
			name:        "date_from after date_to returns error",
			query:       "date_from=2024-02-01&date_to=2024-01-31",
			wantErr:     true,
			errContains: "date_from must be before or equal to date_to",
		},
		{
			name:        "equal dates are allowed",
			query:       "date_from=2024-01-15&date_to=2024-01-15",
			wantErr:     false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.DateFrom == nil || f.DateTo == nil {
					t.Fatal("expected both dates to be set")
				}
				if !f.DateFrom.Equal(*f.DateTo) {
					t.Errorf("expected equal dates, got from=%v to=%v", f.DateFrom, f.DateTo)
				}
			},
		},
		{
			name:    "type filter",
			query:   "type=ACHAT",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Type == nil || *f.Type != "ACHAT" {
					t.Errorf("expected type ACHAT, got %v", f.Type)
				}
			},
		},
		{
			name:    "category filter",
			query:   "category=Food",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Category == nil || *f.Category != "Food" {
					t.Errorf("expected category Food, got %v", f.Category)
				}
			},
		},
		{
			name:    "uncategorized sentinel",
			query:   "category=uncategorized",
			wantErr: false,
			checkFilter: func(t *testing.T, f statement.TransactionFilter) {
				if f.Category == nil || *f.Category != "uncategorized" {
					t.Errorf("expected category uncategorized, got %v", f.Category)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/v1/transactions?"+tt.query, nil)
			filter, err := parseTransactionFilter(req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkFilter != nil {
				tt.checkFilter(t, filter)
			}
		})
	}
}

// contains checks if string s contains substring substr.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr))))
}
