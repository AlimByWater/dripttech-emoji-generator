package bots

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var Manager *manager

type manager struct {
	Bots map[string]*DripBot
	mu   sync.RWMutex
}

func InitializeAllBots(userbot UserBot) error {
	var err error
	bots := make(map[string]*DripBot)
	if os.Getenv("ENV") == "test" {
		var testBot *DripBot
		if testToken := os.Getenv("TEST_BOT_TOKEN"); testToken != "" {
			testBot, err = NewDripBot(testToken, userbot)
			if err != nil {
				return fmt.Errorf("error creating test bot: %w", err)
			}
			slog.Info("Test bot initialized")
		}

		bots[testBot.tgbotApi.Self.UserName] = testBot

	} else {
		// Initialize production bot
		prodBot, err := NewDripBot(os.Getenv("BOT_TOKEN"), userbot)
		if err != nil {
			return fmt.Errorf("error creating production bot: %w", err)
		}
		slog.Info("Production bot initialized")
		vipBot, err := NewDripBot(os.Getenv("VIP_BOT_TOKEN"), userbot)
		if err != nil {
			prodBot.Shutdown(context.Background())
			return fmt.Errorf("error creating vip bot: %w", err)
		}

		slog.Info("VIP bot initialized")
		bots[prodBot.tgbotApi.Self.UserName] = prodBot
		bots[vipBot.tgbotApi.Self.UserName] = vipBot
	}

	Manager = &manager{
		Bots: bots,
	}

	return nil
}

func Start(ctx context.Context) {
	for _, bot := range Manager.Bots {
		go bot.Start(ctx)
	}
}

func Stop(ctx context.Context) {
	Manager.mu.Lock()
	defer Manager.mu.Unlock()
	for _, bot := range Manager.Bots {

		bot.Shutdown(ctx)
	}
}

func (m *manager) GetBotByUsername(username string) *DripBot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Bots[username]
}

// GetMainBotUsername возвращает имя основного бота.
// Временная функция-хелпер
func GetMainBotUsername() string {
	if os.Getenv("ENV") == "test" {
		return "optimus_polygon_bot"
	}

	return "driptechbot"
}
