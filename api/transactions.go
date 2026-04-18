//go:build go1.25

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nickel/statement"
)

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	f, err := parseTransactionFilter(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.serveTransactionList(w, r, f)
}

func (s *Server) handlePatchCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid transaction id")
		return
	}

	// {"category": "Food"} sets it; {"category": null} clears it.
	var body struct {
		Category *string `json:"category"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	// Normalize whitespace-only strings to nil so the DB never stores "".
	if body.Category != nil {
		trimmed := strings.TrimSpace(*body.Category)
		if trimmed == "" {
			body.Category = nil
		} else {
			body.Category = &trimmed
		}
	}

	if err := statement.UpdateTransactionCategory(r.Context(), s.pool, id, body.Category); err != nil {
		if errors.Is(err, statement.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "transaction not found")
			return
		}
		s.logger.Error("update category", "id", id, "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update category")
		return
	}

	row, err := statement.GetTransactionByID(r.Context(), s.pool, id)
	if err != nil {
		s.logger.Error("fetch transaction after update", "id", id, "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "category updated but fetch failed")
		return
	}

	respondJSON(w, http.StatusOK, statement.TransactionRowToResponse(row))
}

// serveTransactionList is the shared pagination kernel used by both
// handleListTransactions and handleListStatementTransactions.
func (s *Server) serveTransactionList(w http.ResponseWriter, r *http.Request, f statement.TransactionFilter) {
	total, err := statement.CountTransactions(r.Context(), s.pool, f)
	if err != nil {
		s.logger.Error("count transactions", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to count transactions")
		return
	}

	rows, err := statement.ListTransactions(r.Context(), s.pool, f)
	if err != nil {
		s.logger.Error("list transactions", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list transactions")
		return
	}

	data := make([]statement.TransactionResponse, len(rows))
	for i, row := range rows {
		data[i] = statement.TransactionRowToResponse(&row)
	}

	respondJSON(w, http.StatusOK, statement.PagedTransactions{
		Data:   data,
		Total:  total,
		Limit:  f.Limit,
		Offset: f.Offset,
	})
}

// parseTransactionFilter reads the common query params into a TransactionFilter.
// Called by both transaction list endpoints.
func parseTransactionFilter(r *http.Request) (statement.TransactionFilter, error) {
	q := r.URL.Query()

	f := statement.TransactionFilter{
		Limit:  50,
		Offset: 0,
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, fmt.Errorf("limit must be a positive integer")
		}
		f.Limit = min(n, 200) // built-in min from Go 1.21+
	}

	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, fmt.Errorf("offset must be a non-negative integer")
		}
		f.Offset = n
	}

	if v := q.Get("type"); v != "" {
		f.Type = &v
	}

	if v := q.Get("date_from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return f, fmt.Errorf("date_from must be YYYY-MM-DD")
		}
		f.DateFrom = &t
	}

	if v := q.Get("date_to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return f, fmt.Errorf("date_to must be YYYY-MM-DD")
		}
		f.DateTo = &t
	}

	// Validate date range: date_from must not be after date_to
	if f.DateFrom != nil && f.DateTo != nil && f.DateFrom.After(*f.DateTo) {
		return f, fmt.Errorf("date_from must be before or equal to date_to")
	}

	if v := q.Get("category"); v != "" {
		// "uncategorized" is a sentinel → query.go maps it to IS NULL
		f.Category = &v
	}

	return f, nil
}
