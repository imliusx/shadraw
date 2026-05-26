ALTER TABLE records
    ADD COLUMN image_params JSONB;

UPDATE records
SET image_params = jsonb_build_object(
    'size',
    CASE
        WHEN ratio IN ('3:4', '9:16') THEN '1024x1536'
        WHEN ratio IN ('4:3', '16:9') THEN '1536x1024'
        WHEN ratio = '1:1' THEN '1024x1024'
        ELSE 'auto'
    END,
    'quality',
    CASE
        WHEN pixels = '4K' THEN 'high'
        WHEN pixels = '1K' THEN 'low'
        ELSE 'medium'
    END,
    'n', 1,
    'output_format', 'png'
)
WHERE image_params IS NULL;
