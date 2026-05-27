CREATE TABLE record_favorites (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    record_id   BIGINT NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_record_favorites_user_record
ON record_favorites (user_id, record_id);

CREATE INDEX idx_record_favorites_user_created
ON record_favorites (user_id, created_at DESC);

CREATE INDEX idx_record_favorites_record_id
ON record_favorites (record_id);

CREATE TRIGGER trg_record_favorites_updated_at
BEFORE UPDATE ON record_favorites
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
