package bots

import (
	"context"
	"emoji-generator/db"
	"fmt"
	"github.com/go-telegram/ui/keyboard/inline"
	"github.com/go-telegram/ui/keyboard/reply"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (d *DripBot) onPackSelect(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	defer func() {
		msgToDelete, loaded := d.messagesToDelete.LoadAndDelete(fmt.Sprintf("%s%d", packDeletePrefixMessage, update.Message.Chat.ID))
		if !loaded {
			return
		}

		id := msgToDelete.(int)
		ok, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    update.Message.Chat.ID,
			MessageID: id,
		})
		if err != nil || !ok {
			slog.Error("delete choose pack message", slog.Int("msg_id", id), slog.String("err", err.Error()), slog.String("username", update.Message.Chat.Username), slog.Int64("user_id", update.Message.From.ID))
		}
	}()

	kb := inline.New(d.bot).
		Row().
		Button("Мои паки", []byte("packs"), d.onRemovePacksSelect).
		Button("Удалить пак", []byte(update.Message.Text), d.onPackDelete)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Выбран пак:\nt.me/addemoji/" + update.Message.Text,
		ReplyMarkup: kb,
	})
	if err != nil {
		slog.Error("send message", slog.String("err", err.Error()))
		return
	}

	return
}

func (d *DripBot) onPackDelete(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	err := db.Postgres.SetDeletedPack(ctx, string(data))
	if err != nil {
		slog.Error("delete emoji pack", slog.String("err", err.Error()), slog.String("name", string(data)), slog.String("username", mes.Message.Chat.Username), slog.Int64("user_id", mes.Message.From.ID))
		d.sendMessageByBot(ctx, mes.Message.Chat.ID, 0, "Не удалось удалить пак. Попробуйте позже", nil)
		return
	}

	_, err = d.bot.DeleteStickerSet(ctx, &bot.DeleteStickerSetParams{
		Name: string(data),
	})
	if err != nil {
		//err = db.Postgres.UnsetDeletedPack(ctx, string(data))
		if strings.Contains(err.Error(), "STICKERSET_INVALID") && strings.Contains(string(data), d.tgbotApi.Self.UserName) {
			d.sendMessageByBot(ctx, mes.Message.Chat.ID, 0, "Похоже пак уже удален.", d.startKeyboard(ctx))
			err = db.Postgres.SetDeletedPack(ctx, string(data))
			if err != nil {
				slog.Error("set delete emoji pack", slog.String("err", err.Error()), slog.String("link", string(data)), slog.String("username", mes.Message.Chat.Username), slog.Int64("user_id", mes.Message.From.ID))
			}
			return
		}
		slog.Error("delete sticker set", slog.String("err", err.Error()), slog.String("name", string(data)), slog.String("username", mes.Message.Chat.Username), slog.Int64("user_id", mes.Message.From.ID))
		d.sendMessageByBot(ctx, mes.Message.Chat.ID, 0, "Не удалось удалить пак. Попробуйте позже", nil)
		return
	}

	params := &bot.SendMessageParams{
		ChatID:      mes.Message.Chat.ID,
		Text:        "Пак удален",
		ReplyMarkup: d.startKeyboard(ctx),
	}

	_, err = d.bot.SendMessage(ctx, params)
	if err != nil {
		slog.Error("send message", slog.String("err", err.Error()))
		d.sendMessageByBot(ctx, mes.Message.Chat.ID, 0, "Пак удален.", nil)
	}
	return
}

func (d *DripBot) onRemovePacksSelect(ctx context.Context, b *bot.Bot, mes models.MaybeInaccessibleMessage, data []byte) {
	//fmt.Println(mes.Message.From.ID)
	//j, _ := json.MarshalIndent(mes, "", "  ")
	//fmt.Println(string(j))

	packs, err := db.Postgres.GetEmojiPacksByCreator(ctx, mes.Message.Chat.ID, d.tgbotApi.Self.UserName, false)
	if err != nil {
		slog.Error("get emoji packs by creator", slog.String("err", err.Error()))
		d.sendErrorMessage(ctx, mes.Message.Chat.ID, 0, 0, "Возникла внутреняя ошибка. Попробуйте позже")
		return
	}
	//
	//defer func() {
	//	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
	//		ChatID:    mes.Message.Chat.ID,
	//		MessageID: mes.Message.ID,
	//	})
	//	if err != nil {
	//		slog.Error("delete message", slog.String("err", err.Error()))
	//	}
	//}()

	if len(packs) != 0 {
		kb := reply.New(
			reply.WithPrefix("reply_keyboard"),
			reply.IsSelective(),
			reply.IsOneTimeKeyboard())
		for i, pack := range packs {
			if pack.PackLink != nil {
				if i%2 == 0 {
					kb.Row()
				}
				kb.Button(*pack.PackLink, b, bot.MatchTypeExact, d.onPackSelect)
			}
		}

		m, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      mes.Message.Chat.ID,
			Text:        "Выберите пак:",
			ReplyMarkup: kb,
		})
		if err != nil {
			slog.Error("send packs keyboard", slog.String("err", err.Error()))
			d.sendErrorMessage(ctx, mes.Message.Chat.ID, 0, 0, "Не удалось отправить список паков")
		}
		d.messagesToDelete.Store(fmt.Sprintf("%s%d", packDeletePrefixMessage, mes.Message.Chat.ID), m.ID)
	} else {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      mes.Message.Chat.ID,
			Text:        "У вас нет паков",
			ReplyMarkup: d.startKeyboard(ctx),
		})
		if err != nil {
			slog.Error("send packs keyboard", slog.String("err", err.Error()))
			d.sendErrorMessage(ctx, mes.Message.Chat.ID, 0, 0, "Не удалось отправить список паков")
		}
	}

}
