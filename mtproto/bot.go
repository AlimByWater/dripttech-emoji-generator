package userbot

import (
	"context"
	"emoji-generator/types"
	"fmt"
	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/functions"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/go-telegram/bot"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type emojiDmHandler interface {
}

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
	u.accessHash.Store(int64(2491830452), int64(1750568581171467725))
	u.accessHash.Store(int64(2002718381), int64(3620867012521107150))

	u.chatIdsToInternalIds.Store("-1002400904088_3", int64(2400904088))
	u.chatIdsToInternalIds.Store("-1002491830452_3", int64(2491830452))
	u.chatIdsToInternalIds.Store("-1002002718381", int64(2002718381))

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
	// regex for message starting with /emoji
	emojiCmd, err := filters.Message.Regex("^/emoji")
	if err != nil {
		return fmt.Errorf("ошибка создания regex: %v", err)
	}

	dispatcher.AddHandlerToGroup(handlers.NewMessage(emojiCmd, u.emoji), 0)

	return nil
}

func (u *User) DeleteMessage(ctx context.Context, msgID int) error {
	sender := message.NewSender(tg.NewClient(u.client))

	//id, ok := u.chatIdsToInternalIds.Load(chatID)
	//if !ok {
	//	return fmt.Errorf("id не найден доступ к чату %s", chatID)
	//}
	//
	//ah, ok := u.accessHash.Load(id)
	//if !ok {
	//	return fmt.Errorf("ah не найден доступ к чату %s", chatID)
	//}
	//peer := &tg.InputPeerChannel{
	//	ChannelID:  id.(int64),
	//	AccessHash: ah.(int64),
	//}

	_, err := sender.Delete().Messages(ctx, msgID)
	//.Delete().Messages(ctx, msgID)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (u *User) SendMessage(ctx context.Context, chatID string, msg bot.SendMessageParams) error {
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
	formats = append(formats,
		styling.Plain(msg.Text),
		styling.Plain("\n"))

	_, err := sender.To(peer).SendAs(peer).Reply(msg.ReplyParameters.MessageID).StyledText(ctx, formats...)
	if err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %v", err)
	}

	return nil
}

func (u *User) SendMessageWithEmojis(ctx context.Context, chatID string, width int, packLink string, command string, emojis []types.EmojiMeta, replyTo int) error {
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

	if width < types.DefaultWidth {
		width = types.DefaultWidth
	}

	// "⠀"
	for i, emoji := range emojis {
		if i == len(emojis)-1 || i == types.MaxStickerInMessage-1 {
			break
		}
		if emoji.Transparent {
			formats = append(formats, styling.Plain("⠀⠀"))
		} else {
			documentID, err := strconv.ParseInt(emoji.DocumentID, 10, 64)
			if err != nil {
				return fmt.Errorf("ошибка при парсинге id документа: %v", err)
			}
			formats = append(formats, styling.CustomEmoji("⭐️", documentID))
		}
		if math.Mod(float64(i+1), float64(width)) == 0 {
			formats = append(formats, styling.Plain("\n"))
		}
	}

	//for i, entity := range msg.Entities {
	//	if i == len(msg.Entities)-1 {
	//		break
	//	}
	//	switch entity.Type {
	//	case "custom_emoji":
	//		documentID, err := strconv.ParseInt(entity.CustomEmojiID, 10, 64)
	//		if err != nil {
	//			return fmt.Errorf("ошибка при парсинге id документа: %v", err)
	//		}
	//		formats = append(formats, styling.CustomEmoji("⭐️", documentID))
	//	}
	//	if math.Mod(float64(i+1), float64(width)) == 0 {
	//		for i := 1; i == 8-width; i++ {
	//			formats = append(formats, styling.Plain("\n"))
	//		}
	//	}
	//}
	formats = append(formats,
		styling.Plain("\t"),
		styling.TextURL("⁂добавить", fmt.Sprintf("https://t.me/addemoji/%s", packLink)),
	)

	//_, err := sender.To(peer).SendAs(channel).ReplyMsg(msgc).StyledText(ctx, formats...)
	_, err := sender.To(peer).SendAs(peer).Reply(replyTo).NoWebpage().StyledText(ctx, formats...)
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
	slog.Info("input peer", update.EffectiveChat().GetInputChannel(), update.EffectiveChat().GetInputPeer())
	slog.Info("message", update.EffectiveMessage.ID, update.EffectiveMessage.Text)
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

func (u *User) getReplyMessage(ctx *ext.Context, chatID int64, replyMsgID int) (*tg.Message, error) {
	messages, err := functions.GetMessages(ctx, u.client.API(), u.client.PeerStorage, chatID, []tg.InputMessageClass{&tg.InputMessageID{ID: replyMsgID}})
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("message not found")
	}

	msg := functions.GetMessageFromMessageClass(messages[0])
	return msg, nil
}

func (u *User) emoji(ctx *ext.Context, update *ext.Update) error {
	workingDir := fmt.Sprintf("/tmp/%d_%d", update.EffectiveChat().GetID(), time.Now().Unix())
	filePath, err := u.downloadMedia(ctx, update, workingDir)
	if err != nil {
		return fmt.Errorf("ошибка при загрузке медиа: %v", err)
	}
	_ = filePath

	return nil
}

func (u *User) downloadMedia(ctx *ext.Context, update *ext.Update, workingDir string) (string, error) {

	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create working directory: %w", err)
	}

	var media tg.MessageMediaClass
	var ok bool
	media, ok = update.EffectiveMessage.GetMedia()
	if !ok {
		if update.EffectiveMessage.ReplyTo != nil {

			if strings.Contains(update.EffectiveMessage.ReplyTo.String(), "ReplyToMsgID:") {
				replySlice := strings.Split(update.EffectiveMessage.ReplyTo.String(), " ")
				replyMsgID := 0
				for _, m := range replySlice {
					if strings.Contains(m, "ReplyToMsgID:") {
						var err error
						replyMsgID, err = strconv.Atoi(strings.Split(m, ":")[1])
						if err != nil {
							return "", fmt.Errorf("ошибка при парсинге id сообщения: %v", err)
						}
					}
				}

				var err error
				replyMsg, err := u.getReplyMessage(ctx, update.EffectiveChat().GetID(), replyMsgID)
				if err != nil {
					return "", err
				}

				media, ok = replyMsg.GetMedia()
				if !ok {
					return "", types.ErrFileNotProvided
				}

				media.TypeName()
			} else {
				return "", types.ErrFileNotProvided
			}
		}

	}

	filename, err := GetMediaFileNameWithId(media)
	if err != nil {
		return "", fmt.Errorf("ошибка при получении имени файла: %v", err)
	}

	_, err = ctx.DownloadMedia(
		media,
		ext.DownloadOutputPath(workingDir+"/"+filename),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("ошибка при скачивании файла: %v", err)
	}

	return workingDir + "/" + filename, nil
}

func GetMediaFileNameWithId(media tg.MessageMediaClass) (string, error) {
	switch v := media.(type) {
	case *tg.MessageMediaPhoto: // messageMediaPhoto#695150d7
		f, ok := v.Photo.AsNotEmpty()
		if !ok {
			return "", fmt.Errorf("unknown media type")
		}

		return fmt.Sprintf("%d.png", f.ID), nil
	case *tg.MessageMediaDocument: // messageMediaDocument#4cf4d72d
		var (
			attr             tg.DocumentAttributeClass
			ok               bool
			filenameFromAttr *tg.DocumentAttributeFilename
			f                *tg.Document
			filename         = "undefined"
		)

		f, ok = v.Document.AsNotEmpty()
		if !ok {
			return "", fmt.Errorf("unknown media type")
		}

		for _, attr = range f.Attributes {
			filenameFromAttr, ok = attr.(*tg.DocumentAttributeFilename)
			if ok {
				filename = filenameFromAttr.FileName
			}

			videoAttr, ok := attr.(*tg.DocumentAttributeVideo)
			if ok && videoAttr.RoundMessage {
				fmt.Println(videoAttr.String())
				filename = fmt.Sprintf("round%d.mp4", f.ID)
			}

		}

		return fmt.Sprintf("%d-%s", f.ID, filename), nil
	case *tg.MessageMediaStory: // messageMediaStory#68cb6283
		f, ok := v.Story.(*tg.StoryItem)
		if !ok {
			return "", fmt.Errorf("unknown media type")
		}
		return GetMediaFileNameWithId(f.Media)
	}
	return "", fmt.Errorf("unknown media type")
}
