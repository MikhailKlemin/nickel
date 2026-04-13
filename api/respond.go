//go:build go1.25

package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(v); err != nil {
		// Log the error - can't change response after WriteHeader
		slog.Error("failed to encode JSON response", "err", err)
	}
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, errorBody{Code: code, Message: message})
}

// decodeJSON reads exactly one JSON value from the request body into v.
// It rejects unknown fields and trailing content after the first value,
// and returns a generic error message safe to forward to API consumers
// (internal field names are never leaked).
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errInvalidBody
	}
	// Reject trailing tokens: {"category":"Food"}{"extra":"junk"}
	if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
		return errInvalidBody
	}
	return nil
}

// errInvalidBody is the single user-facing error for any JSON decode problem.
// Using a package-level value avoids allocating a new error on every bad request.
var errInvalidBody = &staticError{"invalid request body"}

type staticError struct{ msg string }

func (e *staticError) Error() string { return e.msg }
