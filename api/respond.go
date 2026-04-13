//go:build go1.25

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, errorBody{Code: code, Message: message})
}

// decodeJSON reads the request body into v.
// Returns a user-friendly error safe to send in a 400 response.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
