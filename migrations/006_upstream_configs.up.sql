CREATE TABLE upstream_configs (
    id                  SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    base_url            TEXT NOT NULL DEFAULT '',
    api_key_cipher      BYTEA,
    enabled_models      JSONB NOT NULL DEFAULT '[]'::jsonb,
    worker_concurrency  SMALLINT NOT NULL DEFAULT 4
                          CHECK (worker_concurrency BETWEEN 1 AND 16),
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_upstream_configs_updated_at
BEFORE UPDATE ON upstream_configs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
