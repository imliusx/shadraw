ALTER TABLE upstream_configs
    ADD COLUMN site_title TEXT NOT NULL DEFAULT 'shadraw'
        CHECK (char_length(btrim(site_title)) BETWEEN 1 AND 64);
