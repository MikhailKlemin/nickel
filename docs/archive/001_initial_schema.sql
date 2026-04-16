-- Initial schema for Nickel Statement Analyzer
-- Created: 2025-01-01

-- Enable UUID extension if needed (not used in initial version)
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Statements table: holds metadata for each uploaded statement
CREATE TABLE statements (
    id BIGSERIAL PRIMARY KEY,
    period VARCHAR(7) NOT NULL,               -- YYYY-MM format
    iban VARCHAR(34),                         -- IBAN can be up to 34 characters
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Ensure we don't import the same period twice for the same IBAN
    UNIQUE(period, iban)
);

-- Transactions table: individual operations from a statement
CREATE TABLE transactions (
    id BIGSERIAL PRIMARY KEY,
    statement_id BIGINT NOT NULL REFERENCES statements(id) ON DELETE CASCADE,
    transaction_number INTEGER NOT NULL,      -- "No." column in the PDF
    date DATE NOT NULL,                       -- Operation date
    type VARCHAR(50) NOT NULL,                -- TYPE OF OPERATION
    description TEXT NOT NULL,                -- DESCRIPTION
    amount_cents BIGINT NOT NULL,             -- Amount in cents (negative for debits)
    category VARCHAR(100),                    -- User-assigned category (nullable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Prevent duplicate transaction numbers within the same statement
    UNIQUE(statement_id, transaction_number)
);

-- Indexes for common query patterns
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_type ON transactions(type);
CREATE INDEX idx_transactions_category ON transactions(category);
CREATE INDEX idx_transactions_amount_cents ON transactions(amount_cents);
CREATE INDEX idx_statements_period ON statements(period);

-- Function to automatically update updated_at columns
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_statements_updated_at BEFORE UPDATE ON statements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_transactions_updated_at BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
