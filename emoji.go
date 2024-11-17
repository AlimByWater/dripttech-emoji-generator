package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const outputDirTemplate = "/tmp/%s"

// clearDirectory очищает указанную директорию
func clearDirectory(directory string) error {
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		err = os.RemoveAll(filepath.Join(directory, name))
		if err != nil {
			log.Printf("Не удалось удалить %s. Причина: %v", name, err)
		}
	}
	return nil
}

// getVideoDimensions получает размеры видео используя ffprobe
func getVideoDimensions(inputVideo string) (width, height int, err error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-count_packets",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		inputVideo)

	output, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	_, err = fmt.Sscanf(string(output), "%d,%d", &width, &height)
	if err != nil {
		return 0, 0, err
	}

	return width, height, nil
}

// processVideo обрабатывает видео и создает тайлы
func processVideo(args *EmojiCommand) ([]string, error) {

	width, height, err := getVideoDimensions(args.DownloadedFile)
	if err != nil {
		return nil, err
	}

	fmt.Println(width, height)

	tileWidth := 100
	tileHeight := 100
	tilesX := width / tileWidth
	tilesY := height / tileHeight

	var createdFiles []string

	for j := 0; j < tilesY; j++ {
		for i := 0; i < tilesX; i++ {
			x := i * tileWidth
			y := j * tileHeight
			outputFile := filepath.Join(args.WorkingDir, fmt.Sprintf("emoji_%d_%d.webm", j, i))

			var vfArgs []string
			vfArgs = append(vfArgs, fmt.Sprintf("crop=%d:%d:%d:%d", tileWidth, tileHeight, x, y))
			if args.BackgroundColor != "" {
				vfArgs = append(vfArgs, fmt.Sprintf("colorkey=%s:similarity=0.2:blend=0.1", args.BackgroundColor))
			}
			vfArgs = append(vfArgs, fmt.Sprintf("setsar=1:1"))

			cmd := exec.Command("ffmpeg",
				"-i", args.DownloadedFile,
				"-c:v", "libvpx-vp9",
				"-vf", strings.Join(vfArgs, ","),
				"-crf", "24",
				"-b:v", "0",
				"-b:a", "256k",
				"-t", "2.99",
				"-r", "10",
				"-auto-alt-ref", "1",
				"-metadata:s:v:0", "alpha_mode=1",
				"-an",
				outputFile)

			if err := cmd.Run(); err != nil {
				log.Printf("Ошибка при обработке тайла %d_%d: %v", j, i, err)
				continue
			}
			createdFiles = append(createdFiles, outputFile)
		}
	}

	return createdFiles, nil
}

// addEmojis создает новый набор стикеров
func (b *Bot) addEmojis(ctx context.Context, args *EmojiCommand, emojiFiles []string) (*tgbotapi.StickerSet, error) {
	if len(emojiFiles) == 0 {
		slog.LogAttrs(ctx, slog.LevelError, "нет файлов для создания набора", args.ToSlogAttributes()...)
		return nil, fmt.Errorf("нет файлов для создания набора")
	}

	var stickers []tgbotapi.InputSticker
	var updstickers []tgbotapi.UploadStickerConfig

	for _, emojiFile := range emojiFiles {
		file := tgbotapi.FilePath(emojiFile)
		fmt.Println(emojiFile)

		updsticker := tgbotapi.UploadStickerConfig{
			UserID:        args.UserID,
			Sticker:       tgbotapi.RequestFile{Name: emojiFile, Data: file},
			StickerFormat: "video",
		}
		updstickers = append(updstickers, updsticker)
	}

	for _, updsticker := range updstickers {
		resp, err := b.api.Request(updsticker)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "upload sticker", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			if errors.Is(err, tgbotapi.Error{}) {
			} // TODO обработать ошибку в случае переполнения пака, ...
			break
		}

		var uploadedSticker = struct {
			FileID string `json:"file_id"`
		}{}

		if resp.Ok {
			err := json.Unmarshal(resp.Result, &uploadedSticker)
			if err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "unmarshal uploadedsticker", args.ToSlogAttributes(slog.String("err", err.Error()))...)
				break
			}

			sticker := tgbotapi.InputSticker{
				EmojiList: []string{"🎥"},
				Sticker:   tgbotapi.RequestFile{Name: uploadedSticker.FileID, Data: tgbotapi.FileID(uploadedSticker.FileID)},
			}
			stickers = append(stickers, sticker)
		}
	}

	if args.newSet {
		addConfig := tgbotapi.NewStickerSetConfig{
			UserID:        args.UserID,
			Name:          args.PackLink,
			Title:         args.SetName,
			StickerFormat: "video",
			StickerType:   "custom_emoji",
			Stickers:      stickers[:1],
		}
		_, err := b.Request(addConfig)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "new sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
			return nil, fmt.Errorf("не удалось создать пак")
		}
	}

	if args.newSet {
		for _, sticker := range stickers[1:] {
			_, err := b.Request(tgbotapi.AddStickerConfig{
				UserID:  args.UserID,
				Sticker: sticker,
				Name:    args.PackLink,
			})
			if err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "add sticker", args.ToSlogAttributes(slog.String("err", err.Error()))...)
				if errors.Is(err, tgbotapi.Error{}) {
				} // TODO обработать ошибку в случае переполнения пака, ...
				break
			}
		}
	} else {
		for _, sticker := range stickers {
			_, err := b.Request(tgbotapi.AddStickerConfig{
				UserID:  args.UserID,
				Sticker: sticker,
				Name:    args.PackLink,
			})
			if err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "add to sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
				if errors.Is(err, tgbotapi.Error{}) {
				} // TODO обработать ошибку в случае переполнения пака, ...
				break
			}
		}
	}

	resp, err := b.Request(tgbotapi.GetStickerSetConfig{
		Name: args.PackLink,
	})
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "get sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
		return nil, fmt.Errorf("get sticker set: %w", err)
	}

	var stickerSet tgbotapi.StickerSet

	err = json.Unmarshal(resp.Result, &stickerSet)
	if err != nil {
		slog.LogAttrs(ctx, slog.LevelError, "unmarshal sticker set", args.ToSlogAttributes(slog.String("err", err.Error()))...)
		return nil, fmt.Errorf("unmarshal sticker set: %w", err)
	}

	return &stickerSet, nil
}
