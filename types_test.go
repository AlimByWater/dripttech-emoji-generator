package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTruncate(t *testing.T) {
	t.Log(2 % 4)
}

func TestEmojiCommand(t *testing.T) {
	var emojiArgs EmojiCommand

	emojiArgs.SetName = "test"
	emojiArgs.Width = 2
	emojiArgs.BackgroundColor = "#ffffff"
	emojiArgs.PackLink = "pack"
	emojiArgs.Iphone = true

	bot, err := NewBot(nil)
	require.NoError(t, err)

	u := tgbotapi.Update{
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID: 251636949,
			},
		},
		SentFrom: &tgbotapi.User{
			ID: 123,
		},
		//Message: &tgbotapi.Message{
		//	Chat: &tgbotapi.Chat{
		//		ID: 123,
	}
	bot.commandEmoji(tgbotapi.Update{})
}
