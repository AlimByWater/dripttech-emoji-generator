package main

import (
	"bytes"
	"context"
	"emoji-generator/db"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/cavaliergopher/grab/v3"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var validchatIDs = []int64{-1002400904088, -1002400904088_3}

func validchatID(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		for _, chatID := range validchatIDs {
			if chatID == update.Message.Chat.ID {
				next(ctx, b, update)
				return
			}
		}
	}
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	for i, chatID := range validchatIDs {
		if chatID == update.Message.Chat.ID {
			break
		}
		if i == len(validchatIDs)-1 {
			return
		}
	}

	// Проверяем, является ли сообщение командой
	if strings.HasPrefix(update.Message.Text, "/emoji") {
		handleEmojiCommand(ctx, b, update)
	} else if update.Message.Text == "/emoji" {
		handleEmojiCommand(ctx, b, update)
	} else if strings.HasPrefix(update.Message.Caption, "/emoji ") {
		handleEmojiCommand(ctx, b, update)
	} else if update.Message.Caption == "/emoji " {
		handleEmojiCommand(ctx, b, update)
	} else if update.Message.Text == "/info" {
		handleInfoCommand(ctx, b, update)
	}
}

func handleInfoCommand(ctx context.Context, b *bot.Bot, update *models.Update) {

	infoText := `🤖 Бот для создания эмодзи-паков из картинок/видео/GIF

Отправьте медиафайл с командой /emoji и опциональными параметрами в формате param=[value]:

Параметры:
• width=[N] или w=[N] - ширина нарезки (по умолчанию 8). Чем меньше ширина, тем крупнее эмодзи
• background=[цвет] или b=[цвет] - цвет фона, который будет вырезан из изображения. Поддерживаются:
  - HEX формат: b=[0x00FF00]
  - Названия: b=[black], b=[white], b=[pink], b=[green]
• b_sim=[число] - порог схожести цвета с фоном (0-1, по умолчанию 0.1)
• b_blend=[число] - использовать смешивание цветов для удаления фона (0-1, по умолчанию 0.1)
• link=[ссылка] или l=[ссылка] - добавить эмодзи в существующий пак (должен быть создан вами)
• iphone=[true] или i=[true] - оптимизация размера под iPhone`

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   infoText,
	})
	if err != nil {
		slog.Error("Failed to send info message", slog.String("err", err.Error()))
	}
}

const (
	defaultWidth           = 8
	defaultBackgroundSim   = "0.1"
	defaultBackgroundBlend = "0.1"
	defaultStickerFormat   = "video"
	defaultEmojiIcon       = "🎥"
	maxStickersInBatch     = 50
	maxStickersTotal       = 200
)

func handleEmojiCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Extract command arguments
	args := extractCommandArgs(update.Message)
	emojiArgs, err := parseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, err.Error())
		return
	}

	// Setup command defaults and working environment
	setupEmojiCommand(emojiArgs, update.Message)

	// Get bot info and setup pack details
	botInfo, err := b.GetMe(ctx)
	if err != nil {
		slog.Error("Failed to get bot info", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "Не удалось получить информацию о боте")
		return
	}

	emojiPack, err := setupPackDetails(ctx, emojiArgs, botInfo)
	if err != nil {
		slog.Error("Failed to setup pack details", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "пак с подобной ссылкой не найден")
		return
	}

	pgBot, err := db.Postgres.GetBotByName(ctx, botInfo.Username)
	if err != nil {
		slog.Error("Failed to get bot by name", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "Не удалось получить информацию о боте")
		return
	}

	if emojiPack == nil {
		// Create database record
		emojiPack, err = createDatabaseRecord(ctx, emojiArgs, args, pgBot.Name)
		if err != nil {
			slog.Error("Failed to log emoji command",
				slog.String("err", err.Error()),
				slog.String("pack_link", emojiArgs.PackLink),
				slog.Int64("user_id", emojiArgs.UserID))
			sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "Не удалось создать запись в базе данных")
			return
		}
	}

	// Create working directory and download file
	if err := prepareWorkingEnvironment(ctx, b, update, emojiArgs); err != nil {
		handleDownloadError(ctx, b, update, err)
		return
	}

	// Обрабатываем видео
	createdFiles, err := processVideo(emojiArgs)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
		removeDirectory(emojiArgs.WorkingDir)
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Ошибка при обработке видео: %s", err.Error()))
		return
	}

	// Создаем набор стикеров
	stickerSet, err := addEmojis(ctx, b, emojiArgs, createdFiles)
	if err != nil {
		if strings.Contains(err.Error(), "PEER_ID_INVALID") || strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "bot was blocked by the user") {
			inlineKeyboard := tgbotapi.NewInlineKeyboardButtonURL("init", fmt.Sprintf("t.me/%s?start=start", tgbotApi.Self.UserName))
			row := tgbotapi.NewInlineKeyboardRow(inlineKeyboard)
			keyboard := tgbotapi.NewInlineKeyboardMarkup(row)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Чтобы бот мог создавать пак на ваш аккаунт, вам нужно инициировать взаимодействие с ботом"))
			msg.ReplyMarkup = keyboard
			msg.ReplyParameters = tgbotapi.ReplyParameters{
				MessageID: update.Message.ID,
				ChatID:    update.Message.Chat.ID,
			}

			_, err2 := tgbotApi.Send(msg)
			if err2 != nil {
				slog.Error("Failed to send message with emojis", slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID), slog.String("err2", err2.Error()))
			}

			return

		}
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Ошибка при создании набора стикеров: %s", err.Error()))
		return
	}

	// Обновляем количество эмодзи в базе данных
	if err := db.Postgres.SetEmojiCount(ctx, emojiPack.ID, len(stickerSet.Stickers)); err != nil {
		slog.Error("Failed to update emoji count",
			slog.String("err", err.Error()),
			slog.String("pack_link", emojiArgs.PackLink),
			slog.Int64("user_id", emojiArgs.UserID))
	}

	// Создаем сообщение с композицией эмодзи
	messageText := ""
	entities := make([]models.MessageEntity, 0, len(stickerSet.Stickers))

	offset := 0
	emojiIndex := 0
	for i, sticker := range stickerSet.Stickers {
		if i+1%emojiArgs.Width == 0 {
			messageText += "🎥\n"
		} else {
			messageText += "🎥"
		}

		// Добавляем в entities только реальные эмодзи (не прозрачные)
		pos := i % defaultWidth
		isTransparent := false

		if emojiArgs.Width < defaultWidth {
			leftPadding := (defaultWidth - emojiArgs.Width) / 2
			rightPadding := defaultWidth - emojiArgs.Width - leftPadding
			isTransparent = pos < leftPadding || pos >= (defaultWidth-rightPadding)
		}

		if !isTransparent {
			entities = append(entities, models.MessageEntity{
				Type:          "custom_emoji",
				Offset:        offset,
				Length:        2,
				CustomEmojiID: sticker.CustomEmojiID,
			})
			emojiIndex++
		}

		if i+1%emojiArgs.Width == 0 {
			offset += 3 // 2 for emoji + 1 for newline
		} else {
			offset += 2 // 2 for emoji
		}
	}

	// Отправляем ссылку на пак
	topicId := fmt.Sprintf("%d_%d", update.Message.Chat.ID, update.Message.MessageThreadID)
	//_, err = b.SendMessage(ctx, &bot.SendMessageParams{
	//	ChatID:          update.Message.Chat.ID,
	//	MessageThreadID: update.Message.MessageThreadID,
	//	Text:            fmt.Sprintf("Ссылка на пак с эмодзи: https://t.me/addemoji/%s", emojiArgs.PackLink),
	//})
	message := bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            fmt.Sprintf("Ссылка на пак с эмодзи: https://t.me/addemoji/%s", emojiArgs.PackLink),
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
			ChatID:    update.Message.Chat.ID,
		},
	}
	err = userBot.SendMessage(ctx, topicId, message)
	if err != nil {
		slog.Error("Failed to send message with emojis pack link", slog.String("err", err.Error()))
	}

	// Отправляем сообщение с эмодзи
	message = bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            messageText,
		Entities:        entities,
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
			ChatID:    update.Message.Chat.ID,
		},
	}

	err = userBot.SendMessageWithEmojis(ctx, topicId, emojiArgs.Width, message)
	if err != nil {
		slog.Error("Failed to send message with emojis", slog.String("err", err.Error()), slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID))
	}
}

func sendErrorMessage(ctx context.Context, b *bot.Bot, u *models.Update, chatID int64, errToSend string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ReplyParameters: &models.ReplyParameters{
			MessageID: u.Message.ID,
			ChatID:    u.Message.Chat.ID,
		},
		ChatID: chatID,
		Text:   fmt.Sprintf("Возникла ошибка: %s", errToSend),
	})
	if err != nil {
		slog.Error("Failed to send error message", slog.String("err", err.Error()))
	}
}

func extractCommandArgs(msg *models.Message) string {
	var args string
	if strings.HasPrefix(msg.Text, "/emoji") {
		args = strings.TrimPrefix(msg.Text, "/emoji")
	} else if strings.HasPrefix(msg.Caption, "/emoji ") {
		args = strings.TrimPrefix(msg.Caption, "/emoji ")
	}
	return strings.TrimSpace(args)
}

func setupEmojiCommand(args *EmojiCommand, msg *models.Message) {
	// Set default values
	if args.Width == 0 {
		args.Width = defaultWidth
	}
	if args.BackgroundSim == "" {
		args.BackgroundSim = defaultBackgroundSim
	}
	if args.BackgroundBlend == "" {
		args.BackgroundBlend = defaultBackgroundBlend
	}
	if args.SetName == "" {
		args.SetName = strings.TrimSpace(PackTitleTempl)
	} else {
		if len(args.SetName) > TelegramPackLinkAndNameLength-len(PackTitleTempl) {
			args.SetName = args.SetName[:TelegramPackLinkAndNameLength-len(PackTitleTempl)]
		}
		args.SetName = args.SetName + " " + PackTitleTempl
	}

	// Setup working directory and user info
	postfix := fmt.Sprintf("%d_%d", msg.From.ID, time.Now().Unix())
	args.WorkingDir = fmt.Sprintf(outputDirTemplate, postfix)

	args.UserID = msg.From.ID
	if msg.From.IsBot || msg.From.ID < 0 {
		args.UserID = 251636949
	}
	args.UserName = msg.From.Username
}

func setupPackDetails(ctx context.Context, args *EmojiCommand, botInfo *models.User) (*db.EmojiPack, error) {
	if strings.Contains(args.PackLink, botInfo.Username) {
		return handleExistingPack(ctx, args)
	}
	return nil, handleNewPack(args, botInfo)
}

func handleExistingPack(ctx context.Context, args *EmojiCommand) (*db.EmojiPack, error) {
	args.newSet = false
	if strings.Contains(args.PackLink, "t.me/addemoji/") {
		splited := strings.Split(args.PackLink, ".me/addemoji/")
		args.PackLink = strings.TrimSpace(splited[len(splited)-1])
	}

	pack, err := db.Postgres.GetEmojiPackByPackLink(ctx, args.PackLink)
	if err != nil {
		return nil, err
	}
	args.SetName = ""
	return pack, nil
}

func handleNewPack(args *EmojiCommand, botInfo *models.User) error {
	args.newSet = true
	packName := fmt.Sprintf("%s%d_by_%s", "drip_tech", time.Now().Unix(), botInfo.Username)
	if len(packName) > TelegramPackLinkAndNameLength {
		args.PackLink = args.PackLink[:len(packName)-TelegramPackLinkAndNameLength]
		packName = fmt.Sprintf("%s_%s", args.PackLink, botInfo.Username)
	}
	args.PackLink = packName
	return nil
}

func createDatabaseRecord(ctx context.Context, args *EmojiCommand, initialCommand string, botUsername string) (*db.EmojiPack, error) {
	emojiPack := &db.EmojiPack{
		CreatorID:      args.UserID,
		PackName:       args.SetName,
		PackLink:       &args.PackLink,
		InitialCommand: &initialCommand,
		BotName:        botUsername,
		EmojiCount:     0,
	}
	return db.Postgres.LogEmojiCommand(ctx, emojiPack)
}

func prepareWorkingEnvironment(ctx context.Context, b *bot.Bot, update *models.Update, args *EmojiCommand) error {
	if err := os.MkdirAll(args.WorkingDir, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	fileName, err := downloadFile(ctx, b, update.Message, args)
	if err != nil {
		return err
	}
	args.DownloadedFile = fileName
	return nil
}

func handleDownloadError(ctx context.Context, b *bot.Bot, update *models.Update, err error) {
	slog.Error("Failed to download file", slog.String("err", err.Error()))
	var message string
	switch err {
	case ErrFileOfInvalidType:
		message = "Неподдерживаемый тип файла. Поддерживаются: GIF, JPEG, PNG, WebP, MP4, WebM, MPEG"
	case ErrGetFileFromTelegram:
		message = "Не удалось получить файл из Telegram"
	case ErrFileDownloadFailed:
		message = "Ошибка при загрузке файла"
	default:
		message = "Ошибка при загрузке файла"
	}
	sendErrorMessage(ctx, b, update, update.Message.Chat.ID, message)
}

func parseArgs(arg string) (*EmojiCommand, error) {
	var emojiArgs EmojiCommand

	// Разбиваем строку на части, учитывая как пробелы, так и возможные аргументы в квадратных скобках
	var args []string
	parts := strings.Fields(arg)

	for _, part := range parts {
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			// Извлекаем имя параметра и значение из формата param=[value]
			paramStart := strings.Index(part, "=")
			if paramStart == -1 {
				continue
			}

			key := strings.ToLower(part[:paramStart])
			// Извлекаем значение между [ и ]
			valueStart := strings.Index(part, "[")
			valueEnd := strings.Index(part, "]")
			if valueStart == -1 || valueEnd == -1 || valueStart >= valueEnd {
				slog.Error("Invalid arguments", slog.String("err", "Invalid format"), slog.String("arg", part))
				continue
			}
			value := part[valueStart+1 : valueEnd]
			args = append(args, key+"="+value)
		} else {
			// Обрабатываем обычный формат param=value
			args = append(args, part)
		}
	}

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue // Пропускаем несуществующий аргумент
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		// Определяем стандартный ключ из алиаса
		standardKey, exists := argAlias[key]
		if !exists {
			continue // Пропускаем несуществующий аргумент
		}

		// Обрабатываем аргумент в зависимости от стандартного ключа
		switch standardKey {
		case "width":
			width, err := strconv.Atoi(value)
			if err != nil {
				continue
			}
			emojiArgs.Width = width
		case "background":
			emojiArgs.BackgroundColor = ColorToHex(value)
		case "background_blend":
			emojiArgs.BackgroundBlend = value
		case "background_sim":
			emojiArgs.BackgroundSim = value
		case "link":
			emojiArgs.PackLink = value
		case "iphone":
			if value != "true" && value != "false" {
				continue
			}
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

// Функция для обработки ошибок с повторными попытками
func handleTelegramError(err error) (int, error) {
	if err == nil {
		return 0, nil
	}

	if strings.Contains(err.Error(), "retry_after") {
		// Извлекаем время ожидания из ошибки
		parts := strings.Split(err.Error(), "retry_after ")
		slog.Debug("handleTelegramError", slog.String("err", err.Error()), slog.String("parts", strings.Join(parts, " /// ")))
		if len(parts) >= 2 {
			if waitTime, parseErr := strconv.Atoi(strings.TrimSpace(parts[1])); parseErr == nil {
				return waitTime + 5, nil
			}
		}
	}
	return 0, err
}

func uploadSticker(ctx context.Context, b *bot.Bot, userID int64, filename string, data []byte) (string, error) {
	for {
		newSticker, err := b.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
			UserID: userID,
			Sticker: &models.InputFileUpload{
				Filename: filename,
				Data:     bytes.NewReader(data),
			},
			StickerFormat: defaultStickerFormat,
		})

		if err != nil {
			slog.Debug("upload sticker FAILED",
				slog.String("file", filename),
				slog.String("err", err.Error()))
		} else {
			slog.Debug("upload sticker SUCCESS",
				slog.String("file", filename))
		}

		if waitTime, err := handleTelegramError(err); err != nil {
			return "", fmt.Errorf("upload sticker: %w", err)
		} else if waitTime > 0 {
			slog.Info("waiting before retry", "seconds", waitTime)
			time.Sleep(time.Duration(waitTime) * time.Second)
			continue
		}

		return newSticker.FileID, nil
	}
}

func addEmojis(ctx context.Context, b *bot.Bot, args *EmojiCommand, emojiFiles []string) (*models.StickerSet, error) {
	if len(emojiFiles) == 0 {
		return nil, fmt.Errorf("нет файлов для создания набора")
	}

	if len(emojiFiles) > maxStickersTotal {
		return nil, fmt.Errorf("слишком много файлов для создания набора (максимум %d)", maxStickersTotal)
	}

	// Подготавливаем прозрачные стикеры для существующего набора
	transparentSpacing := defaultWidth - args.Width
	var transparentData []byte
	if transparentSpacing > 0 {
		var err error
		transparentData, err = os.ReadFile("transparent.webm")
		if err != nil {
			return nil, fmt.Errorf("open transparent file: %w", err)
		}
	}

	if args.newSet {
		totalEmojis := len(emojiFiles)
		rows := (totalEmojis + args.Width - 1) / args.Width
		totalWithTransparent := rows * defaultWidth
		slog.Debug("addEmojis",
			slog.Int("totalemojis", totalEmojis),
			slog.Int("rows", rows),
			slog.Int("width", args.Width),
			slog.Int("transparent_spacing", transparentSpacing),
			slog.Int("totalWithTransparent", totalWithTransparent))

		if totalWithTransparent > maxStickersTotal {
			return nil, fmt.Errorf("общее количество стикеров (%d) с прозрачными превышает максимум (%d)", totalWithTransparent, maxStickersTotal)
		}

		// Сначала загрузим все эмодзи стикеры
		slog.Debug("uploading emoji stickers", slog.Int("count", len(emojiFiles)))
		emojiFileIDs := make([]string, len(emojiFiles))
		for i, emojiFile := range emojiFiles {
			fileData, err := os.ReadFile(emojiFile)
			if err != nil {
				return nil, fmt.Errorf("open emoji file: %w", err)
			}

			fileID, err := uploadSticker(ctx, b, args.UserID, emojiFile, fileData)
			if err != nil {
				return nil, err
			}
			emojiFileIDs[i] = fileID

			// Небольшая задержка между загрузками
			time.Sleep(time.Millisecond * 200)
		}

		// Создаем новый набор стикеров
		inputStickers := make([]models.InputSticker, 0, totalWithTransparent)
		emojiIndex := 0

		if transparentSpacing > 0 {
			leftPadding := transparentSpacing / 2
			rightPadding := transparentSpacing - leftPadding

			for emojiIndex < len(emojiFileIDs) {
				pos := emojiIndex % args.Width

				// Добавляем левые прозрачные стикеры в начале строки
				if pos == 0 {
					for i := 0; i < leftPadding; i++ {
						transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
						if err != nil {
							return nil, fmt.Errorf("upload transparent sticker: %w", err)
						}

						inputStickers = append(inputStickers, models.InputSticker{
							Sticker:   &models.InputFileString{Data: transparentFileID},
							Format:    defaultStickerFormat,
							EmojiList: []string{defaultEmojiIcon},
						})
						time.Sleep(time.Millisecond * 200)
					}
				}

				// Добавляем эмодзи
				inputStickers = append(inputStickers, models.InputSticker{
					Sticker:   &models.InputFileString{Data: emojiFileIDs[emojiIndex]},
					Format:    defaultStickerFormat,
					EmojiList: []string{defaultEmojiIcon},
				})
				emojiIndex++

				// Добавляем правые прозрачные стикеры в конце строки
				if pos == args.Width-1 || emojiIndex == len(emojiFileIDs) {
					for i := 0; i < rightPadding; i++ {
						transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
						if err != nil {
							return nil, fmt.Errorf("upload transparent sticker: %w", err)
						}

						inputStickers = append(inputStickers, models.InputSticker{
							Sticker:   &models.InputFileString{Data: transparentFileID},
							Format:    defaultStickerFormat,
							EmojiList: []string{defaultEmojiIcon},
						})
						time.Sleep(time.Millisecond * 200)
					}
				}
			}
		} else {
			// Когда не нужны прозрачные стикеры, просто добавляем все эмодзи
			for _, fileID := range emojiFileIDs {
				inputStickers = append(inputStickers, models.InputSticker{
					Sticker:   &models.InputFileString{Data: fileID},
					Format:    defaultStickerFormat,
					EmojiList: []string{defaultEmojiIcon},
				})
			}
		}

		// Создаем новый набор стикеров с первыми 50 стикерами
		idx := len(inputStickers)
		if idx > maxStickersInBatch {
			idx = maxStickersInBatch
		}

		var stickerSet *models.StickerSet
		for {
			ok, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
				UserID:      args.UserID,
				Name:        args.PackLink,
				Title:       args.SetName,
				StickerType: "custom_emoji",
				Stickers:    inputStickers[:idx],
			})

			if err != nil {
				slog.Debug("new sticker set FAILED", slog.String("name", args.PackLink), slog.String("err", err.Error()))
			} else {
				slog.Debug("new sticker set SUCCESS", slog.String("name", args.PackLink), slog.Bool("ok", ok))
			}

			if waitTime, err := handleTelegramError(err); err != nil {
				return nil, fmt.Errorf("create sticker set: %w", err)
			} else if waitTime > 0 {
				slog.Info("waiting before retry", "seconds", waitTime)
				time.Sleep(time.Duration(waitTime) * time.Second)
				continue
			}
			break
		}

		// Добавляем оставшиеся стикеры по одному
		if len(inputStickers) > maxStickersInBatch {
			remaining := len(inputStickers) - maxStickersInBatch
			slog.Debug("adding remaining stickers",
				slog.Int("from", maxStickersInBatch),
				slog.Int("remaining", remaining),
				slog.Int("total", len(inputStickers)))

			for i := maxStickersInBatch; i < len(inputStickers); i++ {
				sticker := inputStickers[i]
				slog.Debug("adding sticker",
					slog.Int("index", i),
					slog.Int("progress", i-maxStickersInBatch+1),
					slog.Int("total_remaining", remaining))

				for {
					_, err := b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
						UserID:  args.UserID,
						Name:    args.PackLink,
						Sticker: sticker,
					})

					if err != nil {
						slog.Debug("add sticker FAILED",
							slog.String("name", args.PackLink),
							slog.Int("index", i),
							slog.String("err", err.Error()))
					} else {
						slog.Debug("add sticker SUCCESS",
							slog.String("name", args.PackLink),
							slog.Int("index", i))
					}

					if waitTime, err := handleTelegramError(err); err != nil {
						return nil, fmt.Errorf("add sticker to set: %w", err)
					} else if waitTime > 0 {
						slog.Info("waiting before retry", "seconds", waitTime)
						time.Sleep(time.Duration(waitTime) * time.Second)
						continue
					}
					break
				}
			}
		}

		// Получаем финальное состояние набора
		for {
			set, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
				Name: args.PackLink,
			})
			if err != nil {
				if waitTime, err := handleTelegramError(err); err != nil {
					return nil, fmt.Errorf("get sticker set: %w", err)
				} else if waitTime > 0 {
					slog.Info("waiting before retry", "seconds", waitTime)
					time.Sleep(time.Duration(waitTime) * time.Second)
					continue
				}
				return nil, fmt.Errorf("get sticker set: %w", err)
			}
			stickerSet = set
			break
		}

		return stickerSet, nil
	}

	////////////////////////////////////////////////////////////////////////////////////
	// Для существующего набора
	if !args.newSet {
		// Сначала загрузим все эмодзи стикеры
		slog.Debug("uploading emoji stickers", slog.Int("count", len(emojiFiles)))
		emojiFileIDs := make([]string, len(emojiFiles))
		for i, emojiFile := range emojiFiles {
			fileData, err := os.ReadFile(emojiFile)
			if err != nil {
				return nil, fmt.Errorf("open emoji file: %w", err)
			}

			fileID, err := uploadSticker(ctx, b, args.UserID, emojiFile, fileData)
			if err != nil {
				return nil, err
			}
			emojiFileIDs[i] = fileID

			// Небольшая задержка между загрузками
			time.Sleep(time.Millisecond * 200)
		}

		// Получаем текущее состояние набора стикеров один раз в начале
		currentSet, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
			Name: args.PackLink,
		})
		if err != nil {
			return nil, fmt.Errorf("get current sticker set: %w", err)
		}

		// Вычисляем количество прозрачных стикеров слева и справа
		leftPadding := transparentSpacing / 2
		rightPadding := transparentSpacing - leftPadding

		// Отслеживаем текущую позицию локально
		currentPosition := len(currentSet.Stickers)
		emojiIndex := 0

		for emojiIndex < len(emojiFileIDs) {
			pos := currentPosition % defaultWidth

			// Добавляем левые прозрачные стикеры в начале строки
			if pos == 0 && transparentSpacing > 0 {
				for i := 0; i < leftPadding; i++ {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, fmt.Errorf("upload transparent sticker: %w", err)
					}

					inputSticker := models.InputSticker{
						Sticker:   &models.InputFileString{Data: transparentFileID},
						Format:    defaultStickerFormat,
						EmojiList: []string{defaultEmojiIcon},
					}

					for {
						_, err = b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
							UserID:  args.UserID,
							Name:    args.PackLink,
							Sticker: inputSticker,
						})

						if err != nil {
							if waitTime, retryErr := handleTelegramError(err); retryErr != nil {
								return nil, fmt.Errorf("add transparent sticker: %w", retryErr)
							} else if waitTime > 0 {
								slog.Info("waiting before retry", "seconds", waitTime)
								time.Sleep(time.Duration(waitTime) * time.Second)
								continue
							}
							return nil, fmt.Errorf("add transparent sticker: %w", err)
						}
						break
					}
					time.Sleep(time.Millisecond * 200)
					currentPosition++
				}
			}

			// Добавляем эмодзи
			inputSticker := models.InputSticker{
				Sticker:   &models.InputFileString{Data: emojiFileIDs[emojiIndex]},
				Format:    defaultStickerFormat,
				EmojiList: []string{defaultEmojiIcon},
			}

			for {
				_, err = b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
					UserID:  args.UserID,
					Name:    args.PackLink,
					Sticker: inputSticker,
				})

				if err != nil {
					if waitTime, retryErr := handleTelegramError(err); retryErr != nil {
						return nil, fmt.Errorf("add emoji sticker: %w", retryErr)
					} else if waitTime > 0 {
						slog.Info("waiting before retry", "seconds", waitTime)
						time.Sleep(time.Duration(waitTime) * time.Second)
						continue
					}
					return nil, fmt.Errorf("add emoji sticker: %w", err)
				}
				break
			}
			time.Sleep(time.Millisecond * 200)
			currentPosition++
			emojiIndex++

			// Проверяем, нужно ли добавить правые прозрачные стикеры
			pos = currentPosition % defaultWidth
			if pos == args.Width && transparentSpacing > 0 {
				for i := 0; i < rightPadding; i++ {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, fmt.Errorf("upload transparent sticker: %w", err)
					}

					inputSticker := models.InputSticker{
						Sticker:   &models.InputFileString{Data: transparentFileID},
						Format:    defaultStickerFormat,
						EmojiList: []string{defaultEmojiIcon},
					}

					for {
						_, err = b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
							UserID:  args.UserID,
							Name:    args.PackLink,
							Sticker: inputSticker,
						})

						if err != nil {
							if waitTime, retryErr := handleTelegramError(err); retryErr != nil {
								return nil, fmt.Errorf("add transparent sticker: %w", retryErr)
							} else if waitTime > 0 {
								slog.Info("waiting before retry", "seconds", waitTime)
								time.Sleep(time.Duration(waitTime) * time.Second)
								continue
							}
							return nil, fmt.Errorf("add transparent sticker: %w", err)
						}
						break
					}
					time.Sleep(time.Millisecond * 200)
					currentPosition++
				}
			}
		}

		// Получаем финальное состояние набора
		finalSet, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
			Name: args.PackLink,
		})
		if err != nil {
			return nil, fmt.Errorf("get final sticker set: %w", err)
		}

		return finalSet, nil
	}

	return nil, fmt.Errorf("unknown sticker set type")
}

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
