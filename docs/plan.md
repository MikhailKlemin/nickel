***

## Project: Nickel Statement Analyzer

### Overview

The system has three main layers: a **Go backend** (PDF ingestion, parsing, REST API), an **Angular frontend** (dashboard, charts, upload), and optionally a **Python microservice** for ML categorization later.

***

## Step 1 — PDF Parsing (Go)

This is the most critical and fragile part. Build it first in isolation.

- Use [`ledongthuc/pdfcpu`](https://github.com/pdfcpu/pdfcpu) or [`unidoc/unipdf`](https://github.com/unidoc/unipdf) to extract raw text from the PDF while preserving layout[^1_2]
- Since Nickel's format has consistent columns (No., Date, Type, Description, Amount), apply line-by-line regex to capture each transaction row[^1_1]
- Key regex targets: date (`\d{2}/\d{2}/\d{4}`), amount (`-?\d+[.,]\d{2}\s*€`), type (PURCHASE, ATM WITHDRAWAL, etc.), and description (everything in between)
- Also parse the **header section**: IBAN, account holder name, statement period
- Output: a structured `Statement` Go struct with a slice of `Transaction`

> **Deliverable:** a standalone CLI tool `parse-nickel-pdf <file.pdf>` that prints JSON — testable independently.

***

## Step 2 — Data Storage (Go + PostgreSQL)

Design the schema before building the API.

- Tables: `statements` (id, period, iban, uploaded_at), `transactions` (id, statement_id, date, type, description, amount, category)
- Add a `UNIQUE` constraint on (statement_id + transaction number) to prevent duplicates on re-upload
- Use `category` as a nullable column — populated manually first, then by ML later
- Use [`sqlc`](https://sqlc.dev/) or [`pgx`](https://github.com/jackc/pgx) directly in Go for DB access

> **Deliverable:** migrations + seeded DB with parsed data from Step 1.

***

## Step 3 — REST API (Go)

Build a clean HTTP API using the standard `net/http` or `chi`/`fiber` router.


| Endpoint | Method | Description |
| :-- | :-- | :-- |
| `/upload` | POST | Accept PDF, trigger parse, store |
| `/statements` | GET | List all uploaded statements |
| `/transactions` | GET | List transactions with filters (date range, type, category) |
| `/analytics/summary` | GET | Monthly totals, avg spend, top categories |
| `/transactions/:id/category` | PATCH | Manually assign a category |

- Return JSON throughout; include pagination for `/transactions`
- Add basic duplicate detection (same period already uploaded)

> **Deliverable:** Postman/Bruno collection proving all endpoints work.

***

## Step 4 — Angular Frontend

Build the UI in phases, starting with data display before adding charts.

- **Phase 4a — Upload \& list:** drag-and-drop PDF upload, statement list, raw transaction table with search/filter
- **Phase 4b — Dashboard:** monthly spending bar chart, spending-by-category pie chart, balance trend line chart (use `ngx-charts` or `Chart.js`)
- **Phase 4c — UX:** category tag editor per transaction, date range picker, CSV export

> **Deliverable:** working SPA connected to the Go API.

***

## Step 5 — Transaction Categorization

Start with **rule-based** categories in Go (no ML needed initially):

- Build a JSON config of keyword → category mappings (e.g., `"TOTALROANNE" → "Transport"`, `"Foot Locker" → "Shopping"`)
- Apply rules at parse time; unknown transactions stay uncategorized
- Let the user manually categorize unknowns via the UI (Step 4c)

If rule-based coverage is insufficient, **add Python** as a sidecar microservice:

- Train a simple classifier (scikit-learn `TfidfVectorizer` + `LogisticRegression`) on labeled transactions
- Expose it as a `/categorize` HTTP endpoint called by the Go API
- Retrain periodically as the user labels more data

> **Deliverable:** >80% of transactions auto-categorized after a few months of usage.

***

## Step 6 — Productionization

- Dockerize all services (`go-api`, `angular`, optionally `python-ml`) with `docker-compose`
- Add a simple **auth layer** (JWT or even HTTP Basic Auth since it's personal-use)
- Scheduled job or manual trigger to re-run categorization after model retrain
- Optionally add a **PDF watch folder** so dropping a file auto-triggers parsing

***

## Recommended Implementation Order

1. PDF parser CLI (Step 1) — validates your core assumption about the PDF format
2. DB schema + Go API skeleton (Steps 2–3) — wires parsing into storage
3. Angular upload + transaction table (Step 4a) — makes it usable
4. Rule-based categorization (Step 5, first part) — adds immediate value
5. Dashboard charts (Step 4b–4c) — the main user-facing goal
6. ML categorization (Step 5, second part) — only if rules aren't enough
