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
//
// З0-и3 (№28): персоналу (admin/owner) после серии не замок, а темп — одна
// попытка в loginStaffPace, верный пароль проходит после паузы. Иначе гость
// с киоска вводил 10 неверных паролей для boss и запирал владельца на 15
// минут — и повторял сколько угодно. Замок по паре «логин + IP» не спасает:
// киоски и стойка клуба выходят в интернет с одного адреса. Серия неудач
// персонала — одна строка login_failed в журнале владельца (audit.go).

import (
	"strings"
	"sync"
	"time"
)

const (
	loginMaxFails   = 10               // неудачных попыток подряд в окне
	loginFailWindow = 15 * time.Minute // окно счёта и срок блокировки
	loginStaffPace  = 30 * time.Second // З0-и3: персонал после серии — одна попытка за столько
)

// loginSeries — неудачи одного логина внутри окна.
type loginSeries struct {
	at       []time.Time
	reported bool // З0-и3: строка login_failed про эту серию уже в журнале
}

type loginGuard struct {
	mu    sync.Mutex
	fails map[string]*loginSeries // ключ — логин в нижнем регистре
}

func newLoginGuard() *loginGuard { return &loginGuard{fails: map[string]*loginSeries{}} }

var loginGuardian = newLoginGuard()

func loginKey(login string) string { return strings.ToLower(strings.TrimSpace(login)) }

// prune — оставить только попытки внутри окна; серия остыла целиком — забыть
// её вместе с отметкой о строке журнала.
func (g *loginGuard) prune(key string, now time.Time) *loginSeries {
	s := g.fails[key]
	if s == nil {
		return nil
	}
	kept := s.at[:0]
	for _, t := range s.at {
		if now.Sub(t) < loginFailWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(g.fails, key)
		return nil
	}
	s.at = kept
	return s
}

// locked — нельзя ли сейчас проверять пароль этого логина и сколько ждать.
// Гость и неизвестный логин: loginMaxFails неудач за окно — замок до остывания
// окна. Персонал (staff): после серии — пауза loginStaffPace от последней неудачи.
func (g *loginGuard) locked(login string, staff bool, now time.Time) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.prune(loginKey(login), now)
	if s == nil || len(s.at) < loginMaxFails {
		return false, 0
	}
	if staff {
		wait := loginStaffPace - now.Sub(s.at[len(s.at)-1])
		return wait > 0, max(wait, 0)
	}
	// Окно считается от самой старой попытки из последних loginMaxFails.
	oldest := s.at[len(s.at)-loginMaxFails]
	return true, loginFailWindow - now.Sub(oldest)
}

// fail — запомнить неудачную попытку: сколько их в серии и report=true ровно
// один раз за серию персонала — когда она дошла до loginMaxFails (З0-и3).
func (g *loginGuard) fail(login string, staff bool, now time.Time) (n int, report bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := loginKey(login)
	s := g.prune(key, now)
	if s == nil {
		s = &loginSeries{}
		g.fails[key] = s
	}
	s.at = append(s.at, now)
	if staff && !s.reported && len(s.at) >= loginMaxFails {
		s.reported = true
		return len(s.at), true
	}
	return len(s.at), false
}

// reset — успешный вход или сброс пароля админом: счётчик обнуляется.
func (g *loginGuard) reset(login string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, loginKey(login))
}
