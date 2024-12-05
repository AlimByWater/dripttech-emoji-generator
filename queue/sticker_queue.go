package queue

import (
	"sync"
)

type StickerQueue struct {
	mu           sync.Mutex
	currentPack  string                     // текущий обрабатываемый пак
	packQueue    []string                   // очередь паков
	waitChannels map[string][]chan struct{} // каналы ожидания для каждого пака
}

func New() *StickerQueue {
	return &StickerQueue{
		waitChannels: make(map[string][]chan struct{}),
	}
}

// Acquire пытается получить доступ к обработке пака
// Возвращает true если можно сразу обрабатывать этот пак
// Если false - нужно ждать сигнала по каналу
func (sq *StickerQueue) Acquire(packLink string) (bool, chan struct{}) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	// Если очередь пустая и нет текущего пака - сразу начинаем обработку
	if sq.currentPack == "" {
		sq.currentPack = packLink
		return true, nil
	}

	// Если это текущий пак - разрешаем обработку
	if sq.currentPack == packLink {
		return true, nil
	}

	// Создаем канал ожидания для этого пака
	ch := make(chan struct{})
	sq.waitChannels[packLink] = append(sq.waitChannels[packLink], ch)

	// Добавляем пак в очередь, если его там еще нет
	if !sq.isInQueue(packLink) {
		sq.packQueue = append(sq.packQueue, packLink)
	}

	return false, ch
}

// Release освобождает текущий пак и переходит к следующему в очереди
func (sq *StickerQueue) Release(packLink string) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	// Если это не текущий пак - ничего не делаем
	if sq.currentPack != packLink {
		return
	}

	// Удаляем текущий пак из очереди
	sq.removeFromQueue(packLink)

	// Если очередь пустая - просто очищаем текущий пак
	if len(sq.packQueue) == 0 {
		sq.currentPack = ""
		return
	}

	// Берем следующий пак из очереди
	nextPack := sq.packQueue[0]
	sq.currentPack = nextPack

	// Сигнализируем всем ожидающим этого пака
	if channels, ok := sq.waitChannels[nextPack]; ok {
		for _, ch := range channels {
			close(ch)
		}
		delete(sq.waitChannels, nextPack)
	}
}

// Clear очищает все очереди
func (sq *StickerQueue) Clear() {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	// Закрываем все каналы ожидания
	for _, channels := range sq.waitChannels {
		for _, ch := range channels {
			close(ch)
		}
	}

	sq.currentPack = ""
	sq.packQueue = nil
	sq.waitChannels = make(map[string][]chan struct{})
}

func (sq *StickerQueue) isInQueue(packLink string) bool {
	for _, p := range sq.packQueue {
		if p == packLink {
			return true
		}
	}
	return false
}

func (sq *StickerQueue) removeFromQueue(packLink string) {
	newQueue := make([]string, 0, len(sq.packQueue))
	for _, p := range sq.packQueue {
		if p != packLink {
			newQueue = append(newQueue, p)
		}
	}
	sq.packQueue = newQueue
}
