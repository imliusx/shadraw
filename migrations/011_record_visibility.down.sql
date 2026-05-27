DROP INDEX IF EXISTS idx_records_public_gallery;

ALTER TABLE records
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS is_public;
