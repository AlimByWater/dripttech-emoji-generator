package userbot

import (
	"emoji-generator/bots"
	"emoji-generator/db"
	"emoji-generator/processing"
	"emoji-generator/types"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/go-telegram/bot/models"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func (u *User) emoji(ctx *ext.Context, update *ext.Update) error {
	var progressMsgID int

	if !update.EffectiveChat().IsAUser() {
		return nil
	}
	permissions, err := db.Postgres.Permissions(ctx, update.EffectiveChat().GetID())
	if err != nil {
		slog.Error("Failed to get permissions", slog.String("err", err.Error()))
		u.sendMessageByBot(ctx, update, "Возникла внутреняя ошибка. Попробуйте позже")
		return err
	}

	if !permissions.Vip {
		u.sendMessageByBot(ctx, update,
			fmt.Sprintf(
				"Вы не ⁂VIP. Возможно когда-нибудь...\nЗа дополнительной информацией пишите @%s или @drip_tech",
				types.VIP_BOT_USERNAME,
			))
		return fmt.Errorf("private generation is not allowed for user")
	}

	// Extract command arguments
	args := processing.ExtractCommandArgs(update.EffectiveMessage.Text, "")
	emojiArgs, err := processing.ParseArgs(args)
	if err != nil {
		slog.Error("Invalid arguments", slog.String("err", err.Error()))
		u.sendMessageByBot(ctx, update, err.Error())
		return err
	}

	// Create progress message
	progressMsgID = u.CreateProgressMessage(ctx, update.EffectiveChat().GetID(), update.EffectiveMessage.GetID(), "Начинаем обработку...")
	defer u.DeleteProgressMessage(ctx, update.EffectiveChat().GetID(), progressMsgID)

	emojiArgs.Permissions = permissions

	// Setup command defaults and working environment
	processing.SetupEmojiCommand(emojiArgs, update.EffectiveChat().GetID(), update.GetUserChat().Username)

	// Update progress message
	u.UpdateProgressMessage(ctx, update.EffectiveChat().GetID(), progressMsgID, "Подготовка данных...")

	// ++++++++ CHOOSE ACTUAL BOT TO CREATE STICKERS ++++++++
	dripBot := u.chooseBot(emojiArgs)
	// ++++++++++++++++++++++++++++++++++++++++++++++++++++++

	emojiPack, err := processing.SetupPackDetails(ctx, emojiArgs, dripBot.BotUserName())
	if err != nil {
		slog.Error("Failed to setup pack details", slog.String("err", err.Error()))
		u.sendMessageByBot(ctx, update, "пак с подобной ссылкой не найден")
		return err
	}

	if err := u.prepareWorkingEnvironment(ctx, update, emojiArgs); err != nil {
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
		u.sendMessageByBot(ctx, update, message)
		return err
	}

	if emojiPack == nil {
		// Create database record
		emojiPack, err = u.createDatabaseRecord(emojiArgs, args, dripBot.BotUserName())
		if err != nil {
			slog.Error("Failed to log emoji command",
				slog.String("err", err.Error()),
				slog.String("pack_link", emojiArgs.PackLink),
				slog.Int64("user_id", emojiArgs.UserID))
			u.sendMessageByBot(ctx, update, "Не удалось создать запись в базе данных")
			return err
		}
	}

	var stickerSet *models.StickerSet
	var emojiMetaRows [][]types.EmojiMeta

	for {

		u.UpdateProgressMessage(ctx, update.EffectiveChat().GetID(), progressMsgID, "Обрабатываем видео...")
		// Обрабатываем видео
		createdFiles, err := processing.ProcessVideo(emojiArgs)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "Ошибка при обработке видео", emojiArgs.ToSlogAttributes(slog.String("err", err.Error()))...)
			err2 := processing.RemoveDirectory(emojiArgs.WorkingDir)
			if err2 != nil {
				slog.Error("Failed to remove directory", slog.String("err", err2.Error()), slog.String("dir", emojiArgs.WorkingDir), slog.String("emojiPackLink", emojiArgs.PackLink), slog.Int64("user_id", emojiArgs.UserID))
			}
			u.sendMessageByBot(ctx, update, fmt.Sprintf("Ошибка при обработке видео: %s", err.Error()))
			return err
		}

		u.UpdateProgressMessage(ctx, update.EffectiveChat().GetID(), progressMsgID, "Создаем эмодзи пак...")
		// Создаем набор стикеров
		stickerSet, emojiMetaRows, err = dripBot.AddEmojis(ctx, emojiArgs, createdFiles)
		if err != nil {
			if strings.Contains(err.Error(), "PEER_ID_INVALID") || strings.Contains(err.Error(), "user not found") || strings.Contains(err.Error(), "bot was blocked by the user") {
				dripBot.SendInitMessage(update.EffectiveChat().GetID(), update.EffectiveMessage.ID)
				// TODO implement later
				//messagesToDelete.Store(update.Message.From.ID, update.Message.ID)
				return err

			}

			if strings.Contains(err.Error(), "STICKER_VIDEO_BIG") {
				emojiArgs.QualityValue++
				continue
			}

			if strings.Contains(err.Error(), "STICKERSET_INVALID") {
				u.sendMessageByBot(ctx, update, fmt.Sprintf("Не получилось создать некоторые эмодзи. Попробуйте еще раз, либо измените файл."))
				return err
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
					u.sendMessageByBot(ctx, update, fmt.Sprintf("Вы сможете создать пак только через %.0f минуты", dur.Minutes()))
					return err
				}
			}

			u.sendMessageByBot(ctx, update, fmt.Sprintf("%s", err.Error()))
			return err
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

	u.UpdateProgressMessage(ctx, update.EffectiveChat().GetID(), progressMsgID, "Генерируем эмодзи-композицию...")
	selectedEmojis := processing.GenerateEmojiMessage(emojiMetaRows, stickerSet, emojiArgs)
	err = u.sendMessageWithEmojisInDM(ctx, update, emojiArgs.Width, emojiArgs.PackLink, selectedEmojis)
	if err != nil {
		slog.Error("Failed to send message with emojis in DM",
			slog.String("err", err.Error()),
			slog.String("pack_link", emojiArgs.PackLink),
			slog.Int64("user_id", emojiArgs.UserID))

		u.sendMessageByBot(ctx, update,
			"Не удалось отправить сообщение c эмоджи композицией, но вот ваш пак: https://t.me/addemoji/"+emojiArgs.PackLink,
		)
	}

	return nil
}

func (u *User) sendMessageWithEmojisInDM(ctx *ext.Context, update *ext.Update, width int, packLink string, emojis []types.EmojiMeta) error {
	sender := message.NewSender(tg.NewClient(u.client))
	peer := u.client.PeerStorage.GetInputPeerById(update.EffectiveChat().GetID())

	formats, err := u.styledText(width, packLink, emojis)
	if err != nil {
		return fmt.Errorf("ошибка форматирования текста: %v", err)
	}

	_, err = sender.To(peer).Reply(update.EffectiveMessage.ID).NoWebpage().StyledText(ctx, formats...)
	if err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %v", err)
	}

	return nil
}

func (u *User) prepareWorkingEnvironment(ctx *ext.Context, update *ext.Update, args *types.EmojiCommand) error {
	// +++++++ FILE ++++++++
	workingDir := fmt.Sprintf("/tmp/%d_%d", update.EffectiveChat().GetID(), time.Now().Unix())
	fileName, err := u.downloadMedia(ctx, update, workingDir)
	if err != nil {
		return fmt.Errorf("ошибка при загрузке медиа: %v", err)
	}

	args.DownloadedFile = fileName
	return nil
}

func (u *User) chooseBot(emojiArgs *types.EmojiCommand) *bots.DripBot {
	var dripBot *bots.DripBot
	if emojiArgs.PackLink == "" {
		dripBot = bots.Manager.GetBotByUsername(types.VIP_BOT_USERNAME)
	} else {
		if strings.Contains(emojiArgs.PackLink, types.VIP_BOT_USERNAME) {
			dripBot = bots.Manager.GetBotByUsername(types.VIP_BOT_USERNAME)
		} else if strings.Contains(emojiArgs.PackLink, types.BOT_USERNAME) {
			dripBot = bots.Manager.GetBotByUsername(types.BOT_USERNAME)
		} else if strings.Contains(emojiArgs.PackLink, types.TEST_BOT_USERNAME) {
			dripBot = bots.Manager.GetBotByUsername(types.TEST_BOT_USERNAME)
		}
	}

	return dripBot
}
