package processing

import (
	"emoji-generator/types"
	"github.com/go-telegram/bot/models"
)

func GenerateEmojiMessage(emojiMetaRows [][]types.EmojiMeta, stickerSet *models.StickerSet, emojiArgs *types.EmojiCommand) []types.EmojiMeta {
	transparentCount := 0
	newEmojis := make([]types.EmojiMeta, 0, types.MaxStickerInMessage)
	for _, row := range emojiMetaRows {
		for _, emoji := range row {
			newEmojis = append(newEmojis, emoji)
			if emoji.Transparent {
				transparentCount++
			}
		}
	}

	// Выбираем нужные эмодзи
	selectedEmojis := make([]types.EmojiMeta, 0, types.MaxStickerInMessage)
	if emojiArgs.NewSet {
		selectedEmojis = newEmojis
	} else {
		// Выбираем последние 100 эмодзи из пака
		startIndex := len(stickerSet.Stickers) - types.MaxStickerInMessage
		if startIndex < 0 {
			startIndex = 0
		}
		for _, sticker := range stickerSet.Stickers[startIndex:] {
			selectedEmojis = append(selectedEmojis, types.EmojiMeta{
				FileID:      sticker.FileID,
				DocumentID:  sticker.CustomEmojiID,
				Transparent: false,
			})
		}
	}

	return selectedEmojis
}
