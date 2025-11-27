-- migrations/20251127202000_create_urls_table.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS urls (
                                    uuid TEXT PRIMARY KEY,
                                    short_url TEXT UNIQUE NOT NULL,
                                    original_url TEXT NOT NULL,
                                    user_id TEXT,
                                    created_at TIMESTAMPTZ DEFAULT NOW()
    );

CREATE INDEX IF NOT EXISTS idx_short_url ON urls(short_url);

-- +goose Down
DROP TABLE IF EXISTS urls;