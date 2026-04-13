//go:build go1.25

package api

import (
	"fmt"
	"net/http"
	"regexp"

	"nickel/statement"
)

// rePeriod matches the strict YYYY-MM format expected by the analytics queries.
var rePeriod = regexp.MustCompile(`^\d{4}-(?:0[1-9]|1[0-2])$`)

func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	periodFrom := q.Get("period_from")
	periodTo := q.Get("period_to")

	if err := validatePeriodParams(periodFrom, periodTo); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	summary, err := statement.GetAnalyticsSummary(r.Context(), s.pool, periodFrom, periodTo)
	if err != nil {
		s.logger.Error("analytics summary", "err", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute analytics")
		return
	}

	respondJSON(w, http.StatusOK, summary)
}

// validatePeriodParams checks that non-empty period bounds are well-formed
// YYYY-MM strings and that from does not come after to.
func validatePeriodParams(from, to string) error {
	if from != "" && !rePeriod.MatchString(from) {
		return fmt.Errorf("period_from must be in YYYY-MM format")
	}
	if to != "" && !rePeriod.MatchString(to) {
		return fmt.Errorf("period_to must be in YYYY-MM format")
	}
	if from != "" && to != "" && from > to {
		return fmt.Errorf("period_from must not be after period_to")
	}
	return nil
}
