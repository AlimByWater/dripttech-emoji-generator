package main

import (
	"bytes"
	"context"
	"emoji-generator/db"
	"emoji-generator/types"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/cavaliergopher/grab/v3"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var validchatIDs = []int64{-1002400904088, -1002400904088_3, -1002491830452, -1002491830452_3}
var messagesToDelete sync.Map

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

func handleStartCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.Chat.Type == models.ChatTypePrivate {
		// delete message
		msgID, ok := messagesToDelete.LoadAndDelete(update.Message.From.ID)
		if ok {
			//err := b.DeleteMessage(ctx, update.Message.Chat.ID, messagesToDelete.Load(update.Message.From.ID).(int))
			//for i := range validchatIDs {
			//	deleted, _ := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: validchatIDs[i], MessageID: msgID.(int)})
			//	if deleted {
			//		break
			//	}
			//}

			err := userBot.DeleteMessage(ctx, msgID.(int))
			if err != nil {
				slog.Error("Error deleting message", "err", err)
			}
		}
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
		ReplyParameters: &models.ReplyParameters{

			MessageID: update.Message.ID,
			ChatID:    update.Message.Chat.ID,
		},
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
	maxStickerInMessage    = 100
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

	var stickerSet *models.StickerSet
	var emojiMetaRows [][]types.EmojiMeta

	for {
		// Обрабатываем видео
		createdFiles, err := processVideo(emojiArgs)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
			err2 := removeDirectory(emojiArgs.WorkingDir)
			if err2 != nil {
				slog.Error("Failed to remove directory", slog.String("err", err2.Error()), slog.String("dir", emojiArgs.WorkingDir), slog.String("emojiPackLink", emojiArgs.PackLink), slog.Int64("user_id", emojiArgs.UserID))
			}
			sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Ошибка при обработке видео: %s", err.Error()))
			return
		}

		// Создаем набор стикеров
		stickerSet, emojiMetaRows, err = addEmojis(ctx, b, emojiArgs, createdFiles)
		if err != nil {
			if strings.Contains(err.Error(), "PEER_ID_INVALID") || strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "bot was blocked by the user") {
				inlineKeyboard := tgbotapi.NewInlineKeyboardButtonURL("/start", fmt.Sprintf("t.me/%s?start=start", tgbotApi.Self.UserName))
				row := tgbotapi.NewInlineKeyboardRow(inlineKeyboard)
				keyboard := tgbotapi.NewInlineKeyboardMarkup(row)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Чтобы бот мог создать пак \nнажмите кнопку ниже\n↓↓↓↓↓↓↓↓"))
				msg.ReplyMarkup = keyboard
				msg.ParseMode = "MarkdownV2"
				msg.ReplyParameters = tgbotapi.ReplyParameters{
					MessageID: update.Message.ID,
					ChatID:    update.Message.Chat.ID,
				}

				_, err2 := tgbotApi.Send(msg)
				if err2 != nil {
					slog.Error("Failed to send message with emojis", slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID), slog.String("err2", err2.Error()))
				}

				// TODO implement later
				//messagesToDelete.Store(update.Message.From.ID, update.Message.ID)

				return

			}

			if strings.Contains(err.Error(), "STICKER_VIDEO_BIG") {
				emojiArgs.QualityValue++
				continue
			}

			sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Ошибка при создании набора стикеров: %s", err.Error()))
			return
		}

		break
	}

	// Обновляем количество эмодзи в базе данных
	if err := db.Postgres.SetEmojiCount(ctx, emojiPack.ID, len(stickerSet.Stickers)); err != nil {
		slog.Error("Failed to update emoji count",
			slog.String("err", err.Error()),
			slog.String("pack_link", emojiArgs.PackLink),
			slog.Int64("user_id", emojiArgs.UserID))
	}

	// Создаем композицию эмодзи, используя метаданные из emojiMetaRows
	// messageText := ""
	// entities := make([]models.MessageEntity, 0, maxStickerInMessage)
	// offset := 0

	// Собираем все непрозрачные эмодзи
	transparentCount := 0
	newEmojis := make([]types.EmojiMeta, 0, maxStickerInMessage)
	for _, row := range emojiMetaRows {
		for _, emoji := range row {
			newEmojis = append(newEmojis, emoji)
			if emoji.Transparent {
				transparentCount++
			}
		}
	}

	// Выбираем нужные эмодзи
	selectedEmojis := make([]types.EmojiMeta, 0, maxStickerInMessage)
	if emojiArgs.NewSet {
		selectedEmojis = newEmojis
	} else {
		// Выбираем последние 100 эмодзи из пака
		startIndex := len(stickerSet.Stickers) - maxStickerInMessage
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

	// Формируем сообщение из выбранных эмодзи
	// currentRow := 0
	// for i, emoji := range selectedEmojis {
	// 	if emoji.Transparent {
	// 		continue
	// 	} else {
	// 		// Добавляем эмодзи в текст сообщения
	// 		messageText += "🎥"

	// 		// Добавляем ссылку на стикер в entities
	// 		entities = append(entities, models.MessageEntity{
	// 			Type:          models.MessageEntityTypeCustomEmoji,
	// 			Offset:        offset,
	// 			Length:        len("🎥"),
	// 			CustomEmojiID: emoji.DocumentID,
	// 		})
	// 		offset += len("🎥")
	// 	}

	// 	newRow := (i + 1) / emojiArgs.Width
	// 	if newRow != currentRow && i < len(selectedEmojis) {
	// 		messageText += "\n"
	// 		offset += 1
	// 		currentRow = newRow
	// 	}
	// }

	// Отправляем ссылку на пак

	// Отправляем сообщение с эмодзи
	//message := bot.SendMessageParams{
	//	ChatID:          update.Message.Chat.ID,
	//	MessageThreadID: update.Message.MessageThreadID,
	//	Text:            messageText,
	//	Entities:        entities,
	//	ReplyParameters: &models.ReplyParameters{
	//		MessageID: update.Message.ID,
	//		ChatID:    update.Message.Chat.ID,
	//	},
	//}

	topicId := fmt.Sprintf("%d_%d", update.Message.Chat.ID, update.Message.MessageThreadID)
	err = userBot.SendMessageWithEmojis(ctx, topicId, emojiArgs.Width, emojiArgs.PackLink, emojiArgs.RawInitCommand, selectedEmojis, update.Message.ID)
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

func setupEmojiCommand(args *types.EmojiCommand, msg *models.Message) {
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
		args.SetName = strings.TrimSpace(types.PackTitleTempl)
	} else {
		if len(args.SetName) > types.TelegramPackLinkAndNameLength-len(types.PackTitleTempl) {
			args.SetName = args.SetName[:types.TelegramPackLinkAndNameLength-len(types.PackTitleTempl)]
		}
		args.SetName = args.SetName + " " + types.PackTitleTempl
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

func setupPackDetails(ctx context.Context, args *types.EmojiCommand, botInfo *models.User) (*db.EmojiPack, error) {
	if strings.Contains(args.PackLink, botInfo.Username) {
		return handleExistingPack(ctx, args)
	}
	return nil, handleNewPack(args, botInfo)
}

func handleExistingPack(ctx context.Context, args *types.EmojiCommand) (*db.EmojiPack, error) {
	args.NewSet = false
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

func handleNewPack(args *types.EmojiCommand, botInfo *models.User) error {
	args.NewSet = true
	packName := fmt.Sprintf("%s%d_by_%s", "dt", time.Now().Unix(), botInfo.Username)
	if len(packName) > types.TelegramPackLinkAndNameLength {
		args.PackLink = args.PackLink[:len(packName)-types.TelegramPackLinkAndNameLength]
		packName = fmt.Sprintf("%s_%s", args.PackLink, botInfo.Username)
	}
	args.PackLink = packName
	return nil
}

func createDatabaseRecord(ctx context.Context, args *types.EmojiCommand, initialCommand string, botUsername string) (*db.EmojiPack, error) {
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

func prepareWorkingEnvironment(ctx context.Context, b *bot.Bot, update *models.Update, args *types.EmojiCommand) error {
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
	case types.ErrFileOfInvalidType:
		message = "Неподдерживаемый тип файла. Поддерживаются: GIF, JPEG, PNG, WebP, MP4, WebM, MPEG"
	case types.ErrGetFileFromTelegram:
		message = "Не удалось получить файл из Telegram"
	case types.ErrFileDownloadFailed:
		message = "Ошибка при загрузке файла"
	default:
		message = "Ошибка при загрузке файла"
	}
	sendErrorMessage(ctx, b, update, update.Message.Chat.ID, message)
}

func parseArgs(arg string) (*types.EmojiCommand, error) {
	var emojiArgs types.EmojiCommand
	emojiArgs.RawInitCommand = "/emoji " + arg
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
		standardKey, exists := types.ArgAlias[key]
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
		case "name":
			emojiArgs.SetName = strings.TrimSpace(value)
		case "background":
			emojiArgs.BackgroundColor = ColorToHex(value)
		case "background_blend":
			value = strings.ReplaceAll(value, ",", ".")
			emojiArgs.BackgroundBlend = value
		case "background_sim":
			value = strings.ReplaceAll(value, ",", ".")
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

func downloadFile(ctx context.Context, b *bot.Bot, m *models.Message, args *types.EmojiCommand) (string, error) {
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
		if slices.Contains(types.AllowedMimeTypes, m.Document.MimeType) {
			fileID = m.Document.FileID
			mimeType = m.Document.MimeType
		} else {
			return "", types.ErrFileOfInvalidType
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
		return "", fmt.Errorf("%w: %w", types.ErrGetFileFromTelegram, err)
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
		return "", types.ErrFileOfInvalidType
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, file.FilePath)
	resp, err := grab.Get(args.WorkingDir+"/saved"+fileExt, fileURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", types.ErrFileDownloadFailed, err)
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

func addEmojis(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFiles []string) (*models.StickerSet, [][]types.EmojiMeta, error) {
	if err := validateEmojiFiles(emojiFiles); err != nil {
		return nil, nil, err
	}

	// Загружаем все файлы эмодзи и возвращаем их fileIDs и метаданные
	emojiFileIDs, emojiMetaRows, err := uploadEmojiFiles(ctx, b, args, emojiFiles)
	if err != nil {
		return nil, nil, err
	}

	// Создаем набор стикеров
	var set *models.StickerSet
	if args.NewSet {
		set, err = createNewStickerSet(ctx, b, args, emojiFileIDs)
	} else {
		set, err = addToExistingStickerSet(ctx, b, args, emojiFileIDs)
	}
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("addEmojis",
		slog.Int("emojiFileIDS count", len(emojiFileIDs)),
		slog.Int("width", args.Width),
		slog.Int("transparent_spacing", defaultWidth-args.Width),
		slog.Int("stickers in set", len(set.Stickers)))

	// Получаем последние maxStickerInMessage стикеров
	// var lastStickers []models.Sticker
	// if args.NewSet {
	// 	if len(set.Stickers) > maxStickerInMessage {
	// 		lastStickers = set.Stickers[len(set.Stickers)-maxStickerInMessage:]
	// 	} else {
	// 		lastStickers = set.Stickers
	// 	}
	// } else {
	// 	totalStickers := len(prevSet.Stickers) + len(emojiFileIDs)
	// 	if totalStickers > maxStickerInMessage {
	// 		startIdx := totalStickers - maxStickerInMessage
	// 		if startIdx < len(prevSet.Stickers) {
	// 			// Берем часть из prevSet и все новые
	// 			lastStickers = append(prevSet.Stickers[startIdx:], set.Stickers...)
	// 		} else {
	// 			// Берем только новые стикеры
	// 			newStartIdx := startIdx - len(prevSet.Stickers)
	// 			lastStickers = set.Stickers[newStartIdx:]
	// 		}
	// 	} else {
	// 		// Если общее количество меньше maxStickerInMessage, берем все
	// 		lastStickers = append(prevSet.Stickers, set.Stickers...)
	// 	}
	// }

	// Обновляем emojiMetaRows только для последних стикеров
	idx := 0
	if !args.NewSet {
		idx = len(set.Stickers) - len(emojiFileIDs) - 1
	}

	for i := range emojiMetaRows {
		for j := range emojiMetaRows[i] {
			emojiMetaRows[i][j].DocumentID = set.Stickers[idx].CustomEmojiID
			idx++
		}
	}

	return set, emojiMetaRows, nil
}

// createNewStickerSet создает новый набор стикеров
func createNewStickerSet(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
	totalWithTransparent := len(emojiFileIDs)

	if totalWithTransparent > maxStickersTotal {
		return nil, fmt.Errorf("общее количество стикеров (%d) с прозрачными превысит максимум (%d)", totalWithTransparent, maxStickersTotal)
	}

	return createStickerSetWithBatches(ctx, b, args, emojiFileIDs)
}

// addToExistingStickerSet добавляет эмодзи в существующий набор
func addToExistingStickerSet(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
	stickerSet, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
	if err != nil {
		return nil, fmt.Errorf("get sticker set: %w", err)
	}

	// Проверяем, что не превысим лимит
	if len(stickerSet.Stickers)+len(emojiFileIDs) > maxStickersTotal {
		return nil, fmt.Errorf(
			"превышен лимит стикеров в наборе (%d + %d > %d)",
			len(stickerSet.Stickers),
			len(emojiFileIDs),
			maxStickersTotal,
		)
	}

	// Добавляем стикеры батчами
	for i := 0; i < len(emojiFileIDs); i++ {
		_, err := b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
			UserID: args.UserID,
			Name:   args.PackLink,
			Sticker: models.InputSticker{
				Sticker: &models.InputFileString{Data: emojiFileIDs[i]},
				Format:  defaultStickerFormat,
				EmojiList: []string{
					defaultEmojiIcon,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("add sticker to set: %w", err)
		}
	}

	return b.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
}

// createStickerSetWithBatches создает новый набор стикеров
func createStickerSetWithBatches(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
	// Создаем новый набор стикеров с первым стикером

	count := len(emojiFileIDs) // count = 112
	if count > maxStickersInBatch {
		count = maxStickersInBatch // count = 50
	}

	firstBatch := make([]models.InputSticker, count)

	for i := 0; i < count; i++ {
		firstBatch[i] = models.InputSticker{
			Sticker: &models.InputFileString{Data: emojiFileIDs[i]},
			Format:  defaultStickerFormat,
			EmojiList: []string{
				defaultEmojiIcon,
			},
		}
	}

	ok, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
		UserID:      args.UserID,
		Name:        args.PackLink,
		Title:       args.SetName,
		StickerType: "custom_emoji",
		Stickers:    firstBatch,
	})
	if err != nil {
		slog.Debug("new sticker set FAILED", slog.String("name", args.PackLink), slog.String("error", err.Error()))
		return nil, fmt.Errorf("create sticker set: %w", err)
	}

	if !ok {
		return nil, fmt.Errorf("failed to create sticker set")
	}

	emojiFileIDs = emojiFileIDs[count:]

	// Добавляем оставшиеся стикеры по одному
	for _, emojiFile := range emojiFileIDs {
		ok, err := b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
			UserID: args.UserID,
			Name:   args.PackLink,
			Sticker: models.InputSticker{
				Sticker: &models.InputFileString{Data: emojiFile},
				Format:  defaultStickerFormat,
				EmojiList: []string{
					defaultEmojiIcon,
				},
			},
		})

		if err != nil {
			return nil, fmt.Errorf("add sticker to set: %w", err)
		}

		if !ok {
			return nil, fmt.Errorf("failed to add sticker to set")
		}
	}

	// Получаем финальное состояние набора
	set, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
	if err != nil {
		return nil, fmt.Errorf("get sticker set: %w", err)
	}

	return set, nil
}

// validateEmojiFiles проверяет корректность входных файлов
func validateEmojiFiles(emojiFiles []string) error {
	if len(emojiFiles) == 0 {
		return fmt.Errorf("нет файлов для создания набора")
	}

	if len(emojiFiles) > maxStickersTotal {
		return fmt.Errorf("слишком много файлов для создания набора (максимум %d)", maxStickersTotal)
	}

	return nil
}

func prepareTransparentData(width int) ([]byte, error) {
	// Подготавливаем прозрачные стикеры если нужно
	transparentSpacing := defaultWidth - width
	transparentData, err := os.ReadFile("transparent.webm")
	if err != nil || transparentSpacing <= 0 {
		return nil, nil
	} else if transparentSpacing > 0 {
		transparentData, err = os.ReadFile("transparent.webm")
		if err != nil {
			return nil, fmt.Errorf("open transparent file: %w", err)
		}
		return transparentData, nil
	}

	return nil, nil
}

// uploadEmojiFiles загружает все файлы эмодзи и возвращает их fileIDs и метаданные
func uploadEmojiFiles(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFiles []string) ([]string, [][]types.EmojiMeta, error) {
	slog.Debug("uploading emoji stickers", slog.Int("count", len(emojiFiles)))

	totalEmojis := len(emojiFiles)
	rows := (totalEmojis + args.Width - 1) / args.Width // Округляем вверх
	emojiMetaRows := make([][]types.EmojiMeta, rows)

	// Подготавливаем прозрачный стикер только если он нужен
	var transparentData []byte
	var err error
	if args.Width < defaultWidth {
		transparentData, err = prepareTransparentData(args.Width)
		if err != nil {
			return nil, nil, err
		}
	}

	for i := range emojiMetaRows {
		emojiMetaRows[i] = make([]types.EmojiMeta, defaultWidth) // Инициализируем каждый ряд с полной шириной
	}

	// Сначала загружаем все эмодзи и заполняем метаданные
	for i, emojiFile := range emojiFiles {
		fileData, err := os.ReadFile(emojiFile)
		if err != nil {
			return nil, nil, fmt.Errorf("open emoji file: %w", err)
		}

		fileID, err := uploadSticker(ctx, b, args.UserID, emojiFile, fileData)
		if err != nil {
			return nil, nil, err
		}

		// Вычисляем позицию в сетке
		row := i / args.Width
		col := i % args.Width

		// Вычисляем отступы для центрирования
		totalPadding := defaultWidth - args.Width
		leftPadding := totalPadding / 2
		if totalPadding > 0 && totalPadding%2 != 0 {
			// Для нечетного количества отступов, слева меньше на 1
			leftPadding = (totalPadding - 1) / 2
		}

		// Загружаем прозрачные эмодзи слева только если нужно
		if args.Width < defaultWidth {
			for j := 0; j < leftPadding; j++ {
				if emojiMetaRows[row][j].FileID == "" {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, nil, fmt.Errorf("upload transparent sticker: %w", err)
					}
					emojiMetaRows[row][j] = types.EmojiMeta{
						FileID:      transparentFileID,
						FileName:    "transparent.webm",
						Transparent: true,
					}
				}
			}
		}

		// Записываем метаданные эмодзи
		pos := col
		if args.Width < defaultWidth {
			pos = col + leftPadding
		}
		emojiMetaRows[row][pos] = types.EmojiMeta{
			FileID:      fileID,
			FileName:    emojiFile,
			Transparent: false,
		}

		// Загружаем прозрачные эмодзи справа только если нужно
		if args.Width < defaultWidth {
			for j := col + leftPadding + 1; j < defaultWidth; j++ {
				if emojiMetaRows[row][j].FileID == "" {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, nil, fmt.Errorf("upload transparent sticker: %w", err)
					}
					emojiMetaRows[row][j] = types.EmojiMeta{
						FileID:      transparentFileID,
						FileName:    "transparent.webm",
						Transparent: true,
					}
				}
			}
		}
	}

	// Теперь собираем emojiFileIDs в правильном порядке
	emojiFileIDs := make([]string, 0, rows*defaultWidth)
	for i := range emojiMetaRows {
		for j := range emojiMetaRows[i] {
			if emojiMetaRows[i][j].FileID != "" {
				emojiFileIDs = append(emojiFileIDs, emojiMetaRows[i][j].FileID)
			}
		}
	}

	return emojiFileIDs, emojiMetaRows, nil
}

func ColorToHex(colorName string) string {
	if colorName == "" {
		return ""
	}
	if hex, exists := types.ColorMap[strings.ToLower(colorName)]; exists {
		return hex
	}

	// Если это уже hex формат или неизвестный цвет, возвращаем как есть
	if strings.HasPrefix(colorName, "0x") {
		return colorName
	}

	return "0x000000" // возвращаем черный по умолчанию
}
