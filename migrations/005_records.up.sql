CREATE TABLE records (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id        BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    prompt            TEXT NOT NULL,
    model             TEXT NOT NULL,
    ratio             TEXT NOT NULL,
    pixels            TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'waiting'
                        CHECK (status IN ('waiting','running','completed','failed')),
    favorite          BOOLEAN NOT NULL DEFAULT FALSE,
    image_path        TEXT,
    error             TEXT,
    reference_images  JSONB,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_records_uuid ON records (uuid);
CREATE INDEX idx_records_user_status_created ON records (user_id, status, created_at DESC);
CREATE INDEX idx_records_status_waiting ON records (id) WHERE status = 'waiting';
CREATE INDEX idx_records_project_id ON records (project_id) WHERE project_id IS NOT NULL;

CREATE TRIGGER trg_records_updated_at
BEFORE UPDATE ON records
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
