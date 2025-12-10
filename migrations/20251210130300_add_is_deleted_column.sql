-- +goose Up
-- +goose StatementBegin
ALTER TABLE urls ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE urls DROP COLUMN IF EXISTS is_deleted;
-- +goose StatementEnd
