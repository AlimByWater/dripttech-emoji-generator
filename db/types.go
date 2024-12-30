package db

import (
	"time"
)

type EmojiPack struct {
	ID             int64     `db:"id"`
	CreatorID      int64     `db:"creator_id"`
	PackName       string    `db:"pack_name"`
	TelegramFileID string    `db:"telegram_file_id"` // telegram_file_id        string    `db:"file_url"`
	PackLink       *string   `db:"pack_link"`
	InitialCommand *string   `db:"initial_command"`
	BotName        string    `db:"bot_name"`
	EmojiCount     int       `db:"emoji_count"`
	Completed      bool      `db:"completed"`
	Deleted        bool      `json:"deleted"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type Bot struct {
	Name      string    `db:"name"`
	Token     string    `db:"token"`
	Blocked   bool      `db:"blocked"`
	CreatedAt time.Time `db:"created_at"`
}
