CREATE TABLE record_images (
    id          BIGSERIAL PRIMARY KEY,
    record_id   BIGINT NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    position    BIGINT NOT NULL,
    image_path  TEXT NOT NULL,
    mime        TEXT NOT NULL,
    extension   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_record_images_record_position UNIQUE (record_id, position)
);

CREATE INDEX idx_record_images_record_id ON record_images (record_id, position);

INSERT INTO record_images (record_id, position, image_path, mime, extension)
SELECT
    id,
    1,
    image_path,
    CASE
        WHEN image_params->>'output_format' = 'jpeg' THEN 'image/jpeg'
        WHEN image_params->>'output_format' = 'webp' THEN 'image/webp'
        ELSE 'image/png'
    END,
    CASE
        WHEN image_params->>'output_format' = 'jpeg' THEN 'jpg'
        WHEN image_params->>'output_format' = 'webp' THEN 'webp'
        ELSE 'png'
    END
FROM records
WHERE image_path IS NOT NULL AND image_path <> '';

CREATE TRIGGER trg_record_images_updated_at
BEFORE UPDATE ON record_images
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
