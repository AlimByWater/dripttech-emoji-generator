package main

import (
	"context"
	userbot "emoji-generator/mtproto"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Ошибка загрузки файла с переменными окружения: %v", err)
	}

	userBot, err := userbot.NewBot()
	if err != nil {
		log.Fatalf("Ошибка при создании бота: %v", err)
	}

	if err := userBot.Init(ctx); err != nil {
		log.Fatalf("Ошибка инициализации userbot: %v", err)
	}

	bot, err := NewBot(userBot)
	if err != nil {
		log.Fatalf("Ошибка при создании бота: %v", err)
	}
	bot.Run(ctx)

	<-ctx.Done()

	log.Println("Bot stopped")

}
