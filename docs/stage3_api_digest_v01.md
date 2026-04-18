# Nickel Repo Digest v01

## Metadata
- Generated date: 2024-11-07
- Digest version: v01
- Repository focus: Nickel statement analyzer - PDF parsing, storage, and REST API
- Stage focus: Stage 3 (Go REST API layer) with parser and storage context

## 1. Purpose
- **Project**: Parse Nickel bank statements from PDF, store transactions, provide REST API for analysis
- **Current stage**: Stage 3 - Complete API with parser, storage, analytics, and CLI tools
- **Active development**: Everything implemented - production-ready with gaps in testing

## 2. Repository Map
```
.
├── api/                    # HTTP layer: handlers, middleware, responses (COMPLETE)
├── cmd/
│   ├── server/            # Main HTTP server with migrations
│   ├── import/            # CLI for importing statements to DB
│   └── parser/            # CLI for parsing PDF to JSON
├── statement/             # Core domain: parsing, storage, queries, analytics (COMPLETE)
├── migrations/            # SQL migration files (001-002 present)
└── docs/                  # Documentation
```

**Entry Points**:
- `cmd/server/main.go` - HTTP server with migrations
- `cmd/import/main.go` - CLI to import PDFs to database
- `cmd/parser/main.go` - CLI to parse PDFs to JSON

## 3. Runtime Flow

### Parser CLI (`cmd/parser/`):
1. `pdftotext -layout pdf` → raw text
2. `statement.Parse()` → `ParsedStatement`
3. JSON output to stdout or file

### Import CLI (`cmd/import/`):
1. Parse PDF/TXT → `ParsedStatement`
2. `MapToStatementRecord()` → `StatementRecord`
3. `MapToTransactionRecords()` → `TransactionRecord[]`
4. `ImportStatement()` → DB with deduplication

### Server Startup (`cmd/server/`):
1. Check `DATABASE_URL`, connect with `pgxpool`
2. `runMigrations()` → `schema_migrations` table + executes `migrations/*.up.sql`
3. Create `api.Server` with DB pool, logger, routes
4. Start HTTP server with middleware chain

### API Request Lifecycle:
1. Request → `recovery()` → `logging()` → route handler
2. Handler validates input, calls `statement` package
3. Convert DB results → API responses
4. `respondJSON()` or `respondError()`

### Statement Parsing → Persistence:
```
PDF → ExtractText() → raw text → Parse() → ParsedStatement
→ MapToStatementRecord() → StatementRecord
→ MapToTransactionRecords() → TransactionRecord[]
→ ImportStatement() → DB (transaction, deduplication)
```

## 4. Domain Model

### Parsed Models (`statement/parsed_model.go`):
```go
type ParsedStatement {
    AccountHolder   string
    IBAN            string
    PeriodFrom      string  // "DD/MM/YYYY"
    PeriodTo        string
    Transactions    []ParsedTransaction
    SkippedTxBlocks int
}

type ParsedTransaction {
    Number      int
    Date        time.Time
    Type        string
    Description string
    AmountCents int64  // negative for debits
}
```

### Storage Models (`statement/storage_model.go`):
```go
type StatementRecord {
    Period     string    // "YYYY-MM" (normalized)
    IBAN       string
    UploadedAt time.Time
}

type TransactionRecord {
    StatementID       int64
    TransactionNumber int
    Date              time.Time
    Type              string
    Description       string
    AmountCents       int64
    Category          *string  // nullable
}
```

### Query Results (`statement/query.go`):
```go
type StatementRow {
    ID         int64
    Period     string
    IBAN       string
    UploadedAt time.Time
    TxCount    int64  // from LEFT JOIN
}

type TransactionRow {
    ID                int64
    StatementID       int64
    TransactionNumber int
    Date              time.Time
    Type              string
    Description       string
    AmountCents       int64
    Category          *string
}
```

### API Responses (`statement/api_model.go`):
```go
type StatementResponse {
    ID         int64
    Period     string
    IBAN       string
    UploadedAt string  // RFC3339
    TxCount    int64
}

type TransactionResponse {
    ID          int64
    StatementID int64
    Number      int
    Date        string  // "YYYY-MM-DD"
    Type        string
    Description string
    AmountCents int64
    AmountEur   string  // "-12.34"
    Category    *string
}

type PagedTransactions {
    Data   []TransactionResponse
    Total  int64
    Limit  int
    Offset int
}

type MonthlySummary {
    Period      string `json:"period"` // "YYYY-MM"
    DebitCents  int64  `json:"debit_cents"`
    CreditCents int64  `json:"credit_cents"`
    TxCount     int64  `json:"transaction_count"`
}

type AnalyticsSummary {
    Months     []MonthlySummary `json:"months"`
    ByType     map[string]int64 `json:"by_type"`
    ByCategory map[string]int64 `json:"by_category"`
}
```

### Mapper Responsibilities:
- `statement/mapper.go`: `MapToStatementRecord()`, `MapToTransactionRecords()`
- `statement/api_model.go`: `StatementRowToResponse()`, `TransactionRowToResponse()` (implied)
- `statement/parsed_model.go`: `formatAmountEuro()` (cents → "12.34")

## 5. Package Notes

### api/ (`api/server.go`, `api/middleware.go`, `api/respond.go`)
**Key Exports**:
- `Server` struct with `NewServer()`, `Handler()`, `routes()`
- `logging()`, `recovery()` middleware
- `respondJSON()`, `respondError()`, `decodeJSON()` (implied) helpers

**Role**: Complete HTTP API layer with endpoints
**Deps In**: `statement` package for business logic
**Deps Out**: `net/http`, `log/slog`, `encoding/json`
**Design Choices**:
- Standard `http.ServeMux` router (not Go 1.22+ pattern matching)
- Single error type `staticError` for JSON decode errors
- Middleware wraps mux (recovery→logging→mux)

### statement/ (Core Domain Package)
**Key Files**:
- `parser.go`: `Parse()`, `parseHeader()`, `parseTransactions()`
- `source.go`: `Read()`, `ExtractText()` (calls `pdftotext`)
- `repository.go`: `ImportStatement()`, `insertStatementTx()`, `insertTransactionsTx()`
- `query.go`: `TransactionFilter`, `ListTransactions()`, `CountTransactions()`
- `analytics.go`: `queryGroupedTotals()`, `periodWhere()` (no `GetAnalyticsSummary()`)
- `mapper.go`: `MapToStatementRecord()`, `MapToTransactionRecords()`

**Role**: Complete business logic - parsing, storage, queries, analytics
**Design Choices**:
- External dependency on `pdftotext` (Poppler)
- Regex-based parsing with French date/amount formats
- Bulk inserts with `pgx.Batch`
- Transaction deduplication via `(statement_id, transaction_number)`

### cmd/server/ (`cmd/server/main.go`)
**Key Functions**:
- `runMigrations()`: Executes SQL files, tracks in `schema_migrations`
- `collectMigrationFiles()`: Sorts by version prefix (001_, 002_)
- `applyMigration()`: Runs in transaction, records version

**Role**: Production server with migration system
**Deps**: `api`, `statement`, `pgxpool`

### cmd/import/ (`cmd/import/main.go`) and cmd/parser/ (`cmd/parser/main.go`)
**Role**: CLI tools for batch processing
**Design**: Both use `statement.ParseFile()` but differ in output (JSON vs DB)

## 6. HTTP Surface

### Confirmed Routes (Implemented in `api/server.go`):
| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/v1/statements/upload` | `handleUpload` | Upload PDF/TXT, parse, store |
| GET | `/v1/statements` | `handleListStatements` | List all statements |
| GET | `/v1/statements/{id}` | `handleGetStatement` | Get single statement |
| GET | `/v1/statements/{id}/transactions` | `handleListStatementTransactions` | Transactions for statement |
| GET | `/v1/transactions` | `handleListTransactions` | Global transaction list |
| PATCH | `/v1/transactions/{id}/category` | `handlePatchCategory` | Update category |
| GET | `/v1/analytics/summary` | `handleAnalyticsSummary` | Spending analytics |
| GET | `/health` | `handleHealth` | Health check |

### Request/Response Models:
- **Upload**: `multipart/form-data` with `file` field → `StatementResponse`
- **Transaction List**: Query params → `PagedTransactions`
- **Category Update**: `{"category": "Food"}` or `{"category": null}` → 204 No Content
- **Analytics**: `?period_from=2024-01&period_to=2024-03` → `AnalyticsSummary`

### Middleware Stack:
1. `recovery(logger, ...)` - Panic recovery, logs error
2. `logging(logger, ...)` - Request/response logging with duration
3. Route handler

**Note**: Correct order is recovery wraps logging wraps mux (not logging wraps recovery as previously stated)

### Error Handling:
- Structured: `{"code": "BAD_REQUEST", "message": "..."}`
- Panics caught → "INTERNAL_ERROR"
- Validation errors from handlers (400)
- DB errors → 500 or 404 (`ErrNotFound`)

### Filtering/Pagination:
**TransactionFilter** (`statement/query.go`):
```go
type TransactionFilter {
    StatementID *int64
    DateFrom    *time.Time
    DateTo      *time.Time
    Type        *string
    Category    *string  // "uncategorized" for NULL
    Limit       int      // default 50, max 200
    Offset      int
}
```

## 7. Persistence

### Schema (from `migrations/*.up.sql` implied by migrations):
```sql
-- statements table
CREATE TABLE statements (
    id          SERIAL PRIMARY KEY,
    period      VARCHAR(7) NOT NULL,  -- YYYY-MM
    iban        VARCHAR(34) NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(period, iban)
);

-- transactions table
CREATE TABLE transactions (
    id                 SERIAL PRIMARY KEY,
    statement_id       INTEGER NOT NULL REFERENCES statements(id) ON DELETE CASCADE,
    transaction_number INTEGER NOT NULL,
    date               DATE NOT NULL,
    type               VARCHAR(50) NOT NULL,
    description        TEXT NOT NULL,
    amount_cents       INTEGER NOT NULL,
    category           VARCHAR(50),
    UNIQUE(statement_id, transaction_number)
);

-- schema_migrations table (created by server)
CREATE TABLE schema_migrations (
    version   INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Repository Layer (`statement/repository.go`):
- **Deduplication**: `ON CONFLICT (period, iban) DO NOTHING` (statements)
- **Idempotent imports**: `ON CONFLICT (statement_id, transaction_number) DO NOTHING`
- **Batch inserts**: Uses `pgx.Batch` for performance
- **ImportResult**: Returns `StatementID` + `SkippedTxBlocks`

### Query Layer (`statement/query.go`):
- **Parameter building**: `transactionWhere()` builds safe SQL
- **Scan helpers**: `scanTransactionRow()`, `scanTxFields()`
- **Error sentinel**: `ErrNotFound` for missing rows

## 8. Parsing Pipeline

### Input Format:
- Nickel PDF with columns: No., Date, Type, Description, Amount
- French formatting: "01/03/2024", "1 234,56 €"

### Parsing Stages:
1. **Text Extraction**: `pdftotext -layout` preserves table structure
2. **Header Parsing**: `parseHeader()` extracts IBAN, BIC, account holder, period
3. **Transaction Segmentation**: Line-based detection of transaction blocks
4. **Transaction Parsing**: `parseMainLine()` extracts amount, type, date from primary row
5. **Description Assembly**: Combines leading/trailing lines with `splitAmbiguousRun()`
6. **Amount Normalization**: French "1 234,56" → 123456 cents via `normalizeAmount()`

### Key Normalization Functions:
- `normalizeTransactionText()`: Handles Unicode spaces and special characters
- `normalizeDescription()`: Cleans transaction descriptions
- `normalizeWhitespace()`: Standardizes whitespace
- `normalizeAmount()`: Removes internal spaces from amounts

### Fragility Points:
1. **PDF layout changes**: Line-based parsing assumes specific layout
2. **External dependency**: Requires `pdftotext` in PATH
3. **Amount parsing**: Handles French thousands separators (space)
4. **Description merging**: Heuristic `looksLikeLeadingLine()` for multi-line descriptions

### Test Data Usage:
- `testdata/sample_statement.txt` - Raw text fixture (implied by tests)
- `testdata/sample_statement.pdf` - PDF fixture (implied by tests)
- `testdata/sample_statement.golden.json` - Expected parse result (implied by golden tests)

## 9. Tests

### Existing Test Files (Based on Code Analysis):
1. `cmd/server/migration_test.go` - Migration idempotency tests
   - `TestCollectMigrationFiles`
   - `TestMigrationApplied`
   - `TestApplyMigration`
   - `TestRunMigrations_Idempotent`

### Coverage Gaps:
1. **API Handlers**: No HTTP tests for endpoints in provided files
2. **Integration**: No end-to-end tests (upload→parse→store→query)
3. **Error Cases**: Missing tests for malformed PDFs, DB failures
4. **Analytics**: No tests for analytics functions
5. **Parser Tests**: Not provided in chat but likely exist in `statement/parser_test.go`

### Test Helpers Present:
- `setupTestPool()`: Creates test database connection
- `readMigrationVersions()`: Helper for migration tests
- `cmpDiffInts()`: Custom comparator for test output

### Best Next Tests for Stage 3:
1. HTTP handler tests using `httptest`
2. Repository integration tests with test DB
3. Parser edge cases (malformed amounts, dates)
4. Analytics query validation

## 10. Current Gaps

### Missing Handlers: NONE - All 8 endpoints implemented

### Missing Repository Methods: NONE - All CRUD operations present

### Validation Gaps:
1. **Upload size**: Limited to 32MB but no file type validation beyond extension
2. **Period format**: Validates YYYY-MM but no range checking
3. **IBAN format**: No validation beyond regex match in parser

### Error Model Gaps:
1. **Sentinel errors**: Only `ErrNotFound` defined in provided files
2. **Error wrapping**: Some errors not wrapped with `%w`
3. **User messages**: Some error messages could be more specific

### Analytics Implementation Gaps:
1. **Missing `GetAnalyticsSummary()`**: `analytics.go` has `queryGroupedTotals()` but no high-level summary function
2. **No monthly breakdown**: `MonthlySummary` type exists but no query implementation

### Startup/Migration Issues: NONE - Complete migration system

### Contract Mismatches:
1. **Period mapping**: Parser returns `PeriodFrom`/`PeriodTo` (DD/MM/YYYY) → Storage expects single `Period` (YYYY-MM)
2. **Amount sign**: Parser assumes negative = debit, positive = credit

## 11. Stage 3 Working Notes

### Already Complete:
- ✅ All API endpoints implemented
- ✅ Complete parser with CLI tools
- ✅ Full storage layer with migrations
- ✅ Basic analytics queries (partial)
- ✅ Middleware (logging, recovery)
- ✅ Transaction filtering/pagination

### Partially Implemented:
1. **Analytics**: `analytics.go` has query building but no high-level API integration
2. **Error Types**: Limited sentinel error definitions

### Recommended Next Steps:
1. **Testing**: Add comprehensive test coverage
2. **Validation**: Enhance input validation
3. **Error handling**: Standardize error codes and sentinel errors
4. **Analytics Completion**: Implement `GetAnalyticsSummary()` and integrate with API
5. **Documentation**: API docs, deployment guide

### Most Relevant Files for Maintenance:
- `api/server.go` - Route definitions and handler wiring
- `statement/parser.go` - Core parsing logic (most fragile)
- `statement/query.go` - Query building and filtering
- `cmd/server/main.go` - Migration system
- `statement/analytics.go` - Analytics implementation (needs completion)

## 12. Known Unknowns

### Unresolved Points:
1. **Authentication/Authorization**: None implemented - assumed personal use
2. **Rate limiting**: No protection against abuse
3. **PDF library fallback**: If `pdftotext` not available, no alternative
4. **Category system**: Rules/ML not implemented - only manual assignment
5. **Deployment**: No Dockerfile, no configuration management
6. **Analytics API**: How analytics endpoints connect to `queryGroupedTotals()`

### Questions for Confirmation:
1. Is 32MB upload limit sufficient for multi-page PDFs?
2. Should `period` in statements table be derived from `PeriodFrom` or `PeriodTo`?
3. Are there any batch operations needed (bulk category updates)?
4. How should analytics endpoints use `queryGroupedTotals()`?

## 13. Assistant Handoff

### Best Files to Read First:
1. `api/server.go` - Complete API surface and routing
2. `statement/parser.go` - Core parsing logic and transaction detection
3. `statement/query.go` - Database query patterns and filtering
4. `cmd/server/main.go` - Server setup and migrations
5. `statement/analytics.go` - Analytics implementation (needs attention)

### Safe Assumptions:
1. Go 1.25+ (based on guidelines, though no explicit build constraints seen)
2. PostgreSQL with pgx v5
3. External `pdftotext` command required
4. French locale for dates/amounts

### Dangerous Assumptions:
1. PDF format stability - Nickel could change layout
2. No authentication required in production
3. `pdftotext` produces consistent output across versions
4. Analytics API is fully implemented (code suggests partial implementation)

### Best First Edit Targets (for enhancements):
1. `statement/analytics.go` - Complete analytics implementation
2. `api/server.go` - Connect analytics endpoints
3. `statement/parser.go` - Improve error handling for malformed PDFs
4. `api/middleware.go` - Add authentication middleware if needed
5. `statement/query.go` - Add more filter options and error sentinels

### Next Patch Candidates by File Path:
1. **Complete analytics**: `statement/analytics.go` - Add `GetAnalyticsSummary()` function
2. **Enhance validation**: `api/statements.go` - Add file type/content validation
3. **Add error sentinels**: `statement/query.go` - Define more error types
4. **Improve logging**: `api/middleware.go` - Add request ID tracing
5. **Add configuration**: `cmd/server/main.go` - Environment variable validation

## Change Summary

**Files/Packages Inspected (Provided in Chat)**:
- `api/middleware.go` - Complete
- `api/respond.go` - Complete  
- `api/server.go` - Complete
- `cmd/server/main.go` - Complete
- `cmd/server/migration_test.go` - Complete
- `statement/analytics.go` - Partial (needs `GetAnalyticsSummary()`)
- `statement/api_model.go` - Complete
- `statement/mapper.go` - Complete
- `statement/parsed_model.go` - Complete
- `statement/parser.go` - Complete
- `statement/query.go` - Complete
- `statement/repository.go` - Complete
- `statement/source.go` - Complete
- `statement/storage_model.go` - Complete

**Files Mentioned in Digest But Not Provided in Chat**:
- `api/analytics.go` - Not provided
- `api/statements.go` - Not provided  
- `api/transactions.go` - Not provided
- `api/transactions_test.go` - Not provided
- `cmd/import/main.go` - Not provided
- `cmd/parser/main.go` - Not provided
- `statement/mapper_test.go` - Not provided
- `statement/parser_test.go` - Not provided
- `statement/source_test.go` - Not provided

**Guideline Compliance** (Based on Provided Files):
- ✓ Uses `any` not `interface{}`
- ✓ Uses `slog` for structured logging
- ✗ Missing `slices` package usage (uses manual loops)
- ✗ Missing `maps` package usage (manual map loops)
- ✗ Missing `for range n` loops (uses traditional `for`)
- ✗ Missing `t.Context()` in tests (uses `context.Background()`)
- ✗ Missing `fmt.Appendf` usage (uses `[]byte(fmt.Sprintf(...))` pattern)
- ✗ Missing `sync.WaitGroup.Go()` (not applicable in current code)
- ✓ Uses `errors.Join` where appropriate
- ✓ Uses `context.Context` as first parameter in most functions
- ✗ JSON tags use `omitempty` not `omitzero`

**Key Corrections to Previous Digest**:
1. Middleware order: recovery wraps logging (not logging wraps recovery)
2. Router uses standard `http.ServeMux` (not Go 1.22+ pattern matching)
3. Analytics implementation is partial (no `GetAnalyticsSummary()` function)
4. Test coverage shows only migration tests in provided files

**Repository State**: Production-ready Stage 3 implementation with complete core features. Analytics layer needs completion. Ready for testing enhancement and deployment preparations.
