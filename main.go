package main

import (
	"context"
	"emoji-generator/bots"
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

	err = bots.InitializeAllBots(userBot)
	if err != nil {
		log.Fatalf("Error initializing bots: %v", err)
	}

	bots.Start(ctx)

	<-ctx.Done()
	slog.Info("Received shutdown signal, waiting for current tasks to complete...")

	// Shutdown bots
	bots.Stop(ctx)

	userBot.Shutdown(ctx)
	db.Postgres.Shutdown()
	slog.Info("All bots stopped")
}
