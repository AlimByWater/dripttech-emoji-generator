package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	// Проверяем, является ли сообщение командой /emoji
	if strings.HasPrefix(update.Message.Text, "/emoji") || strings.HasPrefix(update.Message.Caption, "/emoji ") {
		handleEmojiCommand(ctx, b, update)
	}
}

func handleEmojiCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.From.ID < 0 {
		slog.Error("Invalid arguments", slog.String("err", "User ID is not valid"), slog.Int64("user_id", update.Message.From.ID))
		return
	}

	var args string
	// Получаем аргументы команды
	if strings.HasPrefix(update.Message.Text, "/emoji") {
		args = strings.TrimPrefix(update.Message.Text, "/emoji")
	} else if strings.HasPrefix(update.Message.Caption, "/emoji ") {
		args = strings.TrimPrefix(update.Message.Caption, "/emoji ")
	}

	args = strings.TrimSpace(args)
	emojiArgs, err := parseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		return
	}

	if emojiArgs.SetName == "" {
		emojiArgs.SetName = strings.TrimSpace(PackTitleTempl)
	} else {
		if len(emojiArgs.SetName) > TelegramPackLinkAndNameLength-len(PackTitleTempl) {
			emojiArgs.SetName = emojiArgs.SetName[:TelegramPackLinkAndNameLength-len(PackTitleTempl)]
		}
		emojiArgs.SetName = emojiArgs.SetName + " " + PackTitleTempl
	}

	postfix := fmt.Sprintf("%d_%d", update.Message.From.ID, time.Now().Unix())
	emojiArgs.WorkingDir = fmt.Sprintf(outputDirTemplate, postfix)

	emojiArgs.UserID = update.Message.From.ID
	emojiArgs.UserName = update.Message.From.Username

	botInfo, err := b.GetMe(ctx)
	if err != nil {
		slog.Error("Failed to get bot info", slog.String("err", err.Error()))
		return
	}

	if emojiArgs.PackLink == "" {
		emojiArgs.newSet = true
		packName := fmt.Sprintf("%s%d_by_%s", "dript_tech", time.Now().Unix(), botInfo.Username)
		if len(packName) > TelegramPackLinkAndNameLength {
			emojiArgs.PackLink = emojiArgs.PackLink[:len(packName)-TelegramPackLinkAndNameLength]
			packName = fmt.Sprintf("%s_%s", emojiArgs.PackLink, botInfo.Username)
		}
		emojiArgs.PackLink = packName
	} else if strings.Contains(emojiArgs.PackLink, botInfo.Username) {
		// TODO
	} else {
		emojiArgs.newSet = true
		packName := fmt.Sprintf("%s_by_%s", emojiArgs.PackLink, botInfo.Username)
		if len(packName) > TelegramPackLinkAndNameLength {
			emojiArgs.PackLink = emojiArgs.PackLink[:len(packName)-TelegramPackLinkAndNameLength]
			packName = fmt.Sprintf("%s_%s", emojiArgs.PackLink, botInfo.Username)
		}
		emojiArgs.PackLink = packName
	}

	// Создаем рабочую директорию
	if err := os.MkdirAll(emojiArgs.WorkingDir, 0755); err != nil {
		slog.Error("Failed to create working directory", slog.String("err", err.Error()))
		return
	}

	// Скачиваем файл
	fileName, err := downloadFile(ctx, b, update.Message, emojiArgs)
	if err != nil {
		slog.Error("Failed to download file", slog.String("err", err.Error()))
		return
	}

	emojiArgs.DownloadedFile = fileName

	// Обрабатываем видео
	createdFiles, err := processVideo(emojiArgs)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
		removeDirectory(emojiArgs.WorkingDir)
		return
	}

	// Создаем набор стикеров
	stickerSet, err := addEmojis(ctx, b, emojiArgs, createdFiles)
	if err != nil {
		return
	}

	// Создаем сообщение с композицией эмодзи
	messageText := ""
	entities := make([]models.MessageEntity, 0, len(stickerSet.Stickers))

	offset := 0
	for i, sticker := range stickerSet.Stickers {
		if i+1%emojiArgs.Width == 0 {
			messageText += "🎥\n"
		} else {
			messageText += "🎥"
		}

		entities = append(entities, models.MessageEntity{
			Type:          "custom_emoji",
			Offset:        offset,
			Length:        2,
			CustomEmojiID: sticker.CustomEmojiID,
		})

		if i+1%emojiArgs.Width == 0 {
			offset += 3 // 2 for emoji + 1 for newline
		} else {
			offset += 2 // 2 for emoji
		}
	}

	// Отправляем сообщение с эмодзи
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:   update.Message.Chat.ID,
		Text:     messageText,
		Entities: entities,
	})
	if err != nil {
		slog.Error("Failed to send message with emojis", slog.String("err", err.Error()))
	}

	// Отправляем ссылку на пак
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("Ссылка на пак с эмодзи: https://t.me/addemoji/%s", emojiArgs.PackLink),
	})
	if err != nil {
		slog.Error("Failed to send message with emojis pack link", slog.String("err", err.Error()))
	}
}

func parseArgs(arg string) (*EmojiCommand, error) {
	var emojiArgs EmojiCommand

	args := strings.Split(arg, " ")

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		// Определяем стандартный ключ из алиаса
		standardKey, exists := argAlias[key]
		if !exists {
			continue
		}

		// Обрабатываем аргумент в зависимости от стандартного ключа
		switch standardKey {
		case "name":
			emojiArgs.SetName = value
		case "width":
			width, err := strconv.Atoi(value)
			if err != nil {
				return nil, ErrWidthInvalid
			}
			emojiArgs.Width = width
		case "background":
			emojiArgs.BackgroundColor = ColorToHex(value)
		case "link":
			emojiArgs.PackLink = value
		case "iphone":
			emojiArgs.Iphone = value == "true"
		}
	}

	return &emojiArgs, nil
}

func downloadFile(ctx context.Context, b *bot.Bot, m *models.Message, args *EmojiCommand) (string, error) {
	var fileID string
	var fileExt string
	var mimeType string

	if m.Video != nil {
		fileID = m.Video.FileID
		mimeType = m.Video.MimeType
	} else if m.Photo != nil && len(m.Photo) > 0 {
		fileID = m.Photo[len(m.Photo)-1].FileID
		mimeType = "image/jpeg"
	} else if m.Document != nil {
		if slices.Contains(allowedMimeTypes, m.Document.MimeType) {
			fileID = m.Document.FileID
			mimeType = m.Document.MimeType
		} else {
			return "", ErrFileOfInvalidType
		}
	} else if m.ReplyToMessage != nil {
		if m.ReplyToMessage.Video != nil {
			fileID = m.ReplyToMessage.Video.FileID
			mimeType = m.ReplyToMessage.Video.MimeType
		} else if m.ReplyToMessage.Photo != nil && len(m.ReplyToMessage.Photo) > 0 {
			fileID = m.ReplyToMessage.Photo[len(m.ReplyToMessage.Photo)-1].FileID
			mimeType = "image/jpeg"
		} else if m.ReplyToMessage.Document != nil {
			fileID = m.ReplyToMessage.Document.FileID
			mimeType = m.ReplyToMessage.Document.MimeType
		}
	}

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrGetFileFromTelegram, err)
	}
	args.File = file

	switch mimeType {
	case "image/gif":
		fileExt = ".gif"
	case "image/jpeg":
		fileExt = ".jpg"
	case "image/png":
		fileExt = ".png"
	case "image/webp":
		fileExt = ".webp"
	case "video/mp4":
		fileExt = ".mp4"
	case "video/webm":
		fileExt = ".webm"
	case "video/mpeg":
		fileExt = ".mpeg"
	default:
		return "", ErrFileOfInvalidType
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", os.Getenv("BOT_TOKEN"), file.FilePath)
	resp, err := grab.Get(args.WorkingDir+"/saved"+fileExt, fileURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrFileDownloadFailed, err)
	}

	return resp.Filename, nil
}

func addEmojis(ctx context.Context, b *bot.Bot, args *EmojiCommand, emojiFiles []string) (*models.StickerSet, error) {
	if len(emojiFiles) == 0 {
		slog.LogAttrs(ctx, slog.LevelError, "нет файлов для создания набора", args.ToSlogAttributes()...)
		return nil, fmt.Errorf("нет файлов для создания набора")
	}

	if args.newSet {

		var inputStickers []models.InputSticker
		for i, emojiFile := range emojiFiles {
			openFile, err := os.ReadFile(emojiFile)
			if err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "open file", args.ToSlogAttributes(slog.String("err", err.Error()))...)
				return nil, fmt.Errorf("open file: %w", err)
			}

			newSticker, err := b.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
				UserID: args.UserID,
				Sticker: &models.InputFileUpload{
					Filename: emojiFiles[0],
					Data:     bytes.NewReader(openFile),
				},
				StickerFormat: "video",
			})
			if err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "upload sticker", args.ToSlogAttributes(slog.String("err", err.Error()))...)
				return nil, fmt.Errorf("upload sticker: %w", err)
			}

			inputSticker := models.InputSticker{
				Sticker: &models.InputFileString{
					Data: newSticker.FileID,
				},
				Format:    "video",
				EmojiList: []string{"🎥"},
			}

			inputStickers = append(inputStickers, inputSticker)

			if i == 49 {
				break
			}

		}
		// Создаем новый набор стикеров
		_, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
			UserID:      args.UserID,
			Name:        args.PackLink,
			Title:       args.SetName,
			StickerType: "custom_emoji",
			Stickers:    inputStickers,
		})
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "new sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			return nil, fmt.Errorf("не удалось создать пак")
		}

		emojiFiles = emojiFiles[len(inputStickers):]
	}

	for _, emojiFile := range emojiFiles {
		openFile, err := os.ReadFile(emojiFile)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "open file", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			return nil, fmt.Errorf("open file: %w", err)
		}

		newSticker, err := b.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
			UserID: args.UserID,
			Sticker: &models.InputFileUpload{
				Filename: emojiFiles[0],
				Data:     bytes.NewReader(openFile),
			},
			StickerFormat: "video",
		})
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "upload sticker", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			return nil, fmt.Errorf("upload sticker: %w", err)
		}

		inputSticker := models.InputSticker{
			Sticker: &models.InputFileString{
				Data: newSticker.FileID,
			},
			Format:    "video",
			EmojiList: []string{"🎥"},
		}

		_, err = b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
			UserID:  args.UserID,
			Name:    args.PackLink,
			Sticker: inputSticker,
		})
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "add to sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			break
		}
	}

	// Получаем информацию о наборе стикеров
	stickerSet, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "get sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
		return nil, fmt.Errorf("get sticker set: %w", err)
	}

	return stickerSet, nil
}

// ColorToHex converts color names to hex format (0x000000)
func ColorToHex(colorName string) string {
	if colorName == "" {
		return ""
	}
	if hex, exists := colorMap[strings.ToLower(colorName)]; exists {
		return hex
	}

	// Если это уже hex формат или неизвестный цвет, возвращаем как есть
	if strings.HasPrefix(colorName, "0x") {
		return colorName
	}

	return "0x000000" // возвращаем черный по умолчанию
}
