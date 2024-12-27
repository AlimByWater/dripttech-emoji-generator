package progress

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Message представляет сообщение с прогрессом обработки
type Message struct {
	ChatID    int64
	MessageID int
	Status    string
}

// Manager управляет сообщениями о прогрессе
type Manager struct {
	bot              *bot.Bot
	progressMessages sync.Map
}

// NewManager создает новый менеджер прогресса
func NewManager(bot *bot.Bot) *Manager {
	return &Manager{
		bot: bot,
	}
}

// SendMessage отправляет новое сообщение о прогрессе
func (m *Manager) SendMessage(ctx context.Context, chatID int64, replyToID int, status string) (*Message, error) {
	params := &bot.SendMessageParams{
		ReplyParameters: &models.ReplyParameters{
			MessageID: replyToID,
			ChatID:    chatID,
		},
		ChatID: chatID,
		Text:   status,
	}

	msg, err := m.bot.SendMessage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send progress message: %w", err)
	}

	progress := &Message{
		ChatID:    chatID,
		MessageID: msg.ID,
		Status:    status,
	}

	key := strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(msg.ID)
	m.progressMessages.Store(key, progress)
	return progress, nil
}

// DeleteMessage удаляет сообщение о прогрессе
func (m *Manager) DeleteMessage(ctx context.Context, chatID int64, msgID int) error {
	key := strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(msgID)
	progressRaw, exists := m.progressMessages.Load(key)
	if !exists {
		return nil // Если сообщения нет, это не ошибка
	}

	progress := progressRaw.(*Message)
	params := &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: progress.MessageID,
	}

	_, err := m.bot.DeleteMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to delete progress message: %w", err)
	}

	m.progressMessages.Delete(key)
	return nil
}

// UpdateMessage обновляет существующее сообщение о прогрессе
func (m *Manager) UpdateMessage(ctx context.Context, chatID int64, msgID int, status string) error {
	key := strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(msgID)
	progressRaw, exists := m.progressMessages.Load(key)
	if !exists {
		return fmt.Errorf("progress message not found for chat %d", chatID)
	}

	progress := progressRaw.(*Message)
	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: progress.MessageID,
		Text:      status,
	}

	_, err := m.bot.EditMessageText(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update progress message: %w", err)
	}

	progress.Status = status
	return nil
}
