package userbot

import (
	"context"
	"emoji-generator/db"
	"emoji-generator/types"
	"fmt"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/functions"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

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

func (u *User) sendMessageByBot(ctx *ext.Context, update *ext.Update, text string) {
	sender := message.NewSender(tg.NewClient(u.client))
	peer := u.client.PeerStorage.GetInputPeerById(update.EffectiveChat().GetID())

	_, err := sender.To(peer).Reply(update.EffectiveMessage.ID).Text(ctx, text)
	if err != nil {
		slog.Error("Failed to send message by userBot", slog.String("err", err.Error()))
	}
}

func (u *User) createDatabaseRecord(args *types.EmojiCommand, initialCommand string, botUsername string) (*db.EmojiPack, error) {
	emojiPack := &db.EmojiPack{
		CreatorID:      args.UserID,
		PackName:       args.SetName,
		PackLink:       &args.PackLink,
		InitialCommand: &initialCommand,
		BotName:        botUsername,
		EmojiCount:     0,
	}
	return db.Postgres.LogEmojiCommand(context.Background(), emojiPack)
}
