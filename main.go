package main

import (
	"context"
	"emoji-generator/db"
	"emoji-generator/httpclient"
	userbot "emoji-generator/mtproto"
	"github.com/go-telegram/bot/models"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/go-telegram/bot"
	"golang.org/x/time/rate"

	"github.com/joho/godotenv"
)

var userBot *userbot.User
var tgbotApi *tgbotapi.BotAPI
var wg sync.WaitGroup

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	defer cancel()

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Ошибка загрузки файла с переменными окружения: %v", err)
	}

	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := db.Init()
	if err != nil {
		log.Fatalf("Ошибка инициализации БД: %v", err)
	}
	slog.Info("postgres initialized")

	rl := rate.NewLimiter(rate.Every(1*time.Second), 30)
	c := httpclient.NewClient(rl)

	userBot = userbot.NewBot()
	err = userBot.Init(ctx)
	if err != nil {
		log.Fatalf("Ошибка при создании бота: %v", err)
	}
	slog.Info("UserBot initialized")

	b, err := bot.New(os.Getenv("BOT_TOKEN"),
		bot.WithDefaultHandler(handlerWithWG),
		bot.WithHTTPClient(time.Minute, c))
	if err != nil {
		log.Fatalf("Ошибка при создании бота: %v", err)
	}

	tgbotApi, err = tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatalf("Ошибка при создании бота tgbotapi: %v", err)
	}

	tgbotApi.StopReceivingUpdates()

	// Запускаем бота
	b.Start(ctx)

	<-ctx.Done()
	slog.Info("Получен сигнал завершения, ожидаем завершения текущих задач...")

	// Ждем завершения всех текущих задач
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Ждем завершения задач с таймаутом
	select {
	case <-done:
		slog.Info("Все задачи успешно завершены")
	case <-time.After(30 * time.Second):
		slog.Warn("Превышено время ожидания завершения задач")
	}

	userBot.Shutdown(ctx)
	db.Postgres.Shutdown()
	slog.Info("Bot stopped")
}

func handlerWithWG(ctx context.Context, b *bot.Bot, update *models.Update) {
	wg.Add(1)
	defer wg.Done()

	handler(ctx, b, update)
}
