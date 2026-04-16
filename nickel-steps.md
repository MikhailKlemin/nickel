<img src="https://r2cdn.perplexity.ai/pplx-full-logo-primary-dark%402x.png" style="height:64px;margin-right:32px"/>

# I am custome rof Nickel neobank in France. The problem that it does not provide you any tools for your spending analysis. All you can do is export statement each months in PDF.

I would like to write a service which would accept the PDF parse it, store and provide tools for basic analysis. The stack would be  Go, Angular/TS  and maybe Pyhton for ML if we would have to use it.

My initial idea would be to parse PDF  into text file with laytout preserve and then apply some regexp line by line to capture data we need.

At this stage I would like you to outline the whole project and split it by steps, so we can start implementing it step by step.

Based on the Nickel statement format visible in the search results, the PDF has a clear tabular structure with columns: **No. | DATE | TYPE OF OPERATION | DESCRIPTION | AMOUNT** [^1_1]. This makes your regex-based approach very viable. Here's a full project outline:

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

Starting with the parser CLI as a throwaway script is key — Nickel could change their PDF layout, and you want to isolate that fragility before building anything on top of it.[^1_1]
<span style="display:none">[^1_10][^1_11][^1_12][^1_13][^1_14][^1_15][^1_3][^1_4][^1_5][^1_6][^1_7][^1_8][^1_9]</span>

<div align="center">⁂</div>

[^1_1]: https://fr.scribd.com/document/948367209/nickel

[^1_2]: https://pkg.go.dev/github.com/potterbl/statement_parser

[^1_3]: https://support.nickel.eu/fr-en/how-can-i-obtain-my-account-statements

[^1_4]: https://support.nickel.eu/en-be/how-can-i-obtain-my-account-statements

[^1_5]: https://nickel.eu/sites/default/files/General-Terms-and-Conditions_FR_en.pdf

[^1_6]: https://es.scribd.com/document/964176664/Account-Statement-3

[^1_7]: https://nickel.eu/sites/default/files/General-Terms-and-Conditions_BE_en.pdf

[^1_8]: https://parsio.io/bank-statements/

[^1_9]: https://pypi.org/project/pdf-bank-statement-parser/

[^1_10]: https://parseur.com/solution/banking-finance

[^1_11]: https://nickel.eu/sites/default/files/Use-on-Personal-Data-Policy-Prospects-Customers_FR_en.pdf

[^1_12]: https://github.com/mstroppel/BankStatementParser

[^1_13]: https://mybankstatementanalysis.com/bank-statement-analyser

[^1_14]: https://nickel.eu/sites/default/files/Self-certification-form-FATCA-AEOI-FR_fr.pdf

[^1_15]: https://www.reddit.com/r/golang/comments/1mikkwd/created_a_neat_app_that_decrypts_pdf_bank/

