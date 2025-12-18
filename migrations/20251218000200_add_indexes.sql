-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_urls_user_id ON urls(user_id) WHERE is_deleted = false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_urls_user_id;
-- +goose StatementEnd
