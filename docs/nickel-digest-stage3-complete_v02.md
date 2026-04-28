# Nickel Statement Analyzer — Stage 3 Complete Digest

**Generated:** 2026-04-27  
**Stage:** 3 (Go REST API) — ✅ COMPLETE  
**Next:** Stage 4 — Angular frontend (Angular 21 + Angular Material + ngx-echarts v21)

---

## 1. Project Overview

Parse Nickel bank PDF statements, store transactions in PostgreSQL, serve a REST API, then visualize with an Angular SPA.

**Tech stack:**
- Go 1.25 (`go.mod`: `module nickel`, `go 1.25.0`)
- PostgreSQL 16 (via `pgx/v5`)
- Angular 21 + Angular Material + ngx-echarts v21 (Stage 4, not yet started)
- External dependency: `pdftotext` (Poppler) must be in PATH

**Go guidelines:** All code targets Go 1.25+ idioms — `any`, `min`/`max`, `for range n`, `slices`/`maps` packages, `log/slog`, `errors.Join`, `sync.WaitGroup.Go`, `t.Context()` in tests. Build constraint `//go:build go1.25` on every file.

---

## 2. Repository Structure

```
nickel/
├── api/
│   ├── server.go          # Server struct, NewServer(), routes(), Handler()
│   ├── statements.go      # handleUpload, handleListStatements, handleGetStatement,
│   │                      # handleListStatementTransactions, handleHealth
│   ├── transactions.go    # handleListTransactions, handlePatchCategory,
│   │                      # serveTransactionList, parseTransactionFilter
│   ├── analytics.go       # handleAnalyticsSummary, validatePeriodParams
│   ├── middleware.go      # logging(), recovery(), wrappedWriter
│   ├── respond.go         # respondJSON(), respondError(), decodeJSON()
│   └── transactions_test.go
├── cmd/
│   ├── server/main.go     # Entry point: env, pgxpool, migrations, HTTP server
│   ├── import/main.go     # CLI: parse PDF/TXT → DB
│   └── parser/main.go     # CLI: parse PDF/TXT → JSON stdout
├── statement/
│   ├── parser.go          # Parse(), parseHeader(), parseTransactions()
│   ├── source.go          # ParseFile(), Read(), ExtractText() (calls pdftotext)
│   ├── parsed_model.go    # ParsedStatement, ParsedTransaction, formatAmountEuro()
│   ├── storage_model.go   # StatementRecord, TransactionRecord
│   ├── api_model.go       # StatementResponse, TransactionResponse, PagedTransactions,
│   │                      # MonthlySummary, AnalyticsSummary + ToResponse() funcs
│   ├── mapper.go          # MapToStatementRecord(), MapToTransactionRecords()
│   ├── repository.go      # ImportStatement(), ErrStatementExists, ImportResult
│   ├── query.go           # ListStatements, GetStatementByID, ListTransactions,
│   │                      # CountTransactions, GetTransactionByID,
│   │                      # UpdateTransactionCategory, ErrNotFound, TransactionFilter
│   └── analytics.go       # GetAnalyticsSummary(), queryMonthlyBreakdown(),
│                          # queryGroupedTotals(), periodWhere()
├── migrations/
│   ├── 001_create_statements.up.sql
│   ├── 001_create_statements.down.sql
│   ├── 002_create_transactions.up.sql
│   └── 002_create_transactions.down.sql
├── docker-compose.yml
├── Makefile
└── go.mod
```

---

## 3. Environment & Running

### .env (project root, required)
```dotenv
POSTGRES_USER=nickel
POSTGRES_PASSWORD=nickel_dev
POSTGRES_DB=nickel_db
DATABASE_URL=postgres://nickel:nickel_dev@localhost:5432/nickel_db?sslmode=disable
PORT=8080
```

**Important:** `go run ./cmd/server` does NOT load `.env`. Always use:
```bash
make run-server        # Makefile sources .env via -include .env + export
```

Or inline:
```bash
export $(cat .env | xargs) && go run ./cmd/server
```

### Docker
```bash
docker compose up -d postgres   # starts postgres:16-alpine, exposes :5432
docker compose down -v          # wipe volume when reinitializing credentials
```

Server applies `migrations/*.up.sql` automatically at startup via `schema_migrations` table — no external migrate CLI needed.

### Migrations run order
`cmd/server/main.go:runMigrations()` scans `migrations/` for `*.up.sql`, parses version from filename prefix (`001_`, `002_`), sorts, skips already-applied versions, runs each in a transaction, records version in `schema_migrations`.

---

## 4. Database Schema

```sql
-- Applied by migration 001
CREATE TABLE statements (
    id          BIGSERIAL PRIMARY KEY,
    period      VARCHAR(7)  NOT NULL,          -- "YYYY-MM"
    iban        VARCHAR(34) NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(period, iban)
);
CREATE INDEX idx_statements_period ON statements(period);
-- update_updated_at_column() trigger function + trigger also created

-- Applied by migration 002
CREATE TABLE transactions (
    id                 BIGSERIAL PRIMARY KEY,
    statement_id       BIGINT  NOT NULL REFERENCES statements(id) ON DELETE CASCADE,
    transaction_number INTEGER NOT NULL,
    date               DATE    NOT NULL,
    type               VARCHAR(50)  NOT NULL,  -- ACHAT | VIREMENT | RETRAIT DAB | FRAIS RETRAIT DAB | PRELEVEMENT
    description        TEXT    NOT NULL,
    amount_cents       BIGINT  NOT NULL,        -- negative = debit, positive = credit
    category           VARCHAR(100),            -- nullable, manually or rule-assigned
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(statement_id, transaction_number)
);
-- Indexes: date, type, category, amount_cents, statement_id
-- updated_at trigger also created

-- Created by server at startup (not a migration file)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## 5. Domain Models

### Parsed (parser output)
```go
// statement/parsed_model.go
type ParsedStatement struct {
    AccountHolder   string
    IBAN            string
    PeriodFrom      string  // "DD/MM/YYYY"
    PeriodTo        string  // "DD/MM/YYYY"
    Transactions    []ParsedTransaction
    SkippedTxBlocks int
}

type ParsedTransaction struct {
    Number      int
    Date        time.Time
    DateRaw     string
    Type        string
    Description string
    AmountCents int64   // negative=debit, positive=credit
    AmountEur   string  // "-12.34"
    AmountRaw   string  // "-12,34 €"
}
```

### Storage
```go
// statement/storage_model.go
type StatementRecord struct {
    Period     string    // "YYYY-MM"
    IBAN       string
    UploadedAt time.Time
}

type TransactionRecord struct {
    StatementID       int64
    TransactionNumber int
    Date              time.Time
    Type              string
    Description       string
    AmountCents       int64
    Category          *string  // nullable
}
```

### DB Row types (query results)
```go
// statement/query.go
type StatementRow struct {
    ID         int64
    Period     string
    IBAN       string
    UploadedAt time.Time
    TxCount    int64
}

type TransactionRow struct {
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

### API Response types
```go
// statement/api_model.go
type StatementResponse struct {
    ID         int64  `json:"id"`
    Period     string `json:"period"`
    IBAN       string `json:"iban"`
    UploadedAt string `json:"uploaded_at"`          // RFC3339
    TxCount    int64  `json:"transaction_count"`
}

type TransactionResponse struct {
    ID          int64   `json:"id"`
    StatementID int64   `json:"statement_id"`
    Number      int     `json:"number"`
    Date        string  `json:"date"`               // "YYYY-MM-DD"
    Type        string  `json:"type"`
    Description string  `json:"description"`
    AmountCents int64   `json:"amount_cents"`
    AmountEur   string  `json:"amount_eur"`         // "-12.34"
    Category    *string `json:"category"`            // null when unset
}

type PagedTransactions struct {
    Data   []TransactionResponse `json:"data"`
    Total  int64                 `json:"total"`
    Limit  int                   `json:"limit"`
    Offset int                   `json:"offset"`
}

type MonthlySummary struct {
    Period      string `json:"period"`              // "YYYY-MM"
    DebitCents  int64  `json:"debit_cents"`
    CreditCents int64  `json:"credit_cents"`
    TxCount     int64  `json:"transaction_count"`
}

type AnalyticsSummary struct {
    Months     []MonthlySummary `json:"months"`
    ByType     map[string]int64 `json:"by_type"`
    ByCategory map[string]int64 `json:"by_category"`
}
```

---

## 6. HTTP API Surface

Base URL: `http://localhost:8080`  
Router: Go 1.22+ `http.ServeMux` with method+path patterns (`"POST /v1/..."`)  
All responses: `Content-Type: application/json`  
Error shape: `{"code": "ERROR_CODE", "message": "human readable"}`

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| GET | `/health` | `handleHealth` | `{"status":"ok"}` |
| POST | `/v1/statements/upload` | `handleUpload` | `multipart/form-data`, field `file` (.pdf or .txt) |
| GET | `/v1/statements` | `handleListStatements` | Returns `[]StatementResponse` |
| GET | `/v1/statements/{id}` | `handleGetStatement` | 404 if not found |
| GET | `/v1/statements/{id}/transactions` | `handleListStatementTransactions` | Same filters as global list |
| GET | `/v1/transactions` | `handleListTransactions` | Paginated, filterable |
| PATCH | `/v1/transactions/{id}/category` | `handlePatchCategory` | Body: `{"category":"Food"}` or `{"category":null}` |
| GET | `/v1/analytics/summary` | `handleAnalyticsSummary` | Optional `?period_from=YYYY-MM&period_to=YYYY-MM` |

### Transaction filters (query params)
```
limit=50          default 50, max 200
offset=0
type=ACHAT        exact match on type column
date_from=YYYY-MM-DD
date_to=YYYY-MM-DD
category=Food     exact match; "uncategorized" → IS NULL
```

### Error codes used
| Code | HTTP | Trigger |
|------|------|---------|
| `BAD_REQUEST` | 400 | invalid params, malformed JSON, missing `file` field, wrong multipart |
| `UNSUPPORTED_FILE_TYPE` | 415 | non-.pdf/.txt upload |
| `PAYLOAD_TOO_LARGE` | 413 | file > 32 MiB |
| `UNPROCESSABLE_ENTITY` / `PARSE_ERROR` | 422 | PDF parse failed |
| `ALREADY_EXISTS` | 409 | same period+IBAN already imported |
| `NOT_FOUND` | 404 | statement or transaction not found |
| `INTERNAL_ERROR` | 500 | unexpected errors |

### Middleware chain
```
request → recovery(logger) → logging(logger) → mux → handler
```
- `recovery`: catches panics, logs `path+method+panic_type`, returns 500
- `logging`: wraps `ResponseWriter` to capture status, logs after handler returns

---

## 7. Key Implementation Details

### Upload flow (`api/statements.go:handleUpload`)
1. `r.ParseMultipartForm(32MiB)` — distinguishes `ErrNotMultipart`/`ErrMissingBoundary` (→ 400) from size error (→ 413)
2. `r.FormFile("file")` — rejects missing field
3. Extension allowlist: `.pdf`, `.txt` only (→ 415 otherwise)
4. Write to `os.CreateTemp` with matching extension (needed for `pdftotext`)
5. `statement.ParseFile()` → `ParsedStatement`
6. `statement.MapToStatementRecord()` — normalizes period to "YYYY-MM" from `PeriodFrom` ("DD/MM/YYYY")
7. `statement.ImportStatement()` — atomic insert in pgx transaction, returns `ErrStatementExists` on conflict
8. Fetch back via `GetStatementByID` and return 201

### Category PATCH (`api/transactions.go:handlePatchCategory`)
- Whitespace-only strings normalized to `nil` before DB write
- Returns full updated `TransactionResponse` on 200

### Analytics (`statement/analytics.go`)
- `GetAnalyticsSummary(ctx, pool, periodFrom, periodTo)` — fully implemented
- `queryMonthlyBreakdown()` — groups by `TO_CHAR(date, 'YYYY-MM')`, computes debit/credit/count
- `queryGroupedTotals(col)` — `groupColumn` type-safe enum prevents SQL injection; only debits summed; NULL category → "uncategorized"
- `periodWhere()` — builds safe parameterized WHERE clause for both functions

### Repository (`statement/repository.go`)
- `ImportStatement()` uses `pgx.BeginTxFunc` for atomic statement+transactions insert
- `insertStatementTx()`: `ON CONFLICT (period, iban) DO NOTHING RETURNING id` — on conflict, fetches existing id and returns `ErrStatementExists`
- `insertTransactionsTx()`: `pgx.Batch` bulk insert, `ON CONFLICT (statement_id, transaction_number) DO NOTHING`

### Query building (`statement/query.go`)
- `transactionWhere()` builds safe parameterized WHERE with incrementing `$N` placeholders
- `scanTxFields()` shared scan helper used by both `pgx.Row` and `pgx.Rows` call sites
- Sentinel: `ErrNotFound` (from `pgx.ErrNoRows`)
- Sentinel: `ErrStatementExists` (in `repository.go`)

### Parsing pipeline (`statement/source.go` + `statement/parser.go`)
- `ParseFile(ctx, path, logger)` → reads file, calls `pdftotext -layout` for PDFs
- `Parse(text)` → `parseHeader()` extracts IBAN, account holder, period; `parseTransactions()` line-by-line regex
- French formats: date `02/01/2006`, amount `1 234,56 €` (Unicode thin spaces)
- Amount → cents via `normalizeAmount()` → strips internal spaces → parse float → multiply

---

## 8. Sentinel Errors

```go
// statement/query.go
var ErrNotFound = errors.New("not found")

// statement/repository.go
var ErrStatementExists = errors.New("statement already exists for this period")
```

---

## 9. go.mod Dependencies

```
module nickel
go 1.25.0

require:
  github.com/google/go-cmp v0.7.0
  github.com/jackc/pgx/v5 v5.9.1

indirect:
  github.com/jackc/pgpassfile, pgservicefile, puddle/v2
  golang.org/x/sync v0.17.0
  golang.org/x/text v0.29.0
```

No web framework — standard `net/http` only.

---

## 10. Stage 3 Status: ✅ Complete

All endpoints verified working via Postman collection (`nickel-api.postman_collection.json`).

**Tested and passing:**
- Upload PDF → parse → store → 201 with statement ID
- Duplicate upload → 409 ALREADY_EXISTS
- Wrong file type → 415 UNSUPPORTED_FILE_TYPE
- List statements, get by ID, get by ID 404
- List statement transactions (scoped pagination)
- Global transaction list with all filter combinations
- Date range validation (inverted → 400)
- PATCH category set, clear (null), whitespace normalization, 404
- Analytics summary with and without period filters
- Period format validation, inverted period range → 400

**Known non-issues (intentional design decisions):**
- No authentication — personal use tool
- No rate limiting
- `pdftotext` external dependency with no fallback
- Rule-based categorization deferred to Stage 5

---

## 11. Stage 4 Plan (Next)

**Stack decision:** Angular 21 + Angular Material + ngx-echarts v21 (Apache 2.0)

**ngx-echarts v21 install:**
```bash
npm install echarts ngx-echarts
```

**Angular 21 setup (standalone, no NgModules):**
```typescript
// app.config.ts
import { provideEchartsCore } from 'ngx-echarts';
import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);

export const appConfig: ApplicationConfig = {
  providers: [provideEchartsCore({ echarts }), ...]
};
```

**Note:** Since Angular 19+, `import * as echarts from 'echarts'` is NOT allowed — must use tree-shaken imports from `echarts/core`, `echarts/charts`, `echarts/components`.

**Phases:**
- **4a** — Upload & list: drag-and-drop PDF upload, statement list, transaction table with search/filter
- **4b** — Dashboard: monthly bar chart, category pie chart, balance trend line (ngx-echarts)
- **4c** — UX: category tag editor per transaction, date range picker, CSV export

---

## 12. Useful Make Targets

```bash
make run-server      # start Go server (loads .env)
make db-shell        # open psql in running container
make db-tables       # list tables
make test            # go test ./...
make build           # go build ./cmd/server
```

---

## 13. Sample Data Reference

The golden test fixture has 109 real transactions for period 2026-01, IBAN `FR7616598000012979019000124`, holder `M MIKHAIL KLEMIN`.

Transaction types present: `ACHAT`, `VIREMENT`, `RETRAIT DAB`, `FRAIS RETRAIT DAB`, `PRELEVEMENT`

To seed multi-month data for analytics testing, insert additional statements/transactions directly via psql (see step3-testing-guide.md for SQL).
