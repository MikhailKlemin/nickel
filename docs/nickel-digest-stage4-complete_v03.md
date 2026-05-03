# Nickel Statement Analyzer — Stage 4 Complete Digest

**Generated:** 2026-05-02  
**Stage:** 4 (Angular Frontend) — ✅ COMPLETE  
**Next:** Stage 4c remainder — Go export endpoint (`GET /v1/transactions/export`, `GET /v1/statements/{id}/export`), then Stage 5 — rule-based categorization

---

## 1. Project Overview

Parse Nickel bank PDF statements, store transactions in PostgreSQL, serve a REST API, then visualize with an Angular SPA.

**Tech stack:**
- Go 1.25 (`go.mod`: `module nickel`, `go 1.25.0`)
- PostgreSQL 16 (via `pgx/v5`)
- Angular 21 + Angular Material + ngx-echarts v21
- External dependency: `pdftotext` (Poppler) must be in PATH

**Go guidelines:** All code targets Go 1.25+ idioms — `any`, `min`/`max`, `for range n`, `slices`/`maps` packages, `log/slog`, `errors.Join`, `sync.WaitGroup.Go`, `t.Context()` in tests. Build constraint `//go:build go1.25` on every file.

---

## 2. Repository Structure

```
nickel/
├── api/
│   ├── server.go
│   ├── statements.go
│   ├── transactions.go
│   ├── analytics.go
│   ├── middleware.go
│   ├── respond.go
│   └── transactions_test.go
├── cmd/
│   ├── server/main.go
│   ├── import/main.go
│   └── parser/main.go
├── frontend/                          # Angular 21 SPA
│   ├── src/
│   │   ├── main.ts
│   │   ├── styles.scss
│   │   └── app/
│   │       ├── app.ts                 # Root component, nav, document title tracking
│   │       ├── app.html
│   │       ├── app.scss
│   │       ├── app.config.ts          # provideEchartsCore, provideRouter, provideHttpClient
│   │       ├── app.routes.ts          # Lazy-loaded routes with data.title
│   │       ├── core/
│   │       │   ├── models/
│   │       │   │   └── statement.model.ts
│   │       │   ├── services/
│   │       │   │   └── nickel-api.service.ts
│   │       │   └── interceptors/
│   │       │       └── error.interceptor.ts
│   │       └── features/
│   │           ├── statements/
│   │           │   └── statement-list/
│   │           │       ├── statement-list.component.ts
│   │           │       ├── statement-list.component.html
│   │           │       └── statement-list.component.scss
│   │           ├── transactions/
│   │           │   └── transaction-list/
│   │           │       ├── transaction-list.component.ts
│   │           │       ├── transaction-list.component.html
│   │           │       └── transaction-list.component.scss
│   │           ├── upload/
│   │           │   └── upload/
│   │           │       ├── upload.component.ts
│   │           │       ├── upload.component.html
│   │           │       └── upload.component.scss
│   │           └── analytics/
│   │               └── analytics/
│   │                   ├── analytics.component.ts
│   │                   ├── analytics.component.html
│   │                   └── analytics.component.scss
├── statement/
│   ├── parser.go
│   ├── source.go
│   ├── parsed_model.go
│   ├── storage_model.go
│   ├── api_model.go
│   ├── mapper.go
│   ├── repository.go
│   ├── query.go
│   └── analytics.go
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

## 3. Angular Conventions (CONVENTIONS.md)

All frontend code strictly follows `CONVENTIONS.md` at the repo root. Key rules:

- **Standalone APIs only** — no `NgModule`
- **Signal-first** — `signal()`, `computed()`, `effect()`, `input()`, `output()`
- **`ChangeDetectionStrategy.OnPush`** on every component
- **`inject()`** over constructor injection; constructors omitted
- **Built-in control flow** — `@if`, `@for` (always with `track`), `@switch`
- **No `async` pipe** — signals via `toSignal()`
- **`Array<T>` / `ReadonlyArray<T>`** — never `T[]`
- **`protected`** for template-bound members
- **Member order**: protected fields → private fields → public methods → protected methods → private methods
- **No `ngClass`/`ngStyle`** — use `[class.x]` / `[style.x]`
- **RxJS cleanup** via `takeUntilDestroyed()`

---

## 4. Frontend: Angular Setup

### ngx-echarts install
```bash
npm install echarts ngx-echarts
```

### app.config.ts — echarts registration
```typescript
import { provideEchartsCore } from 'ngx-echarts';
import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, MarkLineComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, MarkLineComponent, CanvasRenderer]);

export const appConfig: ApplicationConfig = {
  providers: [
    provideEchartsCore({ echarts }),
    provideRouter(routes, withComponentInputBinding()),  // binds route params + query params to inputs
    provideHttpClient(withInterceptors([errorInterceptor])),
    ...
  ],
};
```

**Critical:** `import * as echarts from 'echarts'` is NOT allowed since Angular 19+ — must use tree-shaken imports from `echarts/core`.

**Critical:** `MarkLineComponent` must be registered in `echarts.use()` for `markLine` on the trend chart to render.

### ECharts typing note
`CallbackDataParams` is not exported from the top-level `echarts` package. Import from:
```typescript
import type { CallbackDataParams } from 'echarts/types/dist/shared';
```

For mixed series arrays (e.g. `LineSeriesOption` + `BarSeriesOption`), declare each series as an explicitly typed const before passing into `series: []` — avoids union type inference errors:
```typescript
import type { BarSeriesOption, EChartsOption, LineSeriesOption } from 'echarts';

const lineSeries: LineSeriesOption = { type: 'line', ... };
const barSeries: BarSeriesOption  = { type: 'bar',  ... };
this.options.set({ series: [lineSeries, barSeries] });
```

### Document title tracking
Handled in `App` (root component) via a field-initializer subscription — not `ngOnInit` — so `takeUntilDestroyed()` runs in the injection context:

```typescript
private readonly titleSub = this.router.events.pipe(
  filter((e) => e instanceof NavigationEnd),
  takeUntilDestroyed(),
).subscribe(() => {
  let route = this.activatedRoute;
  while (route.firstChild) route = route.firstChild;
  const pageTitle = route.snapshot.data['title'];
  this.titleService.setTitle(pageTitle ? `${pageTitle} — Nickel` : 'Nickel');
});
```

Route `data.title` values: `'Statements'`, `'Transactions'`, `'Upload Statement'`, `'Analytics'`.

### withComponentInputBinding() — query param → input
`provideRouter(routes, withComponentInputBinding())` automatically binds query params to `input()` fields by matching name. Used in `TransactionListComponent`:

```typescript
// Populated from ?category=uncategorized automatically
protected readonly category = input<string>('');
```

`ngOnInit` seeds the filter signal from it:
```typescript
public ngOnInit(): void {
  if (this.category()) this.filterCategory.set(this.category());
  this.load();
}
```

---

## 5. Frontend: Routes

```typescript
// app.routes.ts
{ path: '',                            redirectTo: 'statements', pathMatch: 'full' },
{ path: 'statements',                  data: { title: 'Statements' },       loadComponent: ... },
{ path: 'statements/:id/transactions', data: { title: 'Transactions' },     loadComponent: ... },
{ path: 'transactions',                data: { title: 'Transactions' },     loadComponent: ... },
{ path: 'upload',                      data: { title: 'Upload Statement' }, loadComponent: ... },
{ path: 'analytics',                   data: { title: 'Analytics' },        loadComponent: ... },
{ path: '**',                          redirectTo: 'statements' },
```

Both `/transactions` (global) and `/statements/:id/transactions` (scoped) load `TransactionListComponent`. The `id` input distinguishes them — `Number(id) > 0` means statement-scoped view.

---

## 6. Frontend: Core Models

```typescript
// core/models/statement.model.ts

export type TransactionType =
  | 'ACHAT'
  | 'VIREMENT'
  | 'RETRAIT DAB'
  | 'FRAIS RETRAIT DAB'
  | 'PRELEVEMENT';

export interface Statement {
  id:                number;
  period:            string;   // "YYYY-MM"
  iban:              string;
  uploaded_at:       string;   // RFC3339
  transaction_count: number;
}

export interface Transaction {
  id:           number;
  statement_id: number;
  number:       number;
  date:         string;    // "YYYY-MM-DD"
  type:         TransactionType;
  description:  string;
  amount_cents: number;
  amount_eur:   string;   // "-12.34"
  category:     string | null;
}

export interface PagedTransactions {
  data:   Array<Transaction>;
  total:  number;
  limit:  number;
  offset: number;
}

export interface MonthlySummary {
  period:            string;  // "YYYY-MM"
  debit_cents:       number;
  credit_cents:      number;
  transaction_count: number;
}

export interface AnalyticsSummary {
  months:      Array<MonthlySummary>;
  by_type:     Record<string, number>;
  by_category: Record<string, number>;
}

export interface TransactionFilter {
  limit?:     number;
  offset?:    number;
  type?:      string;
  date_from?: string;
  date_to?:   string;
  category?:  string;
}

export interface ApiError {
  code:    string;
  message: string;
}
```

---

## 7. Frontend: API Service

```typescript
// core/services/nickel-api.service.ts
// Base URL: http://localhost:8080

listStatements(): Observable<Array<Statement>>
uploadStatement(file: File): Observable<Statement>           // POST multipart
getStatement(id: number): Observable<Statement>
listTransactions(filter?: TransactionFilter): Observable<PagedTransactions>
getStatementTransactions(statementId: number, filter?: TransactionFilter): Observable<PagedTransactions>
patchCategory(id: number, category: string | null): Observable<Transaction>
getAnalyticsSummary(periodFrom?: string, periodTo?: string): Observable<AnalyticsSummary>
```

`buildParams(filter)` converts `TransactionFilter` to `HttpParams`, omitting undefined values.

---

## 8. Frontend: Feature Components

### StatementListComponent (`/statements`)
- Loads all statements on init, displays in `mat-table`
- IBAN truncated to 18 chars with `matTooltip` for full value
- Period shown as styled monospace badge
- Row count footer
- "View transactions" chevron navigates to `/statements/:id/transactions`

### TransactionListComponent (`/transactions` and `/statements/:id/transactions`)

**Pagination:**
- `pageSize = 50` (readonly)
- `pageIndex = signal(0)` — drives paginator `[pageIndex]` binding
- `onPage(e)` updates `pageIndex` then calls `load()`
- `onApplyFilters()` resets `pageIndex` to 0 before load — prevents landing on page 3 of a filtered result

**Filter bar** (above the table):
- Type `<mat-select>` — all `TransactionType` values + "All"
- Date from/to — `<input type="date">`
- Category — free text input; `"uncategorized"` is a special sentinel (maps to `IS NULL` in Go)
- Apply button + clear icon button (shown only when `hasActiveFilter()`)
- Filters passed directly into `TransactionFilter` shape — no backend changes needed

**Inline category editing:**
- `editingId = signal<number | null>(null)` — at most one row in edit mode
- Click anywhere in category cell (view mode) → edit mode
- Pencil icon visible on row hover only
- Enter saves, Escape cancels
- `onSaveCategory()` skips API call if value unchanged
- On success: `rows.update(rows => rows.map(...))` patches single row in-place — no full reload
- `saving` signal disables input and swaps checkmark for mini-spinner during PATCH

**Statement-scoped view:**
- `isStatementView = computed(() => Number(this.id()) > 0)`
- Shows back arrow to `/statements` when true
- Uses `getStatementTransactions()` instead of `listTransactions()`

**Uncategorized shortcut:**
- `category = input<string>('')` bound from `?category=` query param
- `ngOnInit` seeds `filterCategory` signal from it
- Navigate to `/transactions?category=uncategorized` to pre-filter

### UploadComponent (`/upload`)
- Drag-and-drop zone + file input button
- Button disabled while `uploading()`
- 409 conflict handled specifically: "This statement period has already been imported."
- Other errors shown via `MatSnackBar`
- Success card shows period + transaction count + link to statements

### AnalyticsComponent (`/analytics`)

**Data loading:**
- `forkJoin` — parallel requests: `getAnalyticsSummary()` + `listTransactions({ category: 'uncategorized', limit: 1 })` (uses `total` only for count)

**KPI cards (computed signals):**
- `totalDebitEur` — sum of `Math.abs(debit_cents)` across all months
- `totalCreditEur` — sum of `credit_cents` across all months
- `worstMonth` — month with highest absolute debit
- `topCategory` — highest-spend category (excludes "uncategorized")
- `uncategorized` — plain signal from the parallel request
- Uncategorized card turns amber when count > 0; "Review →" links to `/transactions?category=uncategorized`

**Charts (three, built in separate private methods):**

`buildBarChart()` — monthly debit/credit grouped bar chart

`buildPieChart()` — spending by category donut; excludes "uncategorized" slice

`buildTrendChart()` — combined line + bar:
- **Cumulative balance line** (blue, smooth, area fill): running sum of `credit_cents + debit_cents` per month
- **Monthly net bars** (green/red per value): `credit_cents + debit_cents` per month
- Zero baseline `markLine` (requires `MarkLineComponent` registered in echarts.use)
- Each bar coloured individually via `itemStyle.color: (params: CallbackDataParams) => ...`

---

## 9. HTTP API Surface (unchanged from Stage 3)

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| GET | `/health` | `handleHealth` | `{"status":"ok"}` |
| POST | `/v1/statements/upload` | `handleUpload` | `multipart/form-data`, field `file` |
| GET | `/v1/statements` | `handleListStatements` | Returns `[]StatementResponse` |
| GET | `/v1/statements/{id}` | `handleGetStatement` | 404 if not found |
| GET | `/v1/statements/{id}/transactions` | `handleListStatementTransactions` | Same filters as global |
| GET | `/v1/transactions` | `handleListTransactions` | Paginated, filterable |
| PATCH | `/v1/transactions/{id}/category` | `handlePatchCategory` | Body: `{"category":"Food"}` or `{"category":null}` |
| GET | `/v1/analytics/summary` | `handleAnalyticsSummary` | Optional `?period_from=YYYY-MM&period_to=YYYY-MM` |

### Transaction filters (query params)
```
limit=50          default 50, max 200
offset=0
type=ACHAT
date_from=YYYY-MM-DD
date_to=YYYY-MM-DD
category=Food     exact match; "uncategorized" → IS NULL
```

---

## 10. Error Interceptor

`core/interceptors/error.interceptor.ts` — global `HttpInterceptorFn`:
- Shows `MatSnackBar` for all errors except 409 (duplicate upload — handled per-component)
- Displays `err.error.message` if available, otherwise generic fallback

---

## 11. Stage 4 Status

### ✅ Phase 4a — Upload & list
- Drag-and-drop PDF upload with progress, error handling, success card
- Statement list with IBAN truncation, period badge, transaction count, navigate to scoped view
- Transaction table with pagination, sticky header, amount colouring, type chip colours

### ✅ Phase 4b — Dashboard charts
- Monthly spending bar chart (debit/credit)
- Spending by category donut chart
- Balance trend line chart (cumulative + monthly net bars)

### ✅ Phase 4c — UX (partial)
- Inline category editing per transaction row ✅
- Filter bar (type, date range, category, clear) ✅
- Document title updates per route ✅
- CSV export ❌ — blocked on backend export endpoint (next task)

---

## 12. Next Steps

### Immediate — new chat (Go backend)
Add two export endpoints to `api/transactions.go`:

| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/transactions/export` | Same filters as list, no pagination cap, streams `text/csv` |
| GET | `/v1/statements/{id}/export` | Scoped to statement, same behaviour |

Implementation notes:
- Reuse `parseTransactionFilter()` and `transactionWhere()` — no new query building needed
- Remove `LIMIT`/`OFFSET` from the query
- Write `Content-Type: text/csv` + `Content-Disposition: attachment; filename="transactions-{period}.csv"` headers before first write
- Stream rows directly via `pgx.Rows` → `encoding/csv` writer — no buffering the full result set
- Frontend: add an Export button to `TransactionListComponent` that opens the URL with current filter params as query string

### After export — Stage 5 (rule-based categorization)
JSON config of keyword → category mappings, applied at import time and retroactively via a new endpoint. See plan.md §Step 5.

**Deferred decisions:**
- Category management UI (dropdown of existing categories + "New…" option) — derive from `SELECT DISTINCT category` for now; a proper `categories` table deferred until multi-user / web service phase
- Analytics period preset filter (Last 3/12 months, by year) — API already supports `?period_from/to`; frontend preset button bar deferred
- ML categorization — deferred to Stage 5 second part if rule coverage is insufficient

---

## 13. Go Backend (unchanged from Stage 3)

### go.mod
```
module nickel
go 1.25.0

require:
  github.com/google/go-cmp v0.7.0
  github.com/jackc/pgx/v5 v5.9.1
```

### Sentinel errors
```go
var ErrNotFound      = errors.New("not found")           // statement/query.go
var ErrStatementExists = errors.New("statement already exists for this period") // statement/repository.go
```

### Useful Make targets
```bash
make run-server      # start Go server (loads .env)
make db-shell        # open psql in running container
make db-tables       # list tables
make test            # go test ./...
make build           # go build ./cmd/server
```

### .env
```dotenv
POSTGRES_USER=nickel
POSTGRES_PASSWORD=nickel_dev
POSTGRES_DB=nickel_db
DATABASE_URL=postgres://nickel:nickel_dev@localhost:5432/nickel_db?sslmode=disable
PORT=8080
```

---

## 14. Sample Data Reference

Golden test fixture: 109 transactions, period `2026-01`, IBAN `FR7616598000012979019000124`, holder `M MIKHAIL KLEMIN`.

Transaction types: `ACHAT`, `VIREMENT`, `RETRAIT DAB`, `FRAIS RETRAIT DAB`, `PRELEVEMENT`

Multiple years of real statements available for import — trend chart becomes meaningful with full dataset loaded.
