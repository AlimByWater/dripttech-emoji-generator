package bots

import (
	"context"
	"database/sql"
	"emoji-generator/db"
	"emoji-generator/types"
	"errors"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/go-telegram/ui/keyboard/inline"
	"github.com/go-telegram/ui/keyboard/reply"
	"log/slog"
)

func (d *DripBot) handleStartCommand(ctx context.Context, b *bot.Bot, update *models.Update) {

	if update.Message.Chat.Type == models.ChatTypePrivate {
		exist, err := db.Postgres.UserExists(ctx, update.Message.From.ID, d.tgbotApi.Self.UserName)
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
			err = d.createBlankDatabaseRecord(ctx, d.tgbotApi.Self.UserName, update.Message.From.ID)
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
			msgID, ok := d.messagesToDelete.LoadAndDelete(update.Message.From.ID)
			if ok {
				for i := range validchatIDs {
					deleted, _ := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: validchatIDs[i], MessageID: msgID.(int)})
					if deleted {
						break
					}
				}
			}
		}

		// delete message
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      update.Message.Chat.ID,
			Text:        "Привет, *" + bot.EscapeMarkdown(update.Message.From.FirstName) + "*",
			ParseMode:   models.ParseModeMarkdown,
			ReplyMarkup: d.menuButtons(ctx),
		})

		startKeyboard := d.startKeyboard(ctx)

		if d.tgbotApi.Self.UserName == types.BOT_USERNAME || d.tgbotApi.Self.UserName == types.TEST_BOT_USERNAME {
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      update.Message.Chat.ID,
				Text:        "Можешь делать паки",
				ReplyMarkup: startKeyboard,
			})

		} else if d.tgbotApi.Self.UserName == types.VIP_BOT_USERNAME {
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      update.Message.Chat.ID,
				Text:        "Добро пожаловать на сервер.\nЯ ⁂VIP бот, а это значит:\n ⁂ Твои запросы обрабатываются вне очереди\n ⁂ Ты можешь получать готовые эмодзи-композиции в ЛС\n ⁂ Ты можешь именовать паки без префикса (параметр name=[])\n⁂ пока что все",
				ReplyMarkup: startKeyboard,
			})
		}
		if err != nil {
			slog.Error("Failed to send message to DM", slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID), slog.String("err", err.Error()))
		}
	}
}

func (d *DripBot) startKeyboard(ctx context.Context) models.ReplyMarkup {
	kb := inline.New(d.bot).
		Row().
		Button("Создать пак", []byte("emoji"), d.onEmojiSelect).
		Button("Мои паки", []byte("packs"), d.onRemovePacksSelect)
	//Button()

	return kb
}

func (d *DripBot) menuButtons(ctx context.Context) models.ReplyMarkup {
	kb := reply.New(
		reply.WithPrefix("reply_keyboard"),
		reply.IsSelective(),
		reply.IsPersistent())

	return kb.Row().
		Button("⁂ Меню", d.bot, bot.MatchTypeExact, d.onMenuSelect)

}

func (d *DripBot) onMenuSelect(ctx context.Context, b *bot.Bot, update *models.Update) {
	keyboard := d.startKeyboard(ctx)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "тут текст меню",
		ReplyMarkup: keyboard,
	})

	if err != nil {
		slog.Error("Failed to send message to DM", slog.String("username", update.Message.From.Username), slog.Int64("user_id", update.Message.From.ID), slog.String("err", err.Error()))
	}
}
