-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS urls (
                                    uuid         TEXT PRIMARY KEY,
                                    short_url    TEXT UNIQUE NOT NULL,
                                    original_url TEXT NOT NULL,
                                    created_at   TIMESTAMPTZ DEFAULT NOW()
    );

CREATE INDEX IF NOT EXISTS idx_short_url ON urls(short_url);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS urls;
-- +goose StatementEnd
