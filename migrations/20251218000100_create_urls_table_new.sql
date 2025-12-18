-- +goose Up
-- +goose StatementBegin
CREATE TABLE urls (
                      id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
                      short_url text UNIQUE NOT NULL,
                      original_url text NOT NULL,
                      user_id uuid NOT NULL,
                      is_deleted boolean DEFAULT false,
                      created_at timestamptz DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE urls;
-- +goose StatementEnd
