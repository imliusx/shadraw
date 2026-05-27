ALTER TABLE records
    ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN published_at TIMESTAMPTZ;

CREATE INDEX idx_records_public_gallery
ON records (published_at DESC, id DESC)
WHERE is_public = TRUE AND status = 'completed' AND image_path IS NOT NULL;
