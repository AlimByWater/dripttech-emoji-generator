package processing

import (
	"emoji-generator/types"
	"fmt"
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
