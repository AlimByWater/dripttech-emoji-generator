package db

import (
	"context"
	"database/sql"
	"emoji-generator/types"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var Postgres *postgres

type postgres struct {
	db *sqlx.DB
}

func Init() error {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_NAME"), "disable",
	)

	db, err := sqlx.Connect("postgres", connString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	p := &postgres{db: db}
	Postgres = p

	return nil
}

func (p *postgres) Shutdown() error {
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	return nil
}

// CreateEmojiPack creates a new emoji pack record
func (p *postgres) CreateEmojiPack(ctx context.Context, pack *EmojiPack) (*EmojiPack, error) {
	query := `
INSERT INTO emoji_packs (
creator_id, pack_name, file_url, pack_link, initial_command, bot_name, emoji_count
) VALUES (
$1, $2, $3, $4, $5, $6, $7
) RETURNING id, created_at, updated_at`

	err := p.db.QueryRowContext(ctx, query, pack.CreatorID, pack.PackName, pack.FileURL, pack.PackLink, pack.InitialCommand, pack.BotName, pack.EmojiCount).
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
func (p *postgres) GetEmojiPacksByCreator(ctx context.Context, creatorID int64) ([]*EmojiPack, error) {
	var packs []*EmojiPack
	query := `SELECT * FROM emoji_packs WHERE creator_id = $1 ORDER BY created_at DESC`

	if err := p.db.SelectContext(ctx, &packs, query, creatorID); err != nil {
		return nil, fmt.Errorf("failed to get emoji packs by creator: %w", err)
	}

	return packs, nil
}

// GetEmojiPackByPackLink retrieves an emoji pack by its pack link
func (p *postgres) GetEmojiPackByPackLink(ctx context.Context, packLink string) (*EmojiPack, error) {
	var pack EmojiPack
	query := `SELECT * FROM emoji_packs WHERE pack_link = $1`

	if err := p.db.GetContext(ctx, &pack, query, packLink); err != nil {
		return nil, fmt.Errorf("failed to get emoji pack by pack link: %w", err)
	}

	return &pack, nil
}

func (p *postgres) GetBotByID(ctx context.Context, id int64) (*Bot, error) {
	var bot Bot
	query := `SELECT * FROM bots WHERE id = $1`

	if err := p.db.GetContext(ctx, &bot, query, id); err != nil {
		return nil, fmt.Errorf("failed to get bot by id: %w", err)
	}

	return &bot, nil
}

func (p *postgres) GetBotByName(ctx context.Context, name string) (*Bot, error) {
	var bot Bot
	query := `SELECT * FROM bots WHERE name = $1`

	if err := p.db.GetContext(ctx, &bot, query, name); err != nil {
		return nil, fmt.Errorf("failed to get bot by name: %w", err)
	}

	return &bot, nil
}

// SetEmojiCount updates the emoji count for a specific pack
func (p *postgres) SetEmojiCount(ctx context.Context, packID int64, count int) error {
	query := `UPDATE emoji_packs SET emoji_count = $1, updated_at = NOW() WHERE id = $2`

	result, err := p.db.ExecContext(ctx, query, count, packID)
	if err != nil {
		return fmt.Errorf("failed to update emoji count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("emoji pack with id %d not found", packID)
	}

	return nil
}

func (p *postgres) UserExists(ctx context.Context, userID int64, botName string) (bool, error) {
	var exists bool
	err := p.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM emoji_packs WHERE creator_id = $1 AND bot_name = $2)`, userID, botName)
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists: %w", err)
	}

	return exists, nil
}

// LogEmojiCommand logs the execution of an emoji command
func (p *postgres) LogEmojiCommand(ctx context.Context, pack *EmojiPack) (*EmojiPack, error) {
	if pack.PackLink == nil {
		return pack, fmt.Errorf("pack link is required")
	}

	// Check if pack already exists
	var existingID int64
	err := p.db.GetContext(ctx, &existingID, `SELECT id FROM emoji_packs WHERE pack_link = $1`, *pack.PackLink)
	if err == nil {
		// Pack exists, update it
		query := `
			UPDATE emoji_packs 
			SET emoji_count = $1, updated_at = NOW() 
			WHERE id = $2`

		if _, err := p.db.ExecContext(ctx, query, pack.EmojiCount, existingID); err != nil {
			return pack, fmt.Errorf("failed to update existing pack: %w", err)
		}
		return pack, nil
	}

	// Pack doesn't exist, create new one
	return p.CreateEmojiPack(ctx, pack)
}

func (p *postgres) HasPermissionForPrivateEmojiGeneration(ctx context.Context, userID int64) (bool, error) {
	var can bool
	err := p.db.GetContext(ctx, &can, `SELECT private_generation FROM permissions WHERE user_id = $1`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if user exists: %w", err)
	}

	return can, nil
}

func (p *postgres) Permissions(ctx context.Context, userID int64) (types.Permissions, error) {
	var permissions []types.Permissions
	q := `SELECT user_id, private_generation, pack_name_without_prefix, use_in_groups, use_by_channel_name, channel_ids FROM permissions WHERE user_id = $1`
	rows, err := p.db.QueryContext(ctx, q, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Permissions{}, nil
		}
		return types.Permissions{}, fmt.Errorf("failed to query permissions by user: %w", err)
	}

	for rows.Next() {
		var permission types.Permissions
		var channelIDs pq.Int64Array
		err := rows.Scan(&permission.UserID, &permission.PrivateGeneration, &permission.PackNameWithoutPrefix, &permission.UseInGroups, &permission.UseByChannelName, &channelIDs)
		if err != nil {
			return types.Permissions{}, fmt.Errorf("failed to scan permissions: %w", err)
		}
		permission.ChannelIDs = channelIDs
		permissions = append(permissions, permission)
	}

	if len(permissions) > 0 {
		return permissions[0], nil
	}

	return types.Permissions{}, nil
}

func (p *postgres) PermissionsByChannelID(ctx context.Context, channelID int64) (types.Permissions, error) {
	var permissions []types.Permissions
	q := `SELECT user_id, private_generation, pack_name_without_prefix, use_in_groups, use_by_channel_name, channel_ids
FROM permissions
WHERE ARRAY[$1]::bigint[] <@ channel_ids`

	rows, err := p.db.QueryContext(ctx, q, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Permissions{}, nil
		}
		return types.Permissions{}, fmt.Errorf("failed to query permissions by channel: %w", err)
	}

	for rows.Next() {
		var permission types.Permissions
		var channelIDs pq.Int64Array
		err := rows.Scan(&permission.UserID, &permission.PrivateGeneration, &permission.PackNameWithoutPrefix, &permission.UseInGroups, &permission.UseByChannelName, &channelIDs)
		if err != nil {
			return types.Permissions{}, fmt.Errorf("failed to scan permissions: %w", err)
		}
		permission.ChannelIDs = channelIDs
		permissions = append(permissions, permission)
	}

	if len(permissions) >= 1 {
		return permissions[0], nil
	}

	return types.Permissions{}, nil
}
