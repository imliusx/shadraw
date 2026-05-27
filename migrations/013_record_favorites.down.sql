DROP TRIGGER IF EXISTS trg_record_favorites_updated_at ON record_favorites;
DROP INDEX IF EXISTS idx_record_favorites_record_id;
DROP INDEX IF EXISTS idx_record_favorites_user_created;
DROP INDEX IF EXISTS uq_record_favorites_user_record;
DROP TABLE IF EXISTS record_favorites;
