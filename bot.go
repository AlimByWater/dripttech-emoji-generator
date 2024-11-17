package main

import (
	"context"
	userbot "emoji-generator/mtproto"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	userBot *userbot.User
}

func NewBot(userBot *userbot.User) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		return nil, err
	}

	return &Bot{api: bot, userBot: userBot}, nil
}

func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	slog.Info("Bot started")

	for {
		select {
		case update := <-updates:
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Minute)
			b.handleUpdate(updateCtx, update)
			updateCancel()
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("panic recovered: ", slog.Any("err", p))
		}
	}()

	if update.Message == nil {
		return
	}

	if update.Message.IsCommand() {
		if update.Message.Command() == "emoji" {
			//if update.FromChat().ID != 0 {}
			b.commandEmoji(update)
		}
	}
}

const tries = 5

func (b *Bot) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	// implement rate limiting
	var err error
	var resp *tgbotapi.APIResponse
	for i := 0; i < tries; i++ {
		resp, err = b.api.Request(c)
		if err != nil {
			return nil, err
		}
		if resp.Ok {
			return resp, nil
		}
		time.Sleep(time.Second)
	}

	slog.Error("Request failed", slog.String("err", err.Error()), slog.Any("request", c))
	return nil, err
}

func (b *Bot) commandEmoji(update tgbotapi.Update) {

	ctx := context.Background()

	if update.SentFrom().ID < 0 {
		// TODO отправить сообщение в чат с ошибкой
		slog.Error("Invalid arguments", slog.String("err", "User ID is not valid"), slog.Int64("user_id", update.SentFrom().ID))
		return
	}
	// ---------------- парсим аргументы команды ----------------

	args := update.Message.CommandArguments()

	emojiArgs, err := b.parseArgs(args)
	if err != nil {
		// TODO отправить сообщение в чат с ошибкой
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
	emojiArgs.UserName = update.Message.From.UserName

	if emojiArgs.PackLink == "" {
		emojiArgs.newSet = true
		packName := fmt.Sprintf("%s%d_by_%s", "dript_tech", time.Now().Unix(), b.api.Self.UserName)
		if len(packName) > TelegramPackLinkAndNameLength {
			emojiArgs.PackLink = emojiArgs.PackLink[:len(packName)-TelegramPackLinkAndNameLength]
			packName = fmt.Sprintf("%s_%s", emojiArgs.PackLink, b.api.Self.UserName)
		}
		emojiArgs.PackLink = packName
	} else if strings.Contains(emojiArgs.PackLink, b.api.Self.UserName) {
		// TODO
	} else {
		emojiArgs.newSet = true
		packName := fmt.Sprintf("%s_by_%s", emojiArgs.PackLink, b.api.Self.UserName)
		if len(packName) > TelegramPackLinkAndNameLength {
			emojiArgs.PackLink = emojiArgs.PackLink[:len(packName)-TelegramPackLinkAndNameLength]
			packName = fmt.Sprintf("%s_%s", emojiArgs.PackLink, b.api.Self.UserName)
		}
		emojiArgs.PackLink = packName
	}

	// --------------------------- Скачиваем файл ---------------------------

	if err := os.MkdirAll(emojiArgs.WorkingDir, 0755); err != nil {
		slog.Error("Failed to create working directory", slog.String("err", err.Error()))
		return
	}

	fileName, err := b.downloadFile(update.Message, emojiArgs)
	if err != nil {
		// TODO отправить сообщение в чат с ошибкой
		slog.Error("Failed to download file", slog.String("err", err.Error()))
		return
	}

	emojiArgs.DownloadedFile = fileName

	// --------------------------- Обрабатываем видео ---------------------------
	createdFiles, err := processVideo(emojiArgs)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
		clearDirectory(emojiArgs.WorkingDir)
		return
	}

	//defer func() {
	//	if err := clearDirectory(emojiArgs.WorkingDir); err != nil {
	//		slog.LogAttrs(ctx, slog.LevelError, "Ошибка при очистке директории: %v", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
	//	}
	//}()
	// --------------------------- Создаем набор стикеров ---------------------------
	stickerSet, err := b.addEmojis(ctx, emojiArgs, createdFiles)
	if err != nil {
		// TODO отправить сообщение в чат с ошибкой
		return
	}

	// Создаем сообщение с композицией эмодзи
	//rows := len(createdFiles)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
	messageText := ""

	// Добавляем эмодзи в текст сообщения и формируем entities
	for i := range stickerSet.Stickers {
		if i+1%emojiArgs.Width == 0 {
			messageText += "🎥\n"
		} else {
			messageText += "🎥"
		}
		// Placeholder символ, который будет заменен на кастомный эмодзи
	}

	msg.Text = messageText
	msg.Entities = make([]tgbotapi.MessageEntity, 0, len(stickerSet.Stickers))

	// # # # #
	// # # # #
	// # # # #

	offset := 0
	// Добавляем entity для каждого эмодзи
	for i, sticker := range stickerSet.Stickers {
		msg.Entities = append(msg.Entities, tgbotapi.MessageEntity{
			Type:          "custom_emoji",
			Offset:        offset,
			Length:        2,
			CustomEmojiID: sticker.CustomEmojiID,
		})

		if i+1%emojiArgs.Width == 0 {
			offset += 2
		}
	}

	// Отправляем сообщение
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("Failed to send message with emojis", slog.String("err", err.Error()))
	}

	linkMsg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Ссылка на пак с эмодзи: https://t.me/addemoji/%s", emojiArgs.PackLink))
	if _, err := b.api.Send(linkMsg); err != nil {
		slog.Error("Failed to send message with emojis pack link", slog.String("err", err.Error()))
	}
}

func (b *Bot) parseArgs(arg string) (*EmojiCommand, error) {
	// parse arguments in format "name=value width=value background=value"
	var emojiArgs EmojiCommand

	args := strings.Split(arg, " ")

	for _, arg := range args {
		if strings.HasPrefix(arg, "name=") {
			emojiArgs.SetName = strings.TrimPrefix(arg, "name=")
		} else if strings.HasPrefix(arg, "width=") {
			width, err := strconv.Atoi(strings.TrimPrefix(arg, "width="))
			if err != nil {
				return nil, ErrWidthInvalid
			}
			emojiArgs.Width = width
		} else if strings.HasPrefix(arg, "background=") {
			emojiArgs.BackgroundColor = strings.TrimPrefix(arg, "background=")
		} else if strings.HasPrefix(arg, "link=") {
			emojiArgs.PackLink = strings.TrimPrefix(arg, "link=")
		} else if strings.HasPrefix(arg, "iphone=") {
			opt := strings.TrimPrefix(arg, "iphone=")
			if opt == "true" {
				emojiArgs.Iphone = true
			}
		}
	}

	return &emojiArgs, nil
}

func (b *Bot) downloadFile(m *tgbotapi.Message, args *EmojiCommand) (string, error) {
	var fileID string
	var fileExt string
	var mimeType string

	if m.Video != nil {
		fileID = m.Video.FileID
		mimeType = m.Video.MimeType
	} else if m.Photo != nil {

		fileID = m.Photo[len(m.Photo)-1].FileID
		mimeType = "image/jpeg"
	} else if m.Document != nil {
		if slices.Contains(allowedMimeTypes, m.Document.MimeType) {
			fileID = m.Document.FileID
			mimeType = m.Document.MimeType
		} else {
			return "", ErrFileOfInvalidType
		}
	} else if m.ReplyToMessage.Video != nil {
		fileID = m.ReplyToMessage.Video.FileID
		mimeType = m.ReplyToMessage.Video.MimeType
	} else if m.ReplyToMessage.Photo != nil {
		fileID = m.ReplyToMessage.Photo[len(m.ReplyToMessage.Photo)-1].FileID
		mimeType = "image/jpeg"
	} else if m.ReplyToMessage.Document != nil {
		fileID = m.ReplyToMessage.Document.FileID
		mimeType = m.ReplyToMessage.Document.MimeType
	}

	file, err := b.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
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

	resp, err := grab.Get(args.WorkingDir+"/saved"+fileExt, file.Link(b.api.Token))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrFileDownloadFailed, err)
	}

	return resp.Filename, nil
}
