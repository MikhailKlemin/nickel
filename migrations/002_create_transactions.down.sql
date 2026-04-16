DROP TRIGGER IF EXISTS update_transactions_updated_at ON transactions;
DROP TABLE IF EXISTS transactions;
-- Function NOT dropped here because it belongs to migration 001
