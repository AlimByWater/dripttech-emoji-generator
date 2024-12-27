package bots

import (
	"context"
	"emoji-generator/db"
	"emoji-generator/processing"
	"emoji-generator/types"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (d *DripBot) handleEmojiCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	//j, _ := json.MarshalIndent(update, "", "  ")
	//fmt.Println(string(j))
	var progressMsgID int
	var permissions types.Permissions
	var err error
	if update.Message.From.Username == "Channel_Bot" || update.Message.From.ID == 1087968824 {
		var id int64
		if update.Message.From.Username == "Channel_Bot" {
			id = update.Message.SenderChat.ID
		} else if update.Message.From.ID == 1087968824 {
			id = update.Message.From.ID
		}

		permissions, err = db.Postgres.PermissionsByChannelID(ctx, id)
		if err != nil {
			slog.Error("Failed to get permissions by channel", slog.String("err", err.Error()))
			d.sendMessageByBot(ctx, update, "Возникла внутреняя ошибка. Попробуйте позже")
			return
		}

		if permissions.UseByChannelName && slices.Contains(permissions.ChannelIDs, update.Message.SenderChat.ID) {
			update.Message.From.ID = permissions.UserID
			update.Message.From.IsBot = false
		} else {
			d.sendMessageByBot(ctx, update, "Вы не можете создать пак от лица канала.")
			return
		}
	} else {
		permissions, err = db.Postgres.Permissions(ctx, update.Message.From.ID)
		if err != nil {
			slog.Error("Failed to get permissions", slog.String("err", err.Error()))
			d.sendMessageByBot(ctx, update, "Возникла внутреняя ошибка. Попробуйте позже")
			return
		}
	}

	// Extract command arguments
	args := processing.ExtractCommandArgs(update.Message.Text, update.Message.Caption)
	emojiArgs, err := processing.ParseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		d.sendErrorMessage(ctx, update, update.Message.Chat.ID, err.Error())
		return
	}

	emojiArgs.Permissions = permissions

	if update.Message.From.IsBot || update.Message.From.ID < 0 {
		d.sendErrorMessage(ctx, update, update.Message.Chat.ID, "Создать пак можно только с личного аккаунта")
		return
	}

	// Setup command defaults and working environment
	processing.SetupEmojiCommand(emojiArgs, update.Message.From.ID, update.Message.From.Username)

	// Get bot info and setup pack details
	botInfo, err := b.GetMe(ctx)
	if err != nil {
		slog.Error("Failed to get bot info", slog.String("err", err.Error()))
		d.sendErrorMessage(ctx, update, update.Message.Chat.ID, "Не удалось получить информацию о боте")
		return
	}

	exist, err := db.Postgres.UserExists(ctx, update.Message.From.ID, botInfo.Username)
	if err != nil {
		slog.Error("Failed to check if user exists", slog.Int64("user_id", update.Message.From.ID), slog.String("err", err.Error()))
		d.SendInitMessage(update.Message.Chat.ID, update.Message.ID)
		return
	}
	if !exist {
		d.SendInitMessage(update.Message.Chat.ID, update.Message.ID)
		return
	}

	emojiPack, err := processing.SetupPackDetails(ctx, emojiArgs, botInfo.Username)
	if err != nil {
		slog.Error("Failed to setup pack details", slog.String("err", err.Error()))
		d.sendErrorMessage(ctx, update, update.Message.Chat.ID, "пак с подобной ссылкой не найден")
		return
	}

	// Create working directory and download file
	if err := d.prepareWorkingEnvironment(ctx, update, emojiArgs); err != nil {
		d.handleDownloadError(ctx, update, err)
		return
	}

	if emojiPack == nil {
		// Create database record
		emojiPack, err = d.createDatabaseRecord(ctx, emojiArgs, args, botInfo.Username)
		if err != nil {
			slog.Error("Failed to log emoji command",
				slog.String("err", err.Error()),
				slog.String("pack_link", emojiArgs.PackLink),
				slog.Int64("user_id", emojiArgs.UserID))
			d.sendErrorMessage(ctx, update, update.Message.Chat.ID, "Не удалось создать запись в базе данных")
			return
		}

		// Отправляем первое сообщение с прогрессом
		progress, err := d.sendProgressMessage(ctx, update.Message.Chat.ID, update.Message.ID, "⏳ Начинаем создание эмодзи-пака...")
		if err != nil {
			slog.Error("Failed to send initial progress message",
				slog.String("err", err.Error()),
				slog.Int64("user_id", emojiArgs.UserID))
		}

		progressMsgID = progress.MessageID
		defer d.deleteProgressMessage(ctx, update.Message.Chat.ID, progressMsgID)
	}

	var stickerSet *models.StickerSet
	var emojiMetaRows [][]types.EmojiMeta

	for {
		// Обновляем статус: начало обработки видео
		err = d.updateProgressMessage(ctx, update.Message.Chat.ID, progressMsgID, "🎬 Обрабатываем видео...")
		if err != nil {
			slog.Error("Failed to update progress message", slog.String("err", err.Error()))
		}

		// Обрабатываем видео
		createdFiles, err := processing.ProcessVideo(emojiArgs)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
			err2 := processing.RemoveDirectory(emojiArgs.WorkingDir)
			if err2 != nil {
				slog.Error("Failed to remove directory", slog.String("err", err2.Error()), slog.String("dir", emojiArgs.WorkingDir), slog.String("emojiPackLink", emojiArgs.PackLink), slog.Int64("user_id", emojiArgs.UserID))
			}
			d.sendErrorMessage(ctx, update, update.Message.Chat.ID, fmt.Sprintf("Ошибка при обработке видео: %s", err.Error()))
			return
		}

		// Обновляем статус: создание стикеров
		err = d.updateProgressMessage(ctx, update.Message.Chat.ID, progressMsgID, "✨ Создаем эмодзи...")
		if err != nil {
			slog.Error("Failed to update progress message", slog.String("err", err.Error()))
		}

		// Создаем набор стикеров
		stickerSet, emojiMetaRows, err = d.AddEmojis(ctx, emojiArgs, createdFiles)
		if err != nil {
			if strings.Contains(err.Error(), "PEER_ID_INVALID") || strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "bot was blocked by the user") {
				d.SendInitMessage(update.Message.Chat.ID, update.Message.ID)
				// TODO implement later
				//messagesToDelete.Store(update.Message.From.ID, update.Message.ID)
				return
			}

			if strings.Contains(err.Error(), "STICKER_VIDEO_BIG") {
				_ = d.updateProgressMessage(ctx, update.Message.Chat.ID, progressMsgID, "🔄 Оптимизируем размер видео...")
				emojiArgs.QualityValue++
				continue
			}

			if strings.Contains(err.Error(), "STICKERSET_INVALID") {
				d.sendErrorMessage(ctx, update, update.Message.Chat.ID, fmt.Sprintf("Не получилось создать некоторые эмодзи. Попробуйте еще раз, либо измените файл."))
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
					d.sendErrorMessage(ctx, update, update.Message.Chat.ID, fmt.Sprintf("Вы сможете создать пак только через %.0f минуты", dur.Minutes()))
					return
				}
			}

			d.sendErrorMessage(ctx, update, update.Message.Chat.ID, fmt.Sprintf("%s", err.Error()))
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

	// Обновляем статус: генерация композиции
	err = d.updateProgressMessage(ctx, update.Message.Chat.ID, progressMsgID, "🎨 Генерируем эмодзи-композицию...")
	if err != nil {
		slog.Error("Failed to update progress message", slog.String("err", err.Error()))
	}

	// Создаем композицию эмодзи, используя метаданные из emojiMetaRows
	// Выбираем нужные эмодзи
	selectedEmojis := processing.GenerateEmojiMessage(emojiMetaRows, stickerSet, emojiArgs)

	var topicId string
	if update.Message.MessageThreadID != 0 {
		topicId = fmt.Sprintf("%d_%d", update.Message.Chat.ID, update.Message.MessageThreadID)
	} else {
		topicId = fmt.Sprintf("%d", update.Message.Chat.ID)
	}
	err = d.userBot.SendMessageWithEmojis(ctx, topicId, emojiArgs.Width, emojiArgs.PackLink, emojiArgs.RawInitCommand, selectedEmojis, update.Message.ID)
	if err != nil {
		slog.Error("Failed to send message with emojis", slog.String("err", err.Error()), slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID))
	}
}
