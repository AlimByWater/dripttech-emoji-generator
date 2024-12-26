package processing

import (
	"context"
	"emoji-generator/db"
	"emoji-generator/types"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	outputDirTemplate      = "/tmp/%s"
	defaultBackgroundSim   = "0.1"
	defaultBackgroundBlend = "0.1"
)

func ExtractCommandArgs(msgText, msgCaption string) string {
	var args string
	if strings.HasPrefix(msgText, "/emoji") {
		args = strings.TrimPrefix(msgText, "/emoji")
	} else if strings.HasPrefix(msgCaption, "/emoji ") {
		args = strings.TrimPrefix(msgCaption, "/emoji ")
	}
	return strings.TrimSpace(args)
}

func SetupEmojiCommand(args *types.EmojiCommand, userID int64, username string) {
	// Set default values
	if args.Width == 0 {
		args.Width = types.DefaultWidth
	}
	if args.BackgroundSim == "" {
		args.BackgroundSim = defaultBackgroundSim
	}
	if args.BackgroundBlend == "" {
		args.BackgroundBlend = defaultBackgroundBlend
	}

	if args.SetName == "" {
		args.SetName = strings.TrimSpace(types.PackTitleTempl)
	} else {
		if args.Permissions.PackNameWithoutPrefix {
			args.SetName = strings.TrimSpace(args.SetName)
		} else {
			if len(args.SetName) > types.TelegramPackLinkAndNameLength-len(types.PackTitleTempl) {
				args.SetName = args.SetName[:types.TelegramPackLinkAndNameLength-len(types.PackTitleTempl)]
			}
			args.SetName = fmt.Sprintf(`%s
%s`, args.SetName, types.PackTitleTempl)
		}

	}

	// Setup working directory and user info
	postfix := fmt.Sprintf("%d_%d", userID, time.Now().Unix())
	args.WorkingDir = fmt.Sprintf(outputDirTemplate, postfix)

	args.UserID = userID
	args.UserName = username
}

func SetupPackDetails(ctx context.Context, args *types.EmojiCommand, botUsername string) (*db.EmojiPack, error) {
	if strings.Contains(args.PackLink, botUsername) {
		return HandleExistingPack(ctx, args)
	}
	return nil, HandleNewPack(args, botUsername)
}

func HandleExistingPack(ctx context.Context, args *types.EmojiCommand) (*db.EmojiPack, error) {
	args.NewSet = false
	if strings.Contains(args.PackLink, "t.me/addemoji/") {
		splited := strings.Split(args.PackLink, ".me/addemoji/")
		args.PackLink = strings.TrimSpace(splited[len(splited)-1])
	}

	pack, err := db.Postgres.GetEmojiPackByPackLink(ctx, args.PackLink)
	if err != nil {
		return nil, err
	}
	args.SetName = ""
	return pack, nil
}

func HandleNewPack(args *types.EmojiCommand, botUsername string) error {
	args.NewSet = true
	packName := fmt.Sprintf("%s%d_by_%s", "dt", time.Now().Unix(), botUsername)
	if len(packName) > types.TelegramPackLinkAndNameLength {
		args.PackLink = args.PackLink[:len(packName)-types.TelegramPackLinkAndNameLength]
		packName = fmt.Sprintf("%s_%s", args.PackLink, botUsername)
	}
	args.PackLink = packName
	return nil
}

func ParseArgs(arg string) (*types.EmojiCommand, error) {
	var emojiArgs types.EmojiCommand
	emojiArgs.RawInitCommand = "/emoji " + arg

	if arg == "" {
		return &emojiArgs, nil
	}

	var args []string
	currentArg := ""
	inBrackets := false

	// Проходим по строке посимвольно для корректной обработки значений в скобках
	for i := 0; i < len(arg); i++ {
		switch arg[i] {
		case '[':
			inBrackets = true
		case ']':
			inBrackets = false
		case ' ':
			if !inBrackets {
				if currentArg != "" {
					args = append(args, currentArg)
					currentArg = ""
				}
			} else {
				currentArg += string(arg[i])
			}
		default:
			currentArg += string(arg[i])
		}
	}
	if currentArg != "" {
		args = append(args, currentArg)
	}

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue // Пропускаем несуществующий аргумент
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		// Определяем стандартный ключ из алиаса
		standardKey, exists := types.ArgAlias[key]
		if !exists {
			continue // Пропускаем несуществующий аргумент
		}

		// Обрабатываем аргумент в зависимости от стандартного ключа
		switch standardKey {
		case "width":
			width, err := strconv.Atoi(value)
			if err != nil {
				continue
			}
			emojiArgs.Width = width
		case "name":
			emojiArgs.SetName = strings.TrimSpace(value)
		case "background":
			emojiArgs.BackgroundColor = ColorToHex(value)
		case "background_blend":
			value = strings.ReplaceAll(value, ",", ".")
			emojiArgs.BackgroundBlend = value
		case "background_sim":
			value = strings.ReplaceAll(value, ",", ".")
			emojiArgs.BackgroundSim = value
		case "link":
			emojiArgs.PackLink = value
		case "iphone":
			if value != "true" && value != "false" {
				continue
			}
			emojiArgs.Iphone = value == "true"
		}
	}

	if (emojiArgs.BackgroundSim != "" || emojiArgs.BackgroundBlend != "") && emojiArgs.BackgroundColor == "" {
		return &emojiArgs, types.ErrInvalidBackgroundArgumentsUse
	}

	return &emojiArgs, nil
}

func ColorToHex(colorName string) string {
	if colorName == "" {
		return ""
	}
	if hex, exists := types.ColorMap[strings.ToLower(colorName)]; exists {
		return hex
	}

	// Если это уже hex формат или неизвестный цвет, возвращаем как есть
	if strings.HasPrefix(colorName, "0x") {
		return colorName
	}

	return "0x000000" // возвращаем черный по умолчанию
}
