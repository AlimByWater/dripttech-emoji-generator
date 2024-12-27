package userbot

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

var (
	ErrChatNotFound       = errors.New("chat not found")
	ErrAccessHashNotFound = errors.New("access hash not found")
)

// CreateProgressMessage создает новое сообщение о прогрессе и сохраняет его
func (u *User) CreateProgressMessage(ctx context.Context, chatID int64, replyTo int, text string) int {
	if chatID <= 0 {
		slog.Warn("Invalid chatID", slog.Int64("chatID", chatID))
		return 0
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	sender := message.NewSender(tg.NewClient(u.client))

	// Получаем peer для чата
	peer := u.client.PeerStorage.GetInputPeerById(chatID)

	// Отправляем сообщение
	res, err := sender.To(peer).Reply(replyTo).StyledText(ctx, styling.Plain(text))
	if err != nil {
		slog.Error("Failed to send progress message",
			slog.String("err", err.Error()),
			slog.Int64("chatID", chatID))
		return 0
	}

	// Извлекаем ID сообщения из результата
	msgID, err := extractMessageID(res)
	if err != nil {
		slog.Error("Failed to extract message ID",
			slog.String("err", err.Error()),
			slog.Int64("chatID", chatID))
		return 0
	}

	chatStr := strconv.FormatInt(chatID, 10)

	// Сохраняем ID сообщения с составным ключом
	key := chatStr + ":" + strconv.Itoa(msgID)
	u.progressMessages.Store(key, msgID)

	return msgID
}

// UpdateProgressMessage обновляет существующее сообщение о прогрессе
func (u *User) UpdateProgressMessage(ctx context.Context, chatID int64, msgID int, text string) {
	if chatID <= 0 || msgID <= 0 {
		slog.Warn("Invalid chatID or msgID",
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	chatStr := strconv.FormatInt(chatID, 10)

	// Получаем ID сообщения по составному ключу
	key := chatStr + ":" + strconv.Itoa(msgID)
	_, ok := u.progressMessages.Load(key)
	if !ok {
		slog.Warn("Progress message not found in storage",
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

	sender := message.NewSender(tg.NewClient(u.client))

	// Получаем peer для чата
	peer := u.client.PeerStorage.GetInputPeerById(chatID)

	// Обновляем сообщение
	_, err := sender.To(peer).Edit(msgID).StyledText(ctx, styling.Plain(text))
	if err != nil {
		slog.Error("Failed to update progress message",
			slog.String("err", err.Error()),
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

}

// DeleteProgressMessage удаляет сообщение о прогрессе
func (u *User) DeleteProgressMessage(ctx context.Context, chatID int64, msgID int) {
	if chatID <= 0 || msgID <= 0 {
		slog.Warn("Invalid chatID or msgID",
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	chatStr := strconv.FormatInt(chatID, 10)

	// Получаем ID сообщения по составному ключу
	key := chatStr + ":" + strconv.Itoa(msgID)
	_, ok := u.progressMessages.Load(key)
	if !ok {
		slog.Warn("Progress message not found in storage",
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

	sender := message.NewSender(tg.NewClient(u.client))

	// Получаем peer для чата
	peer := u.client.PeerStorage.GetInputPeerById(chatID)

	// Удаляем сообщение
	_, err := sender.To(peer).Revoke().Messages(ctx, msgID)
	if err != nil {
		slog.Error("Failed to delete progress message",
			slog.String("err", err.Error()),
			slog.Int64("chatID", chatID),
			slog.Int("msgID", msgID))
	}

	// Удаляем из хранилища
	u.progressMessages.Delete(key)

}

// extractMessageID извлекает ID сообщения из UpdatesClass
func extractMessageID(updates tg.UpdatesClass) (int, error) {
	switch u := updates.(type) {
	case *tg.Updates:
		if len(u.Updates) > 0 {
			if msg, ok := u.Updates[0].(*tg.UpdateMessageID); ok {
				return msg.ID, nil
			}
		}
	case *tg.UpdateShortSentMessage:
		return u.ID, nil
	}
	return 0, errors.New("failed to extract message ID")
}
