DROP INDEX IF EXISTS idx_clicks_event_id;
ALTER TABLE clicks DROP COLUMN IF EXISTS event_id;
