package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	defer cancel()

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Ошибка загрузки файла с переменными окружения: %v", err)
	}

	// Создаем бота с токеном из переменной окружения
	b, err := bot.New(os.Getenv("BOT_TOKEN"), bot.WithDefaultHandler(handler))
	if err != nil {
		log.Fatalf("Ошибка при создании бота: %v", err)
	}

	// Запускаем бота
	b.Start(ctx)

	<-ctx.Done()
	log.Println("Bot stopped")
}
