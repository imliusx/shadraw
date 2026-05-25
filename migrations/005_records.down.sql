DROP TRIGGER IF EXISTS trg_records_updated_at ON records;
DROP INDEX IF EXISTS idx_records_project_id;
DROP INDEX IF EXISTS idx_records_status_waiting;
DROP INDEX IF EXISTS idx_records_user_status_created;
DROP INDEX IF EXISTS uq_records_uuid;
DROP TABLE IF EXISTS records;
