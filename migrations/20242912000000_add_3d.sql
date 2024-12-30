-- +goose Up
-- +goose StatementBegin
CREATE TABLE authors (
                         id SERIAL PRIMARY KEY,
                         channel VARCHAR(255) NOT NULL,
                         logo VARCHAR(255) NOT NULL,
                         name VARCHAR(255) NOT NULL,
                         telegram_user_id INTEGER NOT NULL
);

CREATE TABLE model_objects (
                               id SERIAL PRIMARY KEY,
                               hdri_url VARCHAR(255) NOT NULL,
                               object_url VARCHAR(255) NOT NULL,
                               position FLOAT[] NOT NULL,
                               scale JSONB NOT NULL
);

CREATE TABLE works (
                       id VARCHAR(255) PRIMARY KEY,
                       created_at TIMESTAMP NOT NULL,
                       foreground_color VARCHAR(255) NOT NULL,
                       background_color VARCHAR(255) NOT NULL,
                       in_aquarium BOOLEAN NOT NULL,
                       name VARCHAR(255) NOT NULL,
                       preview_url VARCHAR(255) NOT NULL,
                       object_id INTEGER NOT NULL REFERENCES model_objects(id),
                       authors INTEGER[] NOT NULL REFERENCES authors(id)
);


-- Create indexes


-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS works;
DROP TABLE IF EXISTS model_objects;
DROP TABLE IF EXISTS authors;
-- +goose StatementEnd
