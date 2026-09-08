package main

// К4-и1 (KIOSK.md, №13): состояние сессии от сервера и что с ним делать.
//
// Команды session_start / session_end / lock / unlock идут по живому сокету
// best-effort: если WS лежал ровно в момент завершения сессии, агент остаётся
// с открытой игрой гостя и без очистки между гостями — следующий садится в
// чужой матч. Теперь сервер отвечает на каждый session_tick сообщением
// state {session_active, paused, session_id, reason}, а агент шлёт первый
// тик сразу после (ре)коннекта. reconcile — чистая таблица переходов:
// «что я знал» × «что говорит сервер» × «жив ли трекинг» → действие.

import (
	"encoding/json"
	"log"
	"sync"
)

type sessionState struct {
	Active    bool   `json:"session_active"`
	Paused    bool   `json:"paused"`
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"` // paused | afk — пока Paused
}

type stateAction int

const (
	actNone   stateAction = iota
	actStart              // сессия началась, пока связи не было — как session_start
	actEnd                // сессии нет, а игра/знание о сессии живы — stop + cleanup, один раз
	actLock               // пауза/AFK — запереть экран, игру не трогать
	actUnlock             // пауза снята
)

var stateActionRu = map[stateAction]string{
	actNone: "ничего", actStart: "сессия началась без session_start", actEnd: "сессии нет — закрыть запущенное и очистить",
	actLock: "пауза — запереть", actUnlock: "пауза снята",
}

// reconcile — таблица переходов (чистая, тест). prev — последнее известное
// состояние (nil = агент только запустился и ничего не знает), cur — ответ
// сервера, tracking — агент держит хотя бы одну запущенную игру.
//
//	сервер: нет сессии  + трекинг жив           → actEnd (обрыв в момент конца)
//	сервер: нет сессии  + знали «сессия идёт»   → actEnd (пропущен session_end)
//	сервер: нет сессии  + ничего не знали/пусто → actNone
//	сервер: сессия идёт + не знали / другая     → actStart (пропущен session_start)
//	сервер: пауза       + не были на паузе      → actLock
//	сервер: не пауза    + были на паузе         → actUnlock
func reconcile(prev *sessionState, cur sessionState, tracking bool) stateAction {
	if !cur.Active {
		if tracking || (prev != nil && prev.Active) {
			return actEnd
		}
		return actNone
	}
	newSession := prev == nil || !prev.Active || prev.SessionID != cur.SessionID
	if newSession {
		if cur.Paused {
			return actLock // сразу в заперт: экран должен стоять, игру не трогаем
		}
		return actStart
	}
	if cur.Paused && !prev.Paused {
		return actLock
	}
	if !cur.Paused && prev.Paused {
		return actUnlock
	}
	return actNone
}

// stateKeeper — последнее известное состояние; команды сервера тоже его
// двигают, чтобы state после уже выполненного session_end не запускал
// очистку второй раз.
type stateKeeper struct {
	mu   sync.Mutex
	prev *sessionState
}

func (k *stateKeeper) set(s sessionState) {
	k.mu.Lock()
	defer k.mu.Unlock()
	cp := s
	k.prev = &cp
}

func (k *stateKeeper) update(fn func(s *sessionState)) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.prev == nil {
		k.prev = &sessionState{}
	}
	fn(k.prev)
}

// apply — сверить cur с известным, запомнить cur, вернуть действие.
func (k *stateKeeper) apply(cur sessionState, tracking bool) stateAction {
	k.mu.Lock()
	defer k.mu.Unlock()
	act := reconcile(k.prev, cur, tracking)
	cp := cur
	k.prev = &cp
	return act
}

// onState — обработчик команды state: сверка и действие.
func (a *agent) onState(payload json.RawMessage) {
	var cur sessionState
	if err := json.Unmarshal(payload, &cur); err != nil {
		log.Printf("state: не разобрал %s: %v", string(payload), err)
		return
	}
	tracking := len(a.track.snapshot()) > 0
	act := a.state.apply(cur, tracking)
	if act == actNone {
		return
	}
	log.Printf("state: сессия=%v пауза=%v трекинг=%v → %s", cur.Active, cur.Paused, tracking, stateActionRu[act])
	switch act {
	case actStart:
		disableAccessibilityShortcuts()
	case actEnd:
		a.applyLockAction()
		go a.onSessionEnd()
	case actLock:
		a.applyLockAction()
	case actUnlock:
		// с lock_action=none/kiosk снимать нечего; lock_windows отпирает гость паролем
	}
}
