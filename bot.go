package main

import (
	"bytes"
	"context"
	"database/sql"
	"emoji-generator/db"
	"emoji-generator/queue"
	"emoji-generator/types"
	"errors"
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

var (
	stickerQueue     = queue.New()
	validchatIDs     = []string{"-1002400904088_3", "-1002491830452_3"}
	messagesToDelete sync.Map
)

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	if update.Message.Chat.Type == models.ChatTypePrivate {
		if strings.Contains(update.Message.Text, "start") {
			handleStartCommand(ctx, b, update)
		}
	}

	for i, chatID := range validchatIDs {
		if chatID == fmt.Sprintf("%d_%d", update.Message.Chat.ID, update.Message.MessageThreadID) {
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
		me, err := b.GetMe(ctx)
		if err != nil {
			slog.Error("Failed to get bot info", slog.String("err", err.Error()))
			_, err2 := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Возникла ошибка при получении информации о боте",
			})
			slog.Error("Failed to send message to DM", slog.String("err", err2.Error()), slog.Int64("user_id", update.Message.From.ID))
			return
		}

		exist, err := db.Postgres.UserExists(ctx, update.Message.From.ID, me.Username)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			slog.Error("Failed to check if user exists", slog.String("err", err.Error()))
			_, err2 := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Возникла ошибка при получении информации из БД. Попробуйте позже",
			})
			slog.Error("Failed to send message to DM", slog.String("err", err2.Error()), slog.Int64("user_id", update.Message.From.ID))
			return
		}

		if !exist {
			err = createBlankDatabaseRecord(ctx, me.Username, update.Message.From.ID)
			if err != nil {
				slog.Error("Failed to create blank database record", slog.String("err", err.Error()))
				_, err2 := b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: update.Message.Chat.ID,
					Text:   "Возникла ошибка при создании базы данных",
				})
				slog.Error("Failed to send message to DM", slog.String("err", err2.Error()), slog.Int64("user_id", update.Message.From.ID))
				return
			}

			// delete message
			msgID, ok := messagesToDelete.LoadAndDelete(update.Message.From.ID)
			if ok {
				for i := range validchatIDs {
					deleted, _ := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: validchatIDs[i], MessageID: msgID.(int)})
					if deleted {
						break
					}
				}
			}
		}

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Можешь делать паки",
		})
		if err != nil {
			slog.Error("Failed to send message to DM", slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID), slog.String("err", err.Error()))
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
	defaultBackgroundSim   = "0.1"
	defaultBackgroundBlend = "0.1"
	defaultStickerFormat   = "video"
	defaultEmojiIcon       = "⭐️"
)

func handleEmojiCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	fmt.Println(update.Message.From.ID, update.Message.From.Username)
	if update.Message.From.IsBot || update.Message.From.ID == 1087968824 {
		update.Message.From.ID = 251636949
		update.Message.From.Username = "no_alim"
		update.Message.From.IsBot = false
	}

	// Extract command arguments
	args := extractCommandArgs(update.Message)
	emojiArgs, err := parseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, err.Error())
		return
	}

	if update.Message.From.IsBot || update.Message.From.ID < 0 {
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "Создать пак можно только с личного аккаунта")
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

	exist, err := db.Postgres.UserExists(ctx, update.Message.From.ID, botInfo.Username)
	if err != nil {
		slog.Error("Failed to check if user exists", slog.Int64("user_id", update.Message.From.ID), slog.String("err", err.Error()))
		sendInitMessage(update.Message.Chat.ID, update.Message.ID)
		return
	}
	if !exist {
		sendInitMessage(update.Message.Chat.ID, update.Message.ID)
		return
	}

	emojiPack, err := setupPackDetails(ctx, emojiArgs, botInfo)
	if err != nil {
		slog.Error("Failed to setup pack details", slog.String("err", err.Error()))
		sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "пак с подобной ссылкой не найден")
		return
	}

	// Create working directory and download file
	if err := prepareWorkingEnvironment(ctx, b, update, emojiArgs); err != nil {
		handleDownloadError(ctx, b, update, err)
		return
	}

	if emojiPack == nil {
		// Create database record
		emojiPack, err = createDatabaseRecord(ctx, emojiArgs, args, botInfo.Username)
		if err != nil {
			slog.Error("Failed to log emoji command",
				slog.String("err", err.Error()),
				slog.String("pack_link", emojiArgs.PackLink),
				slog.Int64("user_id", emojiArgs.UserID))
			sendErrorMessage(ctx, b, update, update.Message.Chat.ID, "Не удалось создать запись в базе данных")
			return
		}
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
				sendInitMessage(update.Message.Chat.ID, update.Message.ID)
				// TODO implement later
				//messagesToDelete.Store(update.Message.From.ID, update.Message.ID)
				return

			}

			if strings.Contains(err.Error(), "STICKER_VIDEO_BIG") {
				emojiArgs.QualityValue++
				continue
			}

			if strings.Contains(err.Error(), "STICKERSET_INVALID") {
				sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Не получилось создать некоторые эмодзи. Попробуйте еще раз, либо измените файл."))
				return
			}

			if strings.Contains(err.Error(), "retry_after") {
				parts := strings.Split(err.Error(), "retry_after ")
				var waitTime int
				if len(parts) >= 2 {
					if wt, parseErr := strconv.Atoi(strings.TrimSpace(parts[1])); parseErr == nil {
						waitTime = wt
					}
				}

				if waitTime > 0 {
					dur := time.Duration(waitTime * int(time.Second))
					sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("Вы сможете создать пак только через %.0f минуты", dur.Minutes()))
					return
				}
			}

			sendErrorMessage(ctx, b, update, update.Message.Chat.ID, fmt.Sprintf("%s", err.Error()))
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

	// Формируем сообщение из выбранных эмодзи
	// currentRow := 0
	// for i, emoji := range selectedEmojis {
	// 	if emoji.Transparent {
	// 		continue
	// 	} else {
	// 		// Добавляем эмодзи в текст сообщения
	// 		messageText += "⭐️"

	// 		// Добавляем ссылку на стикер в entities
	// 		entities = append(entities, models.MessageEntity{
	// 			Type:          models.MessageEntityTypeCustomEmoji,
	// 			Offset:        offset,
	// 			Length:        len("⭐️"),
	// 			CustomEmojiID: emoji.DocumentID,
	// 		})
	// 		offset += len("⭐️")
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

func sendInitMessage(chatID int64, msgID int) {
	inlineKeyboard := tgbotapi.NewInlineKeyboardButtonURL("/start", fmt.Sprintf("t.me/%s?start=start", tgbotApi.Self.UserName))
	row := tgbotapi.NewInlineKeyboardRow(inlineKeyboard)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(row)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Чтобы бот мог создать пак \nнажмите кнопку ниже\n↓↓↓↓↓↓↓↓"))
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "MarkdownV2"
	msg.ReplyParameters = tgbotapi.ReplyParameters{
		MessageID: msgID,
		ChatID:    chatID,
	}

	_, err2 := tgbotApi.Send(msg)
	if err2 != nil {
		slog.Error("Failed to send message with emojis", slog.Int64("user_id", chatID), slog.String("err2", err2.Error()))
	}
}

func sendErrorMessage(ctx context.Context, b *bot.Bot, u *models.Update, chatID int64, errToSend string) {
	params := bot.SendMessageParams{
		ReplyParameters: &models.ReplyParameters{
			MessageID: u.Message.ID,
			ChatID:    u.Message.Chat.ID,
		},
		ChatID: chatID,
		Text:   fmt.Sprintf("%s", errToSend),
	}
	//_, err := b.SendMessage(ctx, params)

	chat := fmt.Sprintf("%d", u.Message.Chat.ID)
	if u.Message.MessageThreadID != 0 {
		chat = fmt.Sprintf("%s_%d", chat, u.Message.MessageThreadID)
	}

	err := userBot.SendMessage(ctx, chat, params)
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
		args.Width = types.DefaultWidth
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

//create table public.emoji_packs (
//id integer primary key not null default nextval('emoji_packs_id_seq'::regclass),
//creator_id bigint not null,
//pack_name character varying(255) not null,
//file_url text not null,
//pack_link text,
//initial_command text,
//bot_name character varying(255) not null,
//emoji_count integer not null default 0,
//created_at timestamp with time zone default CURRENT_TIMESTAMP,
//updated_at timestamp with time zone default CURRENT_TIMESTAMP,
//completed boolean default false,
//foreign key (bot_name) references public.bots (name)
//match simple on update no action on delete no action
//);
//create index idx_emoji_packs_creator on emoji_packs using btree (creator_id);
//create index idx_emoji_packs_pack_link on emoji_packs using btree (pack_link);
//create index idx_emoji_packs_bot on emoji_packs using btree (bot_name);
//

func createBlankDatabaseRecord(ctx context.Context, botName string, userID int64) error {
	emojiPack := db.EmojiPack{
		CreatorID:  userID,
		PackName:   "blank",
		BotName:    botName,
		EmojiCount: 0,
	}

	_, err := db.Postgres.CreateEmojiPack(ctx, &emojiPack)
	if err != nil {
		return fmt.Errorf("failed to create blank emoji pack: %w", err)
	}

	return nil
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
	case types.ErrFileNotProvided:
		message = "Нужен файл для создания эмодзи"
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

	if (emojiArgs.BackgroundSim != "" || emojiArgs.BackgroundBlend != "") && emojiArgs.BackgroundColor == "" {
		return &emojiArgs, types.ErrInvalidBackgroundArgumentsUse
	}

	return &emojiArgs, nil
}

func downloadFile(ctx context.Context, b *bot.Bot, m *models.Message, args *types.EmojiCommand) (string, error) {
	var fileID string
	var fileExt string
	var mimeType string

	var exist bool

	if m.Video != nil {
		fileID = m.Video.FileID
		mimeType = m.Video.MimeType
		exist = true
	} else if m.Photo != nil && len(m.Photo) > 0 {
		fileID = m.Photo[len(m.Photo)-1].FileID
		mimeType = "image/jpeg"
		exist = true
	} else if m.Document != nil {
		if slices.Contains(types.AllowedMimeTypes, m.Document.MimeType) {
			fileID = m.Document.FileID
			mimeType = m.Document.MimeType
		} else {
			return "", types.ErrFileOfInvalidType
		}
		exist = true
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
		exist = true
	}

	if exist == false {
		return "", types.ErrFileNotProvided
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
		} else {
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

	// Пытаемся получить доступ к обработке пака
	canProcess, waitCh := stickerQueue.Acquire(args.PackLink)
	if !canProcess {
		// Если нельзя обрабатывать сейчас - ждем своей очереди
		slog.Debug("В ОЧЕРЕДИ", slog.String("pack_link", args.PackLink))
		select {
		case <-ctx.Done():
			stickerQueue.Release(args.PackLink)
			return nil, nil, ctx.Err()
		case <-waitCh:
			slog.Debug("ОЧЕРЕДЬ ПРИШЛА, НАЧИНАЕТСЯ ОБРАБОТКА", slog.String("pack_link", args.PackLink))
		}
	}
	defer stickerQueue.Release(args.PackLink)

	var set *models.StickerSet
	if !args.NewSet {
		var err error
		set, err = b.GetStickerSet(ctx, &bot.GetStickerSetParams{
			Name: args.PackLink,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("get sticker set: %w", err)
		}
	}

	// Загружаем все файлы эмодзи и возвращаем их fileIDs и метаданные
	emojiFileIDs, emojiMetaRows, err := uploadEmojiFiles(ctx, b, args, set, emojiFiles)
	if err != nil {
		return nil, nil, err
	}

	// Создаем набор стикеров

	if args.NewSet {
		set, err = createNewStickerSet(ctx, b, args, emojiFileIDs)
	} else {
		set, err = addToExistingStickerSet(ctx, b, args, set, emojiFileIDs)
	}
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("addEmojis",
		slog.Int("emojiFileIDS count", len(emojiFileIDs)),
		slog.Int("width", args.Width),
		slog.Int("transparent_spacing", types.DefaultWidth-args.Width),
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
	if totalWithTransparent > types.MaxStickersTotal {
		return nil, fmt.Errorf("общее количество стикеров (%d) с прозрачными превысит максимум (%d)", totalWithTransparent, types.MaxStickersTotal)
	}

	return createStickerSetWithBatches(ctx, b, args, emojiFileIDs)
}

// addToExistingStickerSet добавляет эмодзи в существующий набор
func addToExistingStickerSet(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, stickerSet *models.StickerSet, emojiFileIDs []string) (*models.StickerSet, error) {

	// Проверяем, что не превысим лимит
	if len(stickerSet.Stickers)+len(emojiFileIDs) > types.MaxStickersTotal {
		return nil, fmt.Errorf(
			"превышен лимит стикеров в наборе (%d + %d > %d)",
			len(stickerSet.Stickers),
			len(emojiFileIDs),
			types.MaxStickersTotal,
		)
	}

	// Добавляем стикеры батчами
	err := addStickersToSet(ctx, b, args, emojiFileIDs)
	if err != nil {
		return nil, fmt.Errorf("add stickers to set: %w", err)
	}

	return b.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
}

var maxRetries = 5

func addStickersToSet(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFileIDs []string) error {
	for i := 0; i < len(emojiFileIDs); i++ {

		var err error
		for j := 1; j <= maxRetries; j++ {
			_, err = b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
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
			if err == nil {
				slog.Debug("add sticker to set SUCCESS",
					slog.String("file_id", emojiFileIDs[i]),
					slog.String("pack", args.PackLink),
					slog.Int64("user_id", args.UserID),
				)

				break
			} else {
				slog.Debug("error sending sticker", "err", err.Error())
				time.Sleep(time.Second * 1)
			}
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// createStickerSetWithBatches создает новый набор стикеров
func createStickerSetWithBatches(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
	count := len(emojiFileIDs)
	if count > types.MaxStickersInBatch {
		count = types.MaxStickersInBatch
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

	_, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
		UserID:      args.UserID,
		Name:        args.PackLink,
		Title:       args.SetName,
		StickerType: "custom_emoji",
		Stickers:    firstBatch,
	})
	if err != nil && !strings.Contains(err.Error(), "STICKER_VIDEO_NOWEBM") {
		slog.Debug("new sticker set FAILED", slog.String("name", args.PackLink), slog.String("error", err.Error()))
		return nil, fmt.Errorf("create sticker set: %w", err)
	} else if err != nil && strings.Contains(err.Error(), "STICKER_VIDEO_NOWEBM") {
		count = 1
		_, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
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
	}

	emojiFileIDs = emojiFileIDs[count:]

	// Добавляем оставшиеся стикеры по одному
	err = addStickersToSet(ctx, b, args, emojiFileIDs)
	if err != nil {
		return nil, fmt.Errorf("add stickers to set: %w", err)
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

	if len(emojiFiles) > types.MaxStickersTotal {
		return fmt.Errorf("слишком много файлов для создания набора (максимум %d)", types.MaxStickersTotal)
	}

	return nil
}

func prepareTransparentData(width int) ([]byte, error) {
	// Подготавливаем прозрачные стикеры если нужно
	transparentSpacing := types.DefaultWidth - width
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
func uploadEmojiFiles(ctx context.Context, b *bot.Bot, args *types.EmojiCommand, set *models.StickerSet, emojiFiles []string) ([]string, [][]types.EmojiMeta, error) {
	slog.Debug("uploading emoji stickers", slog.Int("count", len(emojiFiles)))

	totalEmojis := len(emojiFiles)
	rows := (totalEmojis + args.Width - 1) / args.Width // Округляем вверх
	emojiMetaRows := make([][]types.EmojiMeta, rows)

	// Проверка на превышение максимального количества стикеров
	totalStickers := len(emojiFiles)
	if args.Width < types.DefaultWidth {
		totalStickers += (types.DefaultWidth - args.Width) * rows
	}

	if set != nil {
		if set.Stickers != nil {
			totalStickers += len(set.Stickers)
		}
	}

	if totalStickers > types.MaxStickersTotal {
		return nil, nil, fmt.Errorf("будет превышено максимальное количество эмодзи в паке (%d из %d)", totalStickers, types.MaxStickersTotal)
	}

	// Подготавливаем прозрачный стикер только если он нужен
	var transparentData []byte
	var err error
	if args.Width < types.DefaultWidth {
		transparentData, err = prepareTransparentData(args.Width)
		if err != nil {
			return nil, nil, err
		}
	}

	for i := range emojiMetaRows {
		if args.Width < types.DefaultWidth {
			emojiMetaRows[i] = make([]types.EmojiMeta, types.DefaultWidth) // Инициализируем каждый ряд с полной шириной
		} else {
			emojiMetaRows[i] = make([]types.EmojiMeta, args.Width) // Инициализируем каждый ряд с полной шириной
		}
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
		} else {
			//slog.Debug("upload sticker SUCCESS",
			//	slog.String("file", emojiFile),
			//	slog.String("pack", args.PackLink),
			//	slog.Int64("user_id", args.UserID),
			//	slog.Bool("transparent", false),
			//)
		}

		// Вычисляем позицию в сетке
		row := i / args.Width
		col := i % args.Width

		// Вычисляем отступы для центрирования
		totalPadding := types.DefaultWidth - args.Width
		leftPadding := totalPadding / 2
		if totalPadding > 0 && totalPadding%2 != 0 {
			// Для нечетного количества отступов, слева меньше на 1
			leftPadding = (totalPadding - 1) / 2
		}

		// Загружаем прозрачные эмодзи слева только если нужно
		if args.Width < types.DefaultWidth {
			for j := 0; j < leftPadding; j++ {
				if emojiMetaRows[row][j].FileID == "" {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, nil, fmt.Errorf("upload transparent sticker: %w", err)
					} else {
						//slog.Debug("upload sticker SUCCESS",
						//	slog.String("file", emojiFile),
						//	slog.String("pack", args.PackLink),
						//	slog.Int64("user_id", args.UserID),
						//	slog.Bool("transparent", true),
						//)
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
		if args.Width < types.DefaultWidth {
			pos = col + leftPadding
		}
		emojiMetaRows[row][pos] = types.EmojiMeta{
			FileID:      fileID,
			FileName:    emojiFile,
			Transparent: false,
		}

		// Загружаем прозрачные эмодзи справа только если нужно
		if args.Width < types.DefaultWidth {
			for j := col + leftPadding + 1; j < types.DefaultWidth; j++ {
				if emojiMetaRows[row][j].FileID == "" {
					transparentFileID, err := uploadSticker(ctx, b, args.UserID, "transparent.webm", transparentData)
					if err != nil {
						return nil, nil, fmt.Errorf("upload transparent sticker: %w", err)
					} else {
						//slog.Debug("upload sticker SUCCESS",
						//	slog.String("file", emojiFile),
						//	slog.String("pack", args.PackLink),
						//	slog.Int64("user_id", args.UserID),
						//	slog.Bool("transparent", true),
						//)
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
	emojiFileIDs := make([]string, 0, rows*types.DefaultWidth)
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
