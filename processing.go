package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// / RoundDimensions rounds the width down to the nearest hundred and adjusts the height
// to maintain the aspect ratio. Both returned dimensions are guaranteed to be even numbers.
func RoundDimensions(width, height int) (int, int) {
	// Round width down to nearest hundred
	newWidth := (width / 100) * 100

	// Calculate new height to maintain aspect ratio
	newHeight := int(float64(height) * (float64(newWidth) / float64(width)))

	// Ensure both dimensions are even numbers
	if newHeight%2 != 0 {
		newHeight--
	}

	return newWidth, newHeight
}

func RoundUpTo100(num int) int {
	return ((num + 99) / 100) * 100
}

// DimensionTo800Width resizes dimensions to have a width of 800
// while maintaining the aspect ratio
func DimensionToNewWidth(width, height, newWidth int) (int, int) {
	newHeight := int(float64(height) * (float64(newWidth) / float64(width)))

	return newWidth, newHeight
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

	if args.Width == 0 {
		args.Width = 8
	}

	width, height, err := getVideoDimensions(args.DownloadedFile)
	if err != nil {
		return nil, err
	}

	fmt.Println(width, height)

	width, height = RoundDimensions(width, height)

	if args.Width != 0 {
		width, height = DimensionToNewWidth(width, height, args.Width*100)
	}
	var i int
	for i = width; i >= 100; i = i / 100 {
	}
	args.Width = i

	args.DownloadedFile, err = resizeVideo(args, width, height)
	if err != nil {
		return nil, err
	}

	fmt.Println(width, height)

	height = RoundUpTo100(height)
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

const outputDirTemplate = "/tmp/%s"

// removeDirectory удаляет указанную директорию
func removeDirectory(directory string) error {
	return os.RemoveAll(directory)
}

func resizeVideo(args *EmojiCommand, toWidth, toHeight int) (string, error) {
	outputFile := filepath.Join(args.WorkingDir, "resized.webm")

	cmd := exec.Command("ffmpeg",
		"-i", args.DownloadedFile,
		"-c:v", "libvpx-vp9",
		"-vf", fmt.Sprintf("scale=%d:%d", toWidth, toHeight),
		"-y",
		outputFile)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ошибка при изменении размера файла: %w", err)
	}

	return outputFile, nil
}
