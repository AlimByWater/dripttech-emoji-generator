package userbot

import (
	"context"
	"fmt"
	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
	"log/slog"
	"os"
	"strconv"
	"sync"
)

type User struct {
	client         *gotgproto.Client
	mu             sync.Mutex
	ctx            context.Context
	accessHash     sync.Map
	lastAccessHash int64
}

func NewBot() (*User, error) {
	accessHash := sync.Map{}
	accessHash.Store(2224939217, 5869140964584068623) //bot bot bot forum
	return &User{
		accessHash: sync.Map{},
	}, nil
}

func (u *User) Init(ctx context.Context) error {
	u.ctx = ctx

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

func (u *User) sendMessage(ctx context.Context, chatID int64, msg tgbotapi.Message) error {
	//u.mu.Lock()
	//defer u.mu.Unlock()

	sender := message.NewSender(tg.NewClient(u.client))

	ah, ok := u.accessHash.Load(chatID)
	if !ok {
		return fmt.Errorf("не найден доступ к чату %d", chatID)
	}
	peer := &tg.InputPeerChannel{
		ChannelID:  chatID,
		AccessHash: ah.(int64),
	}

	//formats := []message.StyledTextOption{
	//	styling.Plain("plaintext"), styling.Plain("\n\n"),
	//	styling.Mention("@durov"), styling.Plain("\n\n"),
	//	styling.Hashtag("#hashtag"), styling.Plain("\n\n"),
	//	styling.BotCommand("/command"), styling.Plain("\n\n"),
	//	styling.URL("https://google.org"), styling.Plain("\n\n"),
	//	styling.Email("example@example.org"), styling.Plain("\n\n"),
	//	styling.Bold("bold"), styling.Plain("\n\n"),
	//	styling.Italic("italic"), styling.Plain("\n\n"),
	//	styling.Underline("underline"), styling.Plain("\n\n"),
	//	styling.Strike("strike"), styling.Plain("\n\n"),
	//	styling.Code("fmt.Println(`Hello, World!`)"), styling.Plain("\n\n"),
	//	styling.Pre("fmt.Println(`Hello, World!`)", "Go"), styling.Plain("\n\n"),
	//	styling.TextURL("clickme", "https://google.com"), styling.Plain("\n\n"),
	//	styling.Phone("+71234567891"), styling.Plain("\n\n"),
	//	styling.Cashtag("$CASHTAG"), styling.Plain("\n\n"),
	//	styling.BankCard("5550111111111111"), styling.Plain("\n\n"),
	//}

	formats := []message.StyledTextOption{
		styling.Plain(msg.Text), styling.Plain("\n\n"),
	}

	//_, err := sender.To(peer).Text(ctx, msg)
	_, err := sender.To(peer).StyledText(ctx, formats...)
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
	select {
	case <-ctx.Done():
		return nil
	default:
		if update.EffectiveChat().GetID() == -1002224939217 || update.EffectiveChat().GetID() == -1002224939217 || update.EffectiveChat().GetID() == 251636949 {
			//slog.Info("new access hash", update.EffectiveUser().Username, " : ", update.EffectiveChat().GetID(), " : ", update.EffectiveChat().GetAccessHash())
			//fmt.Println()
			u.mu.Lock()
			defer u.mu.Unlock()
			u.lastAccessHash = update.EffectiveChat().GetAccessHash()
		}
	}
	return nil
}
