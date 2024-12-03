-- +goose Up
-- +goose StatementBegin
CREATE TABLE emoji_packs (
    id SERIAL PRIMARY KEY,
    creator_id BIGINT NOT NULL,
    pack_name VARCHAR(255) NOT NULL,
    file_url TEXT NOT NULL,
    pack_link TEXT,
    initial_command TEXT,
    bot VARCHAR(255) NOT NULL,
    emoji_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);


-- Create indexes
CREATE INDEX idx_emoji_packs_creator ON emoji_packs(creator_id);
CREATE INDEX idx_emoji_packs_bot ON emoji_packs(bot);
CREATE INDEX idx_emoji_packs_pack_link ON emoji_packs(pack_link);

-- Grant table privileges
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO drip_tech;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO drip_tech;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS emoji_packs;
\c postgres;
DROP DATABASE IF EXISTS drip_tech;
DROP USER IF EXISTS drip_tech;
-- +goose StatementEnd
