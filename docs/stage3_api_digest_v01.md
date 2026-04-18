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

type AnalyticsSummary {
    Months     []MonthlySummary
    ByType     map[string]int64
    ByCategory map[string]int64
}
```

### Mapper Responsibilities:
- `statement/mapper.go`: `MapToStatementRecord()`, `MapToTransactionRecords()`
- `statement/api_model.go`: `StatementRowToResponse()`, `TransactionRowToResponse()`
- `statement/parsed_model.go`: `formatAmountEuro()` (cents → "12.34")

## 5. Package Notes

### api/ (`api/server.go`, `api/middleware.go`, `api/respond.go`)
**Key Exports**:
- `Server` struct with `NewServer()`, `Handler()`, `routes()`
- `logging()`, `recovery()` middleware
- `respondJSON()`, `respondError()`, `decodeJSON()` helpers

**Role**: Complete HTTP API layer with 8 endpoints
**Deps In**: `statement` package for business logic
**Deps Out**: `net/http`, `log/slog`, `encoding/json`
**Design Choices**:
- Go 1.22+ pattern-matching router ("METHOD /path")
- Single error type `staticError` for JSON decode errors
- Middleware wraps mux (recovery→logging→mux)

### statement/ (Core Domain Package)
**Key Files**:
- `parser.go`: `Parse()`, `SplitTransactions()`, `ParseTransaction()`
- `source.go`: `Read()`, `ExtractText()` (calls `pdftotext`)
- `repository.go`: `ImportStatement()`, `insertStatementTx()`, `insertTransactionsTx()`
- `query.go`: `TransactionFilter`, `ListTransactions()`, `CountTransactions()`
- `analytics.go`: `GetAnalyticsSummary()`, `queryMonthlyBreakdown()`
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

### Confirmed Routes (All Implemented):
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

### Schema (from `migrations/*.up.sql`):
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
2. **Header Parsing**: Regex for IBAN, BIC, account holder, period
3. **Transaction Segmentation**: `SplitTransactions()` finds table rows
4. **Main Line Parsing**: `parseMainLine()` extracts amount, type, date
5. **Description Assembly**: Combines leading/trailing lines
6. **Amount Normalization**: French "1 234,56" → 123456 cents

### Key Regex Patterns:
- `rePeriod`: `Du\s+(\d{2}/\d{2}/\d{4})\s+au\s+(\d{2}/\d{2}/\d{4})`
- `reAmount`: `\s+(-?\d[\d\s]*,\d{2}\s*€?)\s*$`
- `txStart`: Transaction row detection

### Fragility Points:
1. **PDF layout changes**: Regexes assume specific column positions
2. **External dependency**: Requires `pdftotext` in PATH
3. **Amount parsing**: Handles French thousands separators (space)
4. **Description merging**: Heuristic `looksLikeLeadingLine()`

### Test Data Usage:
- `testdata/sample_statement.txt` - Raw text fixture
- `testdata/sample_statement.pdf` - PDF fixture
- `testdata/sample_statement.golden.json` - Expected parse result

## 9. Tests

### Existing Test Files:
1. `cmd/server/migration_test.go` - Migration idempotency
2. `statement/mapper_test.go` - `TestMapToStatementRecord`, `TestMapToTransactionRecords`
3. `statement/parser_test.go` - `TestParse_FromTextFixture` (golden test)
4. `statement/source_test.go` - `TestExtractText_FromPDFFixture`
5. `api/transactions_test.go` - `TestParseTransactionFilter`

### Coverage Gaps:
1. **API Handlers**: No HTTP tests for endpoints
2. **Integration**: No end-to-end tests (upload→parse→store→query)
3. **Error Cases**: Missing tests for malformed PDFs, DB failures
4. **Analytics**: No tests for `GetAnalyticsSummary`

### Brittle Cases:
- Parser tests depend on external `pdftotext`
- Golden tests may break on parser changes

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
3. **IBAN format**: No validation beyond regex match

### Error Model Gaps:
1. **Sentinel errors**: Only `ErrNotFound` and `ErrStatementExists` defined
2. **Error wrapping**: Some errors not wrapped with `%w`
3. **User messages**: Some error messages could be more specific

### Startup/Migration Issues: NONE - Complete migration system

### Contract Mismatches:
1. **Period mapping**: Parser returns `PeriodFrom`/`PeriodTo` (DD/MM/YYYY) → Storage expects single `Period` (YYYY-MM)
2. **Amount sign**: Parser assumes negative = debit, positive = credit

## 11. Stage 3 Working Notes

### Already Complete:
- ✅ All API endpoints implemented
- ✅ Complete parser with CLI tools
- ✅ Full storage layer with migrations
- ✅ Analytics queries
- ✅ Middleware (logging, recovery)
- ✅ Transaction filtering/pagination

### Partially Implemented: NOTHING - Stage 3 appears complete

### Recommended Next Steps:
1. **Testing**: Add comprehensive test coverage
2. **Validation**: Enhance input validation
3. **Error handling**: Standardize error codes
4. **Documentation**: API docs, deployment guide

### Most Relevant Files for Maintenance:
- `api/server.go` - Route definitions and handler wiring
- `statement/parser.go` - Core parsing logic (most fragile)
- `statement/query.go` - Query building and filtering
- `cmd/server/main.go` - Migration system

## 12. Known Unknowns

### Unresolved Points:
1. **Authentication/Authorization**: None implemented - assumed personal use
2. **Rate limiting**: No protection against abuse
3. **PDF library fallback**: If `pdftotext` not available, no alternative
4. **Category system**: Rules/ML not implemented - only manual assignment
5. **Deployment**: No Dockerfile, no configuration management

### Questions for Confirmation:
1. Is 32MB upload limit sufficient for multi-page PDFs?
2. Should `period` in statements table be derived from `PeriodFrom` or `PeriodTo`?
3. Are there any batch operations needed (bulk category updates)?

## 13. Assistant Handoff

### Best Files to Read First:
1. `api/server.go` - Complete API surface
2. `statement/parser.go` - Core parsing logic
3. `statement/query.go` - Database query patterns
4. `cmd/server/main.go` - Server setup and migrations

### Safe Assumptions:
1. Go 1.25+ (build constraints present)
2. PostgreSQL with pgx v5
3. External `pdftotext` command required
4. French locale for dates/amounts

### Dangerous Assumptions:
1. PDF format stability - Nickel could change layout
2. No authentication required in production
3. `pdftotext` produces consistent output across versions

### Best First Edit Targets (for enhancements):
1. `api/analytics.go` - Add more analytics endpoints
2. `statement/parser.go` - Improve error handling for malformed PDFs
3. `api/middleware.go` - Add authentication middleware
4. `statement/query.go` - Add more filter options

### Next Patch Candidates by File Path:
1. **Enhance validation**: `api/statements.go` - Add file type/content validation
2. **Add error sentinels**: `statement/query.go` - Define more error types
3. **Improve logging**: `api/middleware.go` - Add request ID tracing
4. **Add configuration**: `cmd/server/main.go` - Environment variable validation

## Change Summary

**Files/Packages Inspected**:
- `api/middleware.go` - Complete
- `api/respond.go` - Complete  
- `api/server.go` - Complete
- `api/analytics.go` - Complete
- `api/statements.go` - Complete
- `api/transactions.go` - Complete
- `api/transactions_test.go` - Complete
- `cmd/server/main.go` - Complete
- `cmd/server/migration_test.go` - Complete
- `cmd/import/main.go` - Complete
- `cmd/parser/main.go` - Complete
- `statement/analytics.go` - Complete
- `statement/api_model.go` - Complete
- `statement/parsed_model.go` - Complete
- `statement/parser.go` - Complete
- `statement/query.go` - Complete
- `statement/repository.go` - Complete
- `statement/source.go` - Complete
- `statement/storage_model.go` - Complete
- `statement/mapper.go` - Complete
- `statement/mapper_test.go` - Complete
- `statement/parser_test.go` - Complete
- `statement/source_test.go` - Complete

**Guideline Compliance**:
- ✓ Uses `any` not `interface{}`
- ✓ Uses `slog` for structured logging
- ✓ Uses `slices` package (SortFunc, Clone)
- ✓ Uses `errors.Join` for error combination
- ✓ Uses `fmt.Appendf` for byte building
- ✓ Uses `context.Context` as first parameter
- ✗ Missing: `maps` package usage (manual map loops)
- ✗ Missing: `for range n` loops (uses traditional for)
- ✗ Missing: `t.Context()` in tests (uses `context.Background()`)

**Repository State**: Production-ready Stage 3 implementation. All core features complete. Ready for testing enhancement and deployment preparations
