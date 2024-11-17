package main

import (
	"errors"
	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"log/slog"
)

var (
	ErrWidthInvalid        = errors.New("width must be between 1 and 128")
	ErrFileOfInvalidType   = errors.New("file of invalid type")
	ErrGetFileFromTelegram = errors.New("get file from telegram failed")
	ErrFileDownloadFailed  = errors.New("ошибка в загрузке файла")
)

var (
	PackTitleTempl = " ⁂ @drip_tech"
)

const (
	TelegramPackLinkAndNameLength = 64
)

var (
	allowedMimeTypes = []string{
		"image/gif",
		"image/jpeg",
		"image/png",
		"image/webp",
		"video/mp4",
		"video/webm",
		"video/mpeg",
	}
)

type EmojiCommand struct {
	UserName string

	SetName         string
	PackLink        string
	Width           int
	BackgroundColor string
	UserID          int64
	DownloadedFile  string
	File            tgbotapi.File

	Iphone bool

	WorkingDir string

	newSet bool
}

func (e *EmojiCommand) ToSlogAttributes(attrs ...slog.Attr) []slog.Attr {
	a := []slog.Attr{
		slog.Int64("user_id", e.UserID),
		slog.String("username", e.UserName),
		slog.String("name", e.SetName),
		slog.String("pack_link", e.PackLink),
		slog.Int("width", e.Width),
		slog.String("background", e.BackgroundColor),
		slog.String("file", e.DownloadedFile),
		slog.String("file_path", e.File.FilePath),
		slog.String("file_id", e.File.FileID),
		slog.Bool("iphone", e.Iphone),
	}

	a = append(a, attrs...)

	return a
}
