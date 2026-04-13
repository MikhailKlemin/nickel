//go:build go1.25

package api

import (
	"net/http"

	"nickel/statement"
)

func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Both params are optional; empty string means "no bound".
	periodFrom := q.Get("period_from") // e.g. "2024-01"
	periodTo := q.Get("period_to")     // e.g. "2024-06"

	summary, err := statement.GetAnalyticsSummary(r.Context(), s.pool, periodFrom, periodTo)
	if err != nil {
		s.logger.Error("analytics summary", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute analytics")
		return
	}

	respondJSON(w, http.StatusOK, summary)
}
