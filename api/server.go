//go:build go1.25

package api

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds the shared dependencies for all HTTP handlers.
// The mux is built once in NewServer and never mutated afterwards,
// so Server is safe to use from multiple goroutines.
type Server struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	mux    *http.ServeMux
}

// NewServer constructs a Server, registers all routes, and returns it.
// Call Handler() to obtain the http.Handler to pass to http.ListenAndServe.
func NewServer(pool *pgxpool.Pool, logger *slog.Logger) *Server {
	s := &Server{
		pool:   pool,
		logger: logger,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

// routes registers every endpoint on s.mux.
//
// Pattern syntax (Go 1.22+): "METHOD /path"
//   - The method prefix makes the mux reject wrong methods with 405
//     automatically — no manual method checks needed in handlers.
//   - {id} is a named wildcard: r.PathValue("id") retrieves it.
//   - A trailing slash pattern like "/v1/statements/" would act as a
//     catch-all subtree; we deliberately omit it so unknown sub-paths
//     get a 404 instead of falling through to the wrong handler.
//
// All paths share the /v1 prefix so a future /v2 can coexist without
// touching existing routes.
func (s *Server) routes() {
	// --- Statements ---

	// Upload a PDF or TXT file; parse, store and return the new statement.
	s.mux.HandleFunc("POST /v1/statements/upload", s.handleUpload)

	// List every statement (lightweight, no transactions).
	s.mux.HandleFunc("GET /v1/statements", s.handleListStatements)

	// Fetch a single statement by its database ID.
	// NOTE: this pattern must be registered AFTER the more-specific
	// "/v1/statements/upload" above.  Go 1.22 mux picks the longest
	// matching literal prefix, so "upload" wins over "{id}" — but
	// registering the specific route first makes the intent explicit.
	s.mux.HandleFunc("GET /v1/statements/{id}", s.handleGetStatement)

	// List transactions that belong to one statement.
	// Kept separate from GET /v1/transactions so callers can paginate
	// a single statement without a statement_id query param.
	s.mux.HandleFunc("GET /v1/statements/{id}/transactions", s.handleListStatementTransactions)

	// --- Transactions ---

	// Global transaction list with optional filters:
	//   ?limit=50&offset=0&type=ACHAT&date_from=2024-01-01&date_to=2024-03-31&category=Food
	// An empty category param returns all; the string "uncategorized"
	// filters to IS NULL rows (handled inside the handler).
	s.mux.HandleFunc("GET /v1/transactions", s.handleListTransactions)

	// Assign or clear the category of one transaction.
	// Body: {"category": "Food"} or {"category": null} to clear.
	s.mux.HandleFunc("PATCH /v1/transactions/{id}/category", s.handlePatchCategory)

	// --- Analytics ---

	// Aggregated spend summary.
	// Optional query params: ?period_from=2024-01&period_to=2024-03
	s.mux.HandleFunc("GET /v1/analytics/summary", s.handleAnalyticsSummary)
}

// limitBodySize middleware limits the request body size for POST, PUT, PATCH requests
func limitBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only limit body size for methods that typically have bodies
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Handler wraps the internal mux with middleware and returns the final
// http.Handler that the server entrypoint passes to http.ListenAndServe.
//
// Middleware is applied outside-in: a request passes through recovery
// first, then logging, then reaches the mux.  This means the logger
// always captures the final status code even when a panic is recovered.
//
//	request → recovery → logging → limitBodySize → mux → handler
func (s *Server) Handler() http.Handler {
	return recovery(s.logger,
		logging(s.logger,
			limitBodySize(10*1024*1024)( // 10MB limit
				s.mux,
			),
		),
	)
}
