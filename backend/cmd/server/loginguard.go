package main

// К0-и5 (KIOSK.md): защита входа от перебора пароля — на АККАУНТ, а не на IP.
//
// Лимитер по IP (ratelimit.go) обходился заголовком X-Forwarded-For, потому
// что gin по умолчанию верит всем прокси (ревью 03.09: 60 попыток подряд без
// единой 429). Доверенные прокси теперь задаются TRUSTED_PROXIES (main.go), а
// брутфорс закрывается независимо от IP: после loginMaxFails неудачных
// попыток за loginFailWindow аккаунт не пускают, пока окно не остынет.
// Успешный вход и сброс пароля админом снимают счётчик. Память процесса:
// сервер один, а после рестарта защита просто начинает считать заново.

import (
	"strings"
	"sync"
	"time"
)

const (
	loginMaxFails   = 10               // неудачных попыток подряд в окне
	loginFailWindow = 15 * time.Minute // окно счёта и срок блокировки
)

type loginGuard struct {
	mu    sync.Mutex
	fails map[string][]time.Time // ключ — логин в нижнем регистре
}

func newLoginGuard() *loginGuard { return &loginGuard{fails: map[string][]time.Time{}} }

var loginGuardian = newLoginGuard()

func loginKey(login string) string { return strings.ToLower(strings.TrimSpace(login)) }

// prune — оставить только попытки внутри окна.
func (g *loginGuard) prune(key string, now time.Time) []time.Time {
	kept := g.fails[key][:0]
	for _, t := range g.fails[key] {
		if now.Sub(t) < loginFailWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(g.fails, key)
		return nil
	}
	g.fails[key] = kept
	return kept
}

// locked — заблокирован ли логин сейчас и сколько ждать до разблокировки.
func (g *loginGuard) locked(login string, now time.Time) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	kept := g.prune(loginKey(login), now)
	if len(kept) < loginMaxFails {
		return false, 0
	}
	// Окно считается от самой старой попытки из последних loginMaxFails.
	oldest := kept[len(kept)-loginMaxFails]
	return true, loginFailWindow - now.Sub(oldest)
}

// fail — запомнить неудачную попытку.
func (g *loginGuard) fail(login string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := loginKey(login)
	g.prune(key, now)
	g.fails[key] = append(g.fails[key], now)
}

// reset — успешный вход или сброс пароля админом: счётчик обнуляется.
func (g *loginGuard) reset(login string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, loginKey(login))
}
