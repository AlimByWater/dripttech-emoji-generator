package bots

import (
	"bytes"
	"context"
	"emoji-generator/db"
	"emoji-generator/httpclient"
	"emoji-generator/processing"
	"emoji-generator/progress"
	"emoji-generator/queue"
	"emoji-generator/types"
	"fmt"
	"github.com/cavaliergopher/grab/v3"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var (
	validchatIDs            = []string{"-1002400904088_3", "-1002491830452_3", "-1002002718381"}
	packDeletePrefixMessage = "pack_delete:"
)

type UserBot interface {
	SendMessageWithEmojis(ctx context.Context, chatID string, width int, packLink string, command string, emojis []types.EmojiMeta, replyTo int) error
	SendMessage(ctx context.Context, chatID string, msg bot.SendMessageParams) error
}

type DripBot struct {
	bot              *bot.Bot
	tgbotApi         *tgbotapi.BotAPI
	userBot          UserBot
	token            string
	wg               sync.WaitGroup
	stickerQueue     *queue.StickerQueue
	messagesToDelete sync.Map
	progressManager  *progress.Manager
}

func NewDripBot(token string, userBot UserBot) (*DripBot, error) {
	rl := rate.NewLimiter(rate.Every(1*time.Second), 100)
	c := httpclient.NewClient(rl)

	dbot := &DripBot{
		userBot:      userBot,
		token:        token,
		stickerQueue: queue.New(),
	}

	b, err := bot.New(token,
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
			dbot.wg.Add(1)
			defer dbot.wg.Done()
			dbot.handler(ctx, b, update)
		}),
		bot.WithHTTPClient(time.Minute, c))
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %w", err)
	}

	tgbotApi, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, c)
	if err != nil {
		return nil, fmt.Errorf("error creating tgbotapi: %w", err)
	}

	tgbotApi.StopReceivingUpdates()

	dbot.bot = b
	dbot.tgbotApi = tgbotApi
	dbot.progressManager = progress.NewManager(b)

	return dbot, nil
}

func (d *DripBot) Start(ctx context.Context) {
	d.bot.Start(ctx)
}

func (d *DripBot) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("All handlers completed successfully")
	case <-time.After(30 * time.Second):
		slog.Warn("Timeout waiting for handlers to complete")
	}
}

func (d *DripBot) BotUserName() string {
	return d.tgbotApi.Self.UserName
}

func (d *DripBot) handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	if update.Message.Chat.Type == models.ChatTypeChannel || update.Message.Chat.Type == models.ChatTypeSupergroup || update.Message.Chat.Type == models.ChatTypeGroup {
		for i, chatID := range validchatIDs {
			if chatID == fmt.Sprintf("%d_%d", update.Message.Chat.ID, update.Message.MessageThreadID) {
				break
			}

			if chatID == fmt.Sprintf("%d", update.Message.Chat.ID) {
				break
			}

			if i == len(validchatIDs)-1 {
				return
			}
		}

		// Проверяем, является ли сообщение командой
		if strings.HasPrefix(update.Message.Text, "/emoji") {
			d.handleEmojiCommand(ctx, b, update)
		} else if update.Message.Text == "/emoji" {
			d.handleEmojiCommand(ctx, b, update)
		} else if strings.HasPrefix(update.Message.Caption, "/emoji ") {
			d.handleEmojiCommand(ctx, b, update)
		} else if update.Message.Caption == "/emoji " {
			d.handleEmojiCommand(ctx, b, update)
		} else if update.Message.Text == "/info" {
			d.handleInfoCommand(ctx, b, update)
		}

		return
	}

	if update.Message.Chat.Type == models.ChatTypePrivate {
		if strings.Contains(update.Message.Text, "start") {
			d.handleStartCommand(ctx, b, update)
			return
		} else if strings.Contains(update.Message.Text, "info") {
			d.handleInfoCommand(ctx, b, update)
			return
		}

		if strings.HasPrefix(update.Message.Text, "/emoji") ||
			strings.HasPrefix(update.Message.Caption, "/emoji ") {
			d.handleEmojiCommandForDM(ctx, b, update)
			return
		}
	}
}

const (
	defaultStickerFormat = "video"
	defaultEmojiIcon     = "⭐️"
)

func (d *DripBot) SendInitMessage(chatID int64, msgID int) {
	inlineKeyboard := tgbotapi.NewInlineKeyboardButtonURL("/start", fmt.Sprintf("t.me/%s?start=start", d.tgbotApi.Self.UserName))
	row := tgbotapi.NewInlineKeyboardRow(inlineKeyboard)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(row)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Чтобы бот мог создать пак \nнажмите кнопку ниже\n↓↓↓↓↓↓↓↓"))
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "MarkdownV2"
	msg.ReplyParameters = tgbotapi.ReplyParameters{
		MessageID: msgID,
		ChatID:    chatID,
	}

	_, err2 := d.tgbotApi.Send(msg)
	if err2 != nil {
		slog.Error("Failed to send message with emojis", slog.Int64("user_id", chatID), slog.String("err2", err2.Error()))
	}
}

func (d *DripBot) sendMessageByBot(ctx context.Context, chatID int64, replyTo int, msgToSend string, keyboard models.ReplyMarkup) {
	params := &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        fmt.Sprintf("%s", msgToSend),
		ReplyMarkup: d.menuButtons(ctx),
	}

	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{
			MessageID: replyTo,
			ChatID:    chatID,
		}
	}

	_, err := d.bot.SendMessage(ctx, params)
	if err != nil {
		slog.Error("Failed to send error message", slog.String("err", err.Error()), slog.Int64("user_id", chatID))
	}
	return
}

func (d *DripBot) sendErrorMessage(ctx context.Context, chatID int64, replyTo int, threadID int, errToSend string) {
	params := bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("%s", errToSend),
	}

	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{
			MessageID: replyTo,
			ChatID:    chatID,
		}
	}
	//_, err := b.SendMessage(ctx, params)

	chat := fmt.Sprintf("%d", chatID)
	if threadID != 0 {
		chat = fmt.Sprintf("%s_%d", chat, threadID)
	}

	err := d.userBot.SendMessage(ctx, chat, params)
	if err != nil {
		slog.Error("Failed to send error message", slog.String("err", err.Error()))
	}

}

func (d *DripBot) createDatabaseRecord(ctx context.Context, args *types.EmojiCommand, initialCommand string, botUsername string) (*db.EmojiPack, error) {
	emojiPack := &db.EmojiPack{
		CreatorID:      args.UserID,
		PackName:       args.SetName,
		PackLink:       &args.PackLink,
		InitialCommand: &initialCommand,
		BotName:        botUsername,
		EmojiCount:     0,
		TelegramFileID: args.File.FileID,
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

func (d *DripBot) sendProgressMessage(ctx context.Context, chatID int64, replyToID int, status string) (*progress.Message, error) {
	return d.progressManager.SendMessage(ctx, chatID, replyToID, status)
}

func (d *DripBot) deleteProgressMessage(ctx context.Context, chatID int64, msgID int) error {
	return d.progressManager.DeleteMessage(ctx, chatID, msgID)
}

func (d *DripBot) updateProgressMessage(ctx context.Context, chatID int64, msgID int, status string) error {
	return d.progressManager.UpdateMessage(ctx, chatID, msgID, status)
}

func (d *DripBot) createBlankDatabaseRecord(ctx context.Context, botName string, userID int64) error {
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

func (d *DripBot) prepareWorkingEnvironment(ctx context.Context, update *models.Update, args *types.EmojiCommand) error {
	if err := os.MkdirAll(args.WorkingDir, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	fileName, err := d.downloadFile(ctx, update.Message, args)
	if err != nil {
		return err
	}
	args.DownloadedFile = fileName
	return nil
}

func (d *DripBot) handleDownloadError(ctx context.Context, update *models.Update, err error) {
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
	d.sendErrorMessage(ctx, update.Message.Chat.ID, update.Message.ID, update.Message.MessageThreadID, message)
}

func (d *DripBot) downloadFile(ctx context.Context, m *models.Message, args *types.EmojiCommand) (string, error) {
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

	file, err := d.bot.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
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

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", d.token, file.FilePath)
	resp, err := grab.Get(args.WorkingDir+"/saved"+fileExt, fileURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", types.ErrFileDownloadFailed, err)
	}

	return resp.Filename, nil
}

// Функция для обработки ошибок с повторными попытками
func (d *DripBot) handleTelegramError(err error) (int, error) {
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

func (d *DripBot) uploadSticker(ctx context.Context, userID int64, filename string, data []byte) (string, error) {
	for {
		newSticker, err := d.bot.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
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

		if waitTime, err := d.handleTelegramError(err); err != nil {
			return "", fmt.Errorf("upload sticker: %w", err)
		} else if waitTime > 0 {
			slog.Info("waiting before retry", "seconds", waitTime)
			time.Sleep(time.Duration(waitTime) * time.Second)
			continue
		}

		return newSticker.FileID, nil
	}
}

func (d *DripBot) AddEmojis(ctx context.Context, args *types.EmojiCommand, emojiFiles []string) (*models.StickerSet, [][]types.EmojiMeta, error) {
	if err := processing.ValidateEmojiFiles(emojiFiles); err != nil {
		return nil, nil, err
	}

	// Пытаемся получить доступ к обработке пака
	canProcess, waitCh := d.stickerQueue.Acquire(args.PackLink)
	if !canProcess {
		// Если нельзя обрабатывать сейчас - ждем своей очереди
		slog.Debug("В ОЧЕРЕДИ", slog.String("pack_link", args.PackLink))
		select {
		case <-ctx.Done():
			d.stickerQueue.Release(args.PackLink)
			return nil, nil, ctx.Err()
		case <-waitCh:
			slog.Debug("ОЧЕРЕДЬ ПРИШЛА, НАЧИНАЕТСЯ ОБРАБОТКА", slog.String("pack_link", args.PackLink))
		}
	}
	defer d.stickerQueue.Release(args.PackLink)

	var set *models.StickerSet
	if !args.NewSet {
		var err error
		set, err = d.bot.GetStickerSet(ctx, &bot.GetStickerSetParams{
			Name: args.PackLink,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("get sticker set: %w", err)
		}
	}

	// Загружаем все файлы эмодзи и возвращаем их fileIDs и метаданные
	emojiFileIDs, emojiMetaRows, err := d.uploadEmojiFiles(ctx, args, set, emojiFiles)
	if err != nil {
		return nil, nil, err
	}

	// Создаем набор стикеров

	if args.NewSet {
		set, err = d.createNewStickerSet(ctx, args, emojiFileIDs)
	} else {
		set, err = d.addToExistingStickerSet(ctx, args, set, emojiFileIDs)
	}
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("addEmojis",
		slog.Int("emojiFileIDS count", len(emojiFileIDs)),
		slog.Int("width", args.Width),
		slog.Int("transparent_spacing", types.DefaultWidth-args.Width),
		slog.Int("stickers in set", len(set.Stickers)))

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
func (d *DripBot) createNewStickerSet(ctx context.Context, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
	totalWithTransparent := len(emojiFileIDs)
	if totalWithTransparent > types.MaxStickersTotal {
		return nil, fmt.Errorf("общее количество стикеров (%d) с прозрачными превысит максимум (%d)", totalWithTransparent, types.MaxStickersTotal)
	}

	return d.createStickerSetWithBatches(ctx, args, emojiFileIDs)
}

// addToExistingStickerSet добавляет эмодзи в существующий набор
func (d *DripBot) addToExistingStickerSet(ctx context.Context, args *types.EmojiCommand, stickerSet *models.StickerSet, emojiFileIDs []string) (*models.StickerSet, error) {

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
	err := d.addStickersToSet(ctx, args, emojiFileIDs)
	if err != nil {
		return nil, fmt.Errorf("add stickers to set: %w", err)
	}

	return d.bot.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
}

var maxRetries = 5

func (d *DripBot) addStickersToSet(ctx context.Context, args *types.EmojiCommand, emojiFileIDs []string) error {
	for i := 0; i < len(emojiFileIDs); i++ {

		var err error
		for j := 1; j <= maxRetries; j++ {
			_, err = d.bot.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
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
				//slog.Debug("add sticker to set SUCCESS",
				//	slog.String("file_id", emojiFileIDs[i]),
				//	slog.String("pack", args.PackLink),
				//	slog.Int64("user_id", args.UserID),
				//)

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
func (d *DripBot) createStickerSetWithBatches(ctx context.Context, args *types.EmojiCommand, emojiFileIDs []string) (*models.StickerSet, error) {
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

	_, err := d.bot.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
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
		_, err := d.bot.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
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
	err = d.addStickersToSet(ctx, args, emojiFileIDs)
	if err != nil {
		return nil, fmt.Errorf("add stickers to set: %w", err)
	}

	// Получаем финальное состояние набора
	set, err := d.bot.GetStickerSet(ctx, &bot.GetStickerSetParams{
		Name: args.PackLink,
	})
	if err != nil {
		return nil, fmt.Errorf("get sticker set: %w", err)
	}

	return set, nil
}

// uploadEmojiFiles загружает все файлы эмодзи и возвращает их fileIDs и метаданные
func (d *DripBot) uploadEmojiFiles(ctx context.Context, args *types.EmojiCommand, set *models.StickerSet, emojiFiles []string) ([]string, [][]types.EmojiMeta, error) {
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
		transparentData, err = processing.PrepareTransparentData(args.Width)
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

		fileID, err := d.uploadSticker(ctx, args.UserID, emojiFile, fileData)
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
					transparentFileID, err := d.uploadSticker(ctx, args.UserID, "transparent.webm", transparentData)
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
					transparentFileID, err := d.uploadSticker(ctx, args.UserID, "transparent.webm", transparentData)
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
