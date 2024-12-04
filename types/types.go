package types

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot/models"
)

type EmojiMeta struct {
	FileID      string `json:"file_id"`
	DocumentID  string `json:"document_id"`
	FileName    string `json:"filename"`
	Transparent bool   `json:"transparent"`
}

var (
	ErrWidthInvalid        = errors.New("width must be between 1 and 128")
	ErrFileOfInvalidType   = errors.New("file of invalid type")
	ErrGetFileFromTelegram = errors.New("get file from telegram failed")
	ErrFileDownloadFailed  = errors.New("ошибка в загрузке файла")

	ErrInvalidFormat = fmt.Errorf("неверный формат параметра, используйте формат param=value или param=[value]")
	ErrUnknownParam  = fmt.Errorf("неизвестный параметр")
	ErrInvalidWidth  = fmt.Errorf("ширина должна быть числом")
	ErrInvalidIphone = fmt.Errorf("параметр iphone должен быть true или false")
)

var (
	PackTitleTempl = " ⁂ @drip_tech"
)

const (
	TelegramPackLinkAndNameLength = 64
)

var (
	AllowedMimeTypes = []string{
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
	BackgroundBlend string
	BackgroundSim   string
	UserID          int64
	DownloadedFile  string
	File            *models.File

	QualityValue int

	RawInitCommand string
	Iphone         bool

	WorkingDir string

	NewSet bool
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

var ArgAlias = map[string]string{
	// width aliases
	"width":  "width",
	"w":      "width",
	"ширина": "width",
	"ш":      "width",

	// name aliases
	"name": "name",
	"n":    "name",
	"имя":  "name",
	"и":    "name",

	// background aliases
	"background": "background",
	"bg":         "background",
	"b":          "background",
	"фон":        "background",
	"ф":          "background",

	"background_blend": "background_blend",
	"bb":               "background_blend",
	"b_blend":          "background_blend",

	"background_sim": "background_sim",
	"bs":             "background_sim",
	"b_sim":          "background_sim",

	// link aliases
	"link":   "link",
	"l":      "link",
	"ссылка": "link",
	"с":      "link",

	// iphone aliases
	"iphone": "iphone",
	"ip":     "iphone",
	"айфон":  "iphone",
	"а":      "iphone",
}

var ColorMap = map[string]string{
	"black":   "0x000000",
	"white":   "0xFFFFFF",
	"red":     "0xFF0000",
	"green":   "0x00FF00",
	"blue":    "0x0000FF",
	"yellow":  "0xFFFF00",
	"cyan":    "0x00FFFF",
	"magenta": "0xFF00FF",
	"gray":    "0x808080",
	"purple":  "0x800080",
	"orange":  "0xFFA500",
	"brown":   "0x8B4513",
	"pink":    "0xFFC0CB",

	"черный":     "0x000000",
	"белый":      "0xFFFFFF",
	"красный":    "0xFF0000",
	"зеленый":    "0x00FF00",
	"зелёный":    "0x00FF00",
	"синий":      "0x0000FF",
	"желтый":     "0xFFFF00",
	"жёлтый":     "0xFFFF00",
	"голубой":    "0x00FFFF",
	"пурпурный":  "0xFF00FF",
	"серый":      "0x808080",
	"фиолетовый": "0x800080",
	"оранжевый":  "0xFFA500",
	"коричневый": "0x8B4513",
	"розовый":    "0xFFC0CB",
}

type EmojiPack struct {
	ID             int64     `db:"id"`
	CreatorID      int64     `db:"creator_id"`
	PackName       string    `db:"pack_name"`
	FileURL        string    `db:"file_url"`
	PackLink       *string   `db:"pack_link"`
	InitialCommand *string   `db:"initial_command"`
	Bot            string    `db:"bot"`
	EmojiCount     int       `db:"emoji_count"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
