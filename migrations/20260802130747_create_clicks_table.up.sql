CREATE TABLE IF NOT EXISTS clicks (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(16) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    ip          TEXT,
    user_agent  TEXT
);

CREATE INDEX IF NOT EXISTS idx_clicks_code_occurred_at ON clicks (code, occurred_at);
