package db

import (
	"context"
	"fmt"
)

// CreateEmojiPack creates a new emoji pack record
func (p *postgres) CreateEmojiPack(ctx context.Context, pack *EmojiPack) (*EmojiPack, error) {
	query := `
INSERT INTO emoji_packs (
creator_id, pack_name, telegram_file_id, pack_link, initial_command, bot_name, emoji_count
) VALUES (
$1, $2, $3, $4, $5, $6, $7
) RETURNING id, created_at, updated_at`

	err := p.db.QueryRowContext(ctx, query, pack.CreatorID, pack.PackName, pack.TelegramFileID, pack.PackLink, pack.InitialCommand, pack.BotName, pack.EmojiCount).
		Scan(&pack.ID, &pack.CreatedAt, &pack.UpdatedAt)
	if err != nil {
		return pack, fmt.Errorf("failed to create emoji pack: %w", err)
	}

	return pack, nil
}

// GetEmojiPackByID retrieves an emoji pack by its ID
func (p *postgres) GetEmojiPackByID(ctx context.Context, id int64) (*EmojiPack, error) {
	var pack EmojiPack
	query := `SELECT * FROM emoji_packs WHERE id = $1`

	if err := p.db.GetContext(ctx, &pack, query, id); err != nil {
		return nil, fmt.Errorf("failed to get emoji pack: %w", err)
	}

	return &pack, nil
}

// GetEmojiPacksByCreator retrieves all emoji packs created by a specific user
func (p *postgres) GetEmojiPacksByCreator(ctx context.Context, creatorID int64, botName string, includeBlank bool) ([]*EmojiPack, error) {
	var packs []*EmojiPack
	var query string
	if includeBlank {
		query = `SELECT * FROM emoji_packs WHERE creator_id = $1 and bot_name = $2 and deleted = false ORDER BY created_at DESC`
	} else {
		query = `SELECT * FROM emoji_packs WHERE creator_id = $1 AND pack_link is not null AND bot_name = $2 and deleted = false ORDER BY created_at DESC`
	}

	if err := p.db.SelectContext(ctx, &packs, query, creatorID, botName); err != nil {
		return nil, fmt.Errorf("failed to get emoji packs by creator: %w", err)
	}

	return packs, nil
}

// GetEmojiPackByPackLink retrieves an emoji pack by its pack link
func (p *postgres) GetEmojiPackByPackLink(ctx context.Context, packLink string) (*EmojiPack, error) {
	var pack EmojiPack
	query := `SELECT * FROM emoji_packs WHERE pack_link = $1 and deleted = false`

	if err := p.db.GetContext(ctx, &pack, query, packLink); err != nil {
		return nil, fmt.Errorf("failed to get emoji pack by pack link: %w", err)
	}

	return &pack, nil
}

func (p *postgres) SetDeletedPack(ctx context.Context, packLink string) error {
	query := `UPDATE emoji_packs SET deleted = true WHERE pack_link = $1`
	_, err := p.db.ExecContext(ctx, query, packLink)
	if err != nil {
		return fmt.Errorf("failed to delete emoji pack: %w", err)
	}
	return nil

}

func (p *postgres) UnsetDeletedPack(ctx context.Context, packLink string) error {
	query := `UPDATE emoji_packs SET deleted = false WHERE pack_link = $1`
	_, err := p.db.ExecContext(ctx, query, packLink)
	if err != nil {
		return fmt.Errorf("failed to delete emoji pack: %w", err)
	}
	return nil

}
