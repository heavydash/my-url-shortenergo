-- migrations/20251127202000_create_urls_table.sql
-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_urls_original_url_unique ON urls(original_url);

-- +goose Down
DROP TABLE IF EXISTS urls;