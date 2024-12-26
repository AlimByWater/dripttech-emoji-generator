package main

import (
	"context"
	"emoji-generator/db"
	userbot "emoji-generator/mtproto"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	slog.Info("Starting Bots...")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	defer cancel()

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Error loading env file: %v", err)
	}

	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := db.Init()
	if err != nil {
		log.Fatalf("Error initializing DB: %v", err)
	}
	slog.Info("postgres initialized")

	// Initialize UserBot
	userBot := userbot.NewBot()
	err = userBot.Init(ctx)
	if err != nil {
		log.Fatalf("Error initializing UserBot: %v", err)
	}
	slog.Info("UserBot initialized")

	var bots []*DripBot

	if os.Getenv("ENV") == "test" {
		var testBot *DripBot
		if testToken := os.Getenv("TEST_BOT_TOKEN"); testToken != "" {
			testBot, err = NewDripBot(testToken, userBot)
			if err != nil {
				log.Fatalf("Error creating test bot: %v", err)
			}
			slog.Info("Test bot initialized")
		}

		bots = append(bots, testBot)
	} else {
		// Initialize production bot
		prodBot, err := NewDripBot(os.Getenv("BOT_TOKEN"), userBot)
		if err != nil {
			log.Fatalf("Error creating production bot: %v", err)
		}
		bots = append(bots, prodBot)
	}

	for _, bot := range bots {
		go bot.Start(ctx)
	}

	<-ctx.Done()
	slog.Info("Received shutdown signal, waiting for current tasks to complete...")

	// Shutdown bots
	for _, bot := range bots {
		bot.Shutdown(ctx)
	}

	userBot.Shutdown(ctx)
	db.Postgres.Shutdown()
	slog.Info("All bots stopped")
}
