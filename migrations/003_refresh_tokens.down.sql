DROP TRIGGER IF EXISTS trg_refresh_tokens_updated_at ON refresh_tokens;
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
DROP INDEX IF EXISTS uq_refresh_tokens_token_hash;
DROP TABLE IF EXISTS refresh_tokens;
