package main

import (
	"context"
	"emoji-generator/db"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"os"
	"os/signal"
	"testing"
)

func TestPrefix(t *testing.T) {
	t.Log(48 + 8 - 1/8)
}

func TestCreateBlankDatabaseRecord(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Ошибка загрузки файла с переменными окружения: %v", err)
	}

	err := db.Init()
	if err != nil {
		t.Fatalf("Ошибка инициализации БД: %v", err)
	}

	err = createBlankDatabaseRecord(ctx, "optimus_polygon_bot", 251636949)
	assert.Error(t, err)

}
