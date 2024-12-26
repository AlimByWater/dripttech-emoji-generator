package bots

import (
	"context"
	"emoji-generator/db"
	"emoji-generator/processing"
	"emoji-generator/types"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func (d *DripBot) handleEmojiCommandForDM(ctx context.Context, b *bot.Bot, update *models.Update) {
	permissions, err := db.Postgres.Permissions(ctx, update.Message.From.ID)
	if err != nil {
		slog.Error("Failed to get permissions", slog.String("err", err.Error()))
		d.sendMessageByBot(ctx, update, "Возникла внутреняя ошибка. Попробуйте позже")
		return
	}

	if !permissions.PrivateGeneration {
		d.sendMessageByBot(ctx, update, "Вы не можете создавать паки в личном чате. Возможно когда-нибудь...")
		return
	} else {
		if permissions.Vip {
			d.sendMessageByBot(ctx, update, "Если хотите сразу получать эмоджи-композиции - отправьте ту же команду и картинку сюда @drip_tech_helper")
		}
	}

	// Extract command arguments
	args := processing.ExtractCommandArgs(update.Message.Text, update.Message.Caption)
	emojiArgs, err := processing.ParseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		d.sendMessageByBot(ctx, update, err.Error())
		return
	}

	emojiArgs.Permissions = permissions

	// Setup command defaults and working environment
	processing.SetupEmojiCommand(emojiArgs, update.Message.From.ID, update.Message.From.Username)

	// Get bot info and setup pack details
	botInfo, err := b.GetMe(ctx)
	if err != nil {
		slog.Error("Failed to get bot info", slog.String("err", err.Error()))
		d.sendMessageByBot(ctx, update, "Не удалось получить информацию о боте")
		return
	}

	emojiPack, err := processing.SetupPackDetails(ctx, emojiArgs, botInfo.Username)
	if err != nil {
		slog.Error("Failed to setup pack details", slog.String("err", err.Error()))
		d.sendMessageByBot(ctx, update, "пак с подобной ссылкой не найден")
		return
	}

	// Create working directory and download file
	if err := d.prepareWorkingEnvironment(ctx, update, emojiArgs); err != nil {
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
		d.sendMessageByBot(ctx, update, message)
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
			d.sendMessageByBot(ctx, update, "Не удалось создать запись в базе данных")
			return
		}
	}

	var stickerSet *models.StickerSet

	for {
		// Обрабатываем видео
		createdFiles, err := processing.ProcessVideo(emojiArgs)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
			err2 := processing.RemoveDirectory(emojiArgs.WorkingDir)
			if err2 != nil {
				slog.Error("Failed to remove directory", slog.String("err", err2.Error()), slog.String("dir", emojiArgs.WorkingDir), slog.String("emojiPackLink", emojiArgs.PackLink), slog.Int64("user_id", emojiArgs.UserID))
			}
			d.sendMessageByBot(ctx, update, fmt.Sprintf("Ошибка при обработке видео: %s", err.Error()))
			return
		}

		// Создаем набор стикеров
		stickerSet, _, err = d.AddEmojis(ctx, emojiArgs, createdFiles)
		if err != nil {
			if strings.Contains(err.Error(), "PEER_ID_INVALID") || strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "bot was blocked by the user") {
				d.SendInitMessage(update.Message.Chat.ID, update.Message.ID)
				// TODO implement later
				//messagesToDelete.Store(update.Message.From.ID, update.Message.ID)
				return

			}

			if strings.Contains(err.Error(), "STICKER_VIDEO_BIG") {
				emojiArgs.QualityValue++
				continue
			}

			if strings.Contains(err.Error(), "STICKERSET_INVALID") {
				d.sendMessageByBot(ctx, update, fmt.Sprintf("Не получилось создать некоторые эмодзи. Попробуйте еще раз, либо измените файл."))
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
					d.sendMessageByBot(ctx, update, fmt.Sprintf("Вы сможете создать пак только через %.0f минуты", dur.Minutes()))
					return
				}
			}

			d.sendMessageByBot(ctx, update, fmt.Sprintf("%s", err.Error()))
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

	d.sendMessageByBot(ctx, update, fmt.Sprintf("Ваш пак\n%s", "https://t.me/addemoji/"+emojiArgs.PackLink))

}
