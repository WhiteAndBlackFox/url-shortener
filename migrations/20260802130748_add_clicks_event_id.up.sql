ALTER TABLE clicks ADD COLUMN event_id VARCHAR(32);

-- Nullable + a plain UNIQUE index: Postgres treats NULLs as distinct from
-- each other in a unique index, so pre-existing rows without an event_id
-- (inserted before this migration) don't conflict with anything. Every row
-- written going forward always sets it (see internal/gateway/transport/http/link.go).
CREATE UNIQUE INDEX IF NOT EXISTS idx_clicks_event_id ON clicks (event_id);
