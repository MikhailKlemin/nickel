CREATE TABLE transactions (
    id BIGSERIAL PRIMARY KEY,
    statement_id BIGINT NOT NULL REFERENCES statements(id) ON DELETE CASCADE,
    transaction_number INT NOT NULL,
    date DATE NOT NULL,
    type VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    category VARCHAR(100) NULL,
    UNIQUE (statement_id, transaction_number)
);

CREATE INDEX transactions_statement_id_idx ON transactions (statement_id);
CREATE INDEX transactions_date_idx ON transactions (date);
CREATE INDEX transactions_category_idx ON transactions (category);
