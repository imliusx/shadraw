-- Reverse 001_common: drop helper and extensions if no other consumer.
-- Extensions are guarded to avoid breaking unrelated databases.

DROP FUNCTION IF EXISTS set_updated_at();
-- We intentionally do NOT drop extensions; they are cheap and may be used elsewhere.
