package bots

import (
	"context"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
)

func (d *DripBot) handleInfoCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	d.sendInfoMessage(ctx, update.Message.Chat.ID, update.Message.ID)
}

func (d *DripBot) sendInfoMessage(ctx context.Context, chatID int64, replyTo int) {
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

	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   infoText,
	}

	if replyTo != 0 {
		params.ReplyParameters.MessageID = replyTo
		params.ReplyParameters.ChatID = chatID
	}

	_, err := d.bot.SendMessage(ctx, params)
	if err != nil {
		slog.Error("Failed to send info message", slog.String("err", err.Error()), slog.Int64("user_id", chatID))
	}
}
