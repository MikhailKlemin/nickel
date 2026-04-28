# Step 3 — Populate DB & Test API with Postman

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Go | 1.25+ | `brew install go` |
| Docker | any | [docs.docker.com](https://docs.docker.com/get-docker/) |
| Postman | any | [postman.com](https://www.postman.com/downloads/) |

---

## 1. Environment setup

### 1a. Create `.env` at the project root

```dotenv
POSTGRES_USER=nickel
POSTGRES_PASSWORD=nickel_dev
POSTGRES_DB=nickel_db
DATABASE_URL=postgres://nickel:nickel_dev@localhost:5432/nickel_db?sslmode=disable
PORT=8080
```

The Makefile sources this file automatically (`-include .env` / `export`).

### 1b. Start Postgres

```bash
docker compose up -d postgres

# Wait for healthy
docker compose ps          # should show "(healthy)"
```

---

## 2. Run the server (migrations included)

The server applies all `migrations/*.up.sql` files automatically at startup via its built-in migration runner — no external `migrate` CLI needed.

```bash
go run ./cmd/server
```

Expected output:
```
time=... level=INFO msg="applied migration" version=1 file=001_create_statements.up.sql
time=... level=INFO msg="applied migration" version=2 file=002_create_transactions.up.sql
time=... level=INFO msg="database connection established"
time=... level=INFO msg="starting server" port=8080
```

Verify the tables were created:

```bash
make db-tables
# Should list: schema_migrations, statements, transactions
```

---

## 3. Seed real data

The fastest path is using the upload endpoint directly — no SQL needed.

### Option A — curl (one command)

```bash
curl -s -X POST http://localhost:8080/v1/statements/upload \
  -F "file=@statement/testdata/sample_statement.pdf" \
  | jq .
```

Expected response (abbreviated):
```json
{
  "id": 1,
  "period": "2026-01",
  "iban": "FR7616598000012979019000124",
  "uploaded_at": "2026-04-26T...",
  "transaction_count": 109
}
```

### Option B — Postman (see Section 4)

Use the **Upload Statement (PDF)** request from the collection. The test script captures `statement_id` automatically.

### Option C — import CLI (if your project has `cmd/import`)

```bash
go run ./cmd/import --file statement/testdata/sample_statement.pdf
```

### Verifying in DB

```bash
make db-shell

-- Inside psql:
SELECT id, period, iban, transaction_count
FROM statements
JOIN (SELECT statement_id, count(*) AS transaction_count FROM transactions GROUP BY statement_id) t
  ON t.statement_id = statements.id;

SELECT id, date, type, description, amount_cents
FROM transactions
LIMIT 10;
```

### Seeding multiple months (optional)

To get meaningful analytics across months, you need statements for different periods. Since you only have one sample PDF, you can seed synthetic data directly:

```bash
# Open psql
make db-shell
```

```sql
-- Insert a second statement for Feb 2026
INSERT INTO statements (period, iban)
VALUES ('2026-02', 'FR7616598000012979019000124')
RETURNING id;
-- Note the returned id, e.g. 2

-- Seed a handful of transactions for Feb
INSERT INTO transactions (statement_id, transaction_number, date, type, description, amount_cents)
VALUES
  (2, 1, '2026-02-03', 'ACHAT',       'LIDL AMIENS',             -3420),
  (2, 2, '2026-02-05', 'ACHAT',       'AMAZON MARKETPLACE',      -1999),
  (2, 3, '2026-02-10', 'VIREMENT',    'SALAIRE FEVRIER',         150000),
  (2, 4, '2026-02-14', 'ACHAT',       'SNCF BILLET PARIS',       -5490),
  (2, 5, '2026-02-20', 'PRELEVEMENT', 'EDF ELECTRICITE',         -8700),
  (2, 6, '2026-02-22', 'RETRAIT DAB', 'DISTRIBUTEUR BNP',       -10000),
  (2, 7, '2026-02-25', 'ACHAT',       'DECATHLON AMIENS',        -4599),
  (2, 8, '2026-02-28', 'ACHAT',       'CARREFOUR CITY',          -2134);

-- Seed a March statement
INSERT INTO statements (period, iban)
VALUES ('2026-03', 'FR7616598000012979019000124')
RETURNING id;

INSERT INTO transactions (statement_id, transaction_number, date, type, description, amount_cents)
VALUES
  (3, 1, '2026-03-01', 'ACHAT',       'MATCH VILLENEUVE',         -1890),
  (3, 2, '2026-03-05', 'VIREMENT',    'REMBOURSEMENT AMI',         5000),
  (3, 3, '2026-03-10', 'PRELEVEMENT', 'ORANGE MOBILE',            -2099),
  (3, 4, '2026-03-15', 'ACHAT',       'RESTAURANT LE ZINC',       -3200),
  (3, 5, '2026-03-20', 'RETRAIT DAB', 'CASH SERVICES',           -20000),
  (3, 6, '2026-03-25', 'ACHAT',       'BOULANGERIE DUPONT',        -520);
```

---

## 4. Import the Postman collection

1. Open Postman → **Import** → drag in `nickel-api.postman_collection.json`
2. The collection creates one variable automatically: `base_url = http://localhost:8080`
3. `statement_id` and `transaction_id` are populated by test scripts — no manual setup

### Recommended run order

Run requests top-to-bottom within each folder. The dependency chain is:

```
Health / GET /health
  ↓
Statements / Upload Statement (PDF)         ← sets {{statement_id}}
  ↓
Statements / List Statement Transactions    ← sets {{transaction_id}}
  ↓
Transactions / PATCH Category — set        ← uses {{transaction_id}}
  ↓
Analytics / Summary                         ← uses all seeded data
```

### Run the whole collection at once

Use **Collection Runner** (▶ button next to the collection name):

- Keep default order
- Iterations: 1
- Delay: 100ms (avoids timing issues on slow machines)

All 20+ requests should show green ✓.

---

## 5. Key request reference

### Upload a PDF

```
POST /v1/statements/upload
Content-Type: multipart/form-data

file: <binary .pdf or .txt>
```

### List transactions with filters

```
GET /v1/transactions?type=ACHAT&date_from=2026-01-01&date_to=2026-01-31&limit=20&offset=0
GET /v1/transactions?category=uncategorized
GET /v1/statements/1/transactions?limit=50
```

Available `type` values from the sample statement: `ACHAT`, `VIREMENT`, `RETRAIT DAB`, `FRAIS RETRAIT DAB`, `PRELEVEMENT`

### Categorize a transaction

```
PATCH /v1/transactions/1/category
Content-Type: application/json

{"category": "Alimentation"}   // set
{"category": null}             // clear
```

### Analytics

```
GET /v1/analytics/summary
GET /v1/analytics/summary?period_from=2026-01&period_to=2026-03
```

---

## 6. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `DATABASE_URL environment variable is required` | `.env` not loaded | Run `source .env && go run ./cmd/server` or use `make run-server` |
| `409 ALREADY_EXISTS` on upload | Same period already in DB | Expected — the duplicate test is intentional |
| `500` on upload | PDF parse error or DB not ready | Check server logs; ensure Postgres is healthy |
| `transaction_id` is empty in PATCH requests | List Statement Transactions not run first | Run that request before the PATCH tests |
| Postman can't reach localhost | Server not running | Confirm `go run ./cmd/server` is running in another terminal |

---

## 7. Useful DB queries for exploration

```sql
-- Spending by type for January
SELECT type, SUM(amount_cents) / 100.0 AS total_eur, COUNT(*) AS n
FROM transactions t
JOIN statements s ON s.id = t.statement_id
WHERE s.period = '2026-01'
GROUP BY type
ORDER BY total_eur;

-- Top 10 biggest expenses
SELECT date, type, description, amount_cents / 100.0 AS eur
FROM transactions
WHERE amount_cents < 0
ORDER BY amount_cents
LIMIT 10;

-- Uncategorized transaction count
SELECT COUNT(*) FROM transactions WHERE category IS NULL;

-- After some PATCH calls: breakdown by category
SELECT category, COUNT(*) AS n, SUM(amount_cents) / 100.0 AS total_eur
FROM transactions
WHERE category IS NOT NULL
GROUP BY category
ORDER BY total_eur;
```
