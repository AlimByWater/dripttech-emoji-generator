package db

import (
	"context"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestPostgres_PermissionsByChannelID(t *testing.T) {
	require.NoError(t, godotenv.Load("/Users/admin/go/src/emoji-generator/.env"))
	os.Setenv("ENV", "test")
	err := Init()
	require.NoError(t, err)

	permission, err := Postgres.PermissionsByChannelID(context.Background(), -1001901113896)
	require.NoError(t, err)

	t.Log(permission)
}

func TestPostgres_GetEmojiPacksByCreator(t *testing.T) {
	require.NoError(t, godotenv.Load("/Users/admin/go/src/emoji-generator/.env"))
	os.Setenv("ENV", "test")
	err := Init()
	require.NoError(t, err)

	packs, err := Postgres.GetEmojiPacksByCreator(context.Background(), 1162899041, false)
	require.NoError(t, err)
	t.Log(len(packs))
}
