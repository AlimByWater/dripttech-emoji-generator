package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func RoundDimensions(width, height int) (int, int) {
	newWidth := (width / 100) * 100
	newHeight := int(float64(height) * (float64(newWidth) / float64(width)))
	if newHeight%2 != 0 {
		newHeight--
	}
	return newWidth, newHeight
}

func RoundUpTo100(num int) int {
	return ((num + 99) / 100) * 100
}

func DimensionToNewWidth(width, height, newWidth int) (int, int) {
	newHeight := int(float64(height) * (float64(newWidth) / float64(width)))
	return newWidth, newHeight
}

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

type tile struct {
	OutputFile string
	FFmpegArgs []string
	Position   int
}

type processResult struct {
	filename string
	position int
	err      error
}

func processVideo(args *EmojiCommand) ([]string, error) {
	width, height, err := getVideoDimensions(args.DownloadedFile)
	if err != nil {
		return nil, err
	}

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

	originalHeight := height // Сохраняем исходную высоту
	//height = RoundUpTo100(height)
	lastRowHeight := originalHeight % 100 // Высота последнего ряда до округления

	tileWidth := 100
	tileHeight := 100
	tilesX := width / tileWidth
	tilesY := height / tileHeight
	if lastRowHeight > 0 {
		tilesY++
	}

	baseFFmpegArgs := []string{
		"-i", args.DownloadedFile,
		"-c:v", "libvpx-vp9",
		"-crf", "24",
		"-b:v", "0",
		"-b:a", "256k",
		"-t", "3.0",
		"-r", "10",
		"-auto-alt-ref", "1",
		"-metadata:s:v:0", "alpha_mode=1",
		"-an",
	}

	jobs := make(chan tile, tilesX*tilesY)
	results := make(chan processResult, tilesX*tilesY)

	numWorkers := 4
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(jobs, results, &wg)
	}

	go func() {
		position := 0
		for j := 0; j < tilesY; j++ {
			for i := 0; i < tilesX; i++ {
				outputFile := filepath.Join(args.WorkingDir, fmt.Sprintf("emoji_%d_%d.webm", j, i))

				var vfArgs []string

				//fmt.Println(tilesY, tileWidth, tileHeight, lastRowHeight, height, width)

				if j == tilesY-1 && lastRowHeight > 0 {
					// 			 0xF5DEB3
					padColor := "#04F404@0.1" // По умолчанию прозрачный
					if args.BackgroundColor != "" {
						padColor = args.BackgroundColor
					} else if args.BackgroundBlend != "" {
						args.BackgroundColor = padColor
					}
					vfArgs = []string{
						fmt.Sprintf("crop=%d:%d:%d:%d", tileWidth, lastRowHeight, i*tileWidth, j*tileHeight),
						fmt.Sprintf("scale=100:%d", lastRowHeight),
						fmt.Sprintf("pad=100:100:%d:0:color=%s", 100-lastRowHeight, padColor),
					}
				} else {
					vfArgs = []string{
						fmt.Sprintf("crop=%d:%d:%d:%d", tileWidth, tileHeight, i*tileWidth, j*tileHeight),
					}
				}

				if args.BackgroundColor != "" {
					vfArgs = append(vfArgs, fmt.Sprintf("colorkey=%s:similarity=%s:blend=%s", args.BackgroundColor, args.BackgroundSim, args.BackgroundBlend))
				}
				vfArgs = append(vfArgs, "setsar=1:1")
				ffmpegArgs := make([]string, len(baseFFmpegArgs))
				copy(ffmpegArgs, baseFFmpegArgs)
				ffmpegArgs = append(ffmpegArgs, "-vf", strings.Join(vfArgs, ","), outputFile)

				jobs <- tile{
					OutputFile: outputFile,
					FFmpegArgs: ffmpegArgs,
					Position:   position,
				}
				position++
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	resultSlice := make([]string, tilesX*tilesY)
	var errors []error

	for result := range results {
		if result.err != nil {
			errors = append(errors, result.err)
			log.Printf("Error during processing: %v", result.err)
			continue
		}
		resultSlice[result.position] = result.filename
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("encountered %d errors during processing", len(errors))
	}

	// Убираем пустые элементы из результата
	var finalResults []string
	for _, res := range resultSlice {
		if res != "" {
			finalResults = append(finalResults, res)
		}
	}

	return finalResults, nil
}

func worker(jobs <-chan tile, results chan<- processResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for tile := range jobs {
		cmd := exec.Command("ffmpeg", tile.FFmpegArgs...)
		err := cmd.Run()
		results <- processResult{
			filename: tile.OutputFile,
			position: tile.Position,
			err:      err,
		}
	}
}

const outputDirTemplate = "/tmp/%s"

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
