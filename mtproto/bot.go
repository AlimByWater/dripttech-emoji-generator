package userbot

import (
	"context"
	"fmt"
	"github.com/gotd/td/telegram/message/styling"
	"log/slog"
	"math"
	"os"
	"strconv"
	"sync"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/go-telegram/bot"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

type User struct {
	client               *gotgproto.Client
	mu                   sync.Mutex
	ctx                  context.Context
	accessHash           sync.Map
	chatIdsToInternalIds sync.Map
	lastAccessHash       int64
}

func NewBot() *User {
	u := &User{
		accessHash:           sync.Map{},
		chatIdsToInternalIds: sync.Map{},
	}
	return u
}

func (u *User) Init(ctx context.Context) error {
	u.ctx = ctx

	u.accessHash.Store(int64(2400904088), int64(4253614615109204755))

	u.chatIdsToInternalIds.Store("-1002400904088_3", int64(2400904088))

	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return fmt.Errorf("ошибка парсинга APP_ID: %v", err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return fmt.Errorf("не указан APP_HASH")
	}

	phone := os.Getenv("TG_PHONE")
	if phone == "" {
		return fmt.Errorf("не указан номер телефона TG_PHONE")
	}

	var err2 error
	u.client, err2 = gotgproto.NewClient(
		appID,
		appHash,
		gotgproto.ClientTypePhone(phone),
		&gotgproto.ClientOpts{
			Session: sessionMaker.SqlSession(sqlite.Open("session/user/session.db")),
		},
	)
	if err2 != nil {
		return fmt.Errorf("ошибка создания клиента: %v", err2)
	}

	dispatcher := u.client.Dispatcher
	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, u.echo), 0)

	return nil
}

func (u *User) SendMessage(ctx context.Context, chatID string, width int, msg bot.SendMessageParams) error {
	sender := message.NewSender(tg.NewClient(u.client))

	id, ok := u.chatIdsToInternalIds.Load(chatID)
	if !ok {
		return fmt.Errorf("id не найден доступ к чату %s", chatID)
	}

	ah, ok := u.accessHash.Load(id)
	if !ok {
		return fmt.Errorf("ah не найден доступ к чату %s", chatID)
	}
	peer := &tg.InputPeerChannel{
		ChannelID:  id.(int64),
		AccessHash: ah.(int64),
	}

	var formats []message.StyledTextOption
	for i, entity := range msg.Entities {
		switch entity.Type {
		case "custom_emoji":
			documentID, err := strconv.ParseInt(entity.CustomEmojiID, 10, 64)
			if err != nil {
				return fmt.Errorf("ошибка при парсинге id документа: %v", err)
			}
			formats = append(formats, styling.CustomEmoji("🎥", documentID))
		}
		if math.Mod(float64(i+1), float64(width)) == 0 {
			formats = append(formats, styling.Plain("\n"))
		}
	}

	//_, err := sender.To(peer).SendAs(channel).ReplyMsg(msgc).StyledText(ctx, formats...)
	_, err := sender.To(peer).SendAs(peer).Reply(msg.ReplyParameters.MessageID).StyledText(ctx, formats...)
	if err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %v", err)
	}

	return nil
}

func (u *User) Shutdown(ctx context.Context) {
	slog.Info("Завершение работы User...")
	u.client.Stop()
	slog.Info("User остановлен")
}

func (u *User) echo(ctx *ext.Context, update *ext.Update) error {
	//slog.Info("new access hash", update.EffectiveUser().Username, " : ", update.EffectiveChat().GetID(), " : ", update.EffectiveChat().GetAccessHash())
	//slog.Info("input peer", update.EffectiveChat().GetInputChannel(), update.EffectiveChat().GetInputPeer())
	select {
	case <-ctx.Done():
		return nil
	default:
		if update.EffectiveChat().GetID() == -1002224939217 || update.EffectiveChat().GetID() == -1002224939217 || update.EffectiveChat().GetID() == 251636949 {
			u.mu.Lock()
			defer u.mu.Unlock()
			u.lastAccessHash = update.EffectiveChat().GetAccessHash()
		}
	}
	return nil
}
