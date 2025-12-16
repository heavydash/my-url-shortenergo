-- +goose Up
-- +goose StatementBegin
ALTER TABLE urls ALTER COLUMN user_id TYPE uuid USING user_id::uuid;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE urls ALTER COLUMN user_id TYPE text USING user_id::text;
-- +goose StatementEnd
