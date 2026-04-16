DROP TRIGGER IF EXISTS update_statements_updated_at ON statements;
DROP TABLE IF EXISTS statements;
-- Drop the function after all dependent triggers are removed (migration 002 down runs first)
DROP FUNCTION IF EXISTS update_updated_at_column();
