package main

import "testing"

// К4-и1: таблица переходов «знал × сервер × трекинг → действие».
func TestReconcile(t *testing.T) {
	on := &sessionState{Active: true, SessionID: "s1"}
	paused := &sessionState{Active: true, Paused: true, SessionID: "s1"}
	off := &sessionState{}
	cases := []struct {
		name     string
		prev     *sessionState
		cur      sessionState
		tracking bool
		want     stateAction
	}{
		// обрыв WS в момент завершения: агент ничего не знает, но держит игру
		{"нет сессии, игра жива (агент перезапущен)", nil, sessionState{}, true, actEnd},
		{"нет сессии, знали что идёт — пропущен session_end", on, sessionState{}, false, actEnd},
		{"нет сессии, знали что идёт, игра жива", on, sessionState{}, true, actEnd},
		{"нет сессии, и не было", off, sessionState{}, false, actNone},
		{"нет сессии, агент только запустился, пусто", nil, sessionState{}, false, actNone},
		// после выполненного session_end состояние сброшено — второй очистки нет
		{"после session_end сервер подтверждает: сессии нет", off, sessionState{}, false, actNone},
		// пропущенный session_start
		{"сессия идёт, агент не знал", nil, sessionState{Active: true, SessionID: "s1"}, false, actStart},
		{"сессия идёт, знали что нет", off, sessionState{Active: true, SessionID: "s1"}, false, actStart},
		{"другая сессия (гость сменился, оба события пропущены)", on, sessionState{Active: true, SessionID: "s2"}, false, actStart},
		{"та же сессия — ничего", on, sessionState{Active: true, SessionID: "s1"}, true, actNone},
		// пауза
		{"пауза, не были на паузе", on, sessionState{Active: true, Paused: true, SessionID: "s1", Reason: "afk"}, true, actLock},
		{"пауза, агент не знал о сессии", nil, sessionState{Active: true, Paused: true, SessionID: "s1"}, false, actLock},
		{"пауза продолжается", paused, sessionState{Active: true, Paused: true, SessionID: "s1"}, true, actNone},
		{"пауза снята", paused, sessionState{Active: true, SessionID: "s1"}, true, actUnlock},
		// сессия кончилась прямо на паузе (бюджет паузы) — закрыть, а не отпереть
		{"на паузе сервер говорит: сессии нет", paused, sessionState{}, true, actEnd},
	}
	for _, tc := range cases {
		if got := reconcile(tc.prev, tc.cur, tc.tracking); got != tc.want {
			t.Errorf("%s: %s, ожидалось %s", tc.name, stateActionRu[got], stateActionRu[tc.want])
		}
	}
}

// stateKeeper.apply запоминает cur, поэтому одно и то же расхождение
// срабатывает ровно один раз.
func TestStateKeeperOnce(t *testing.T) {
	var k stateKeeper
	k.set(sessionState{Active: true, SessionID: "s1"})
	if act := k.apply(sessionState{}, false); act != actEnd {
		t.Fatalf("первый state без сессии: %s, ожидалось actEnd", stateActionRu[act])
	}
	if act := k.apply(sessionState{}, false); act != actNone {
		t.Fatalf("повторный state без сессии: %s, ожидалось ничего", stateActionRu[act])
	}
	// команды сервера двигают то же знание
	k.update(func(s *sessionState) { s.Active, s.Paused = true, true })
	if act := k.apply(sessionState{Active: true, Paused: true}, true); act != actNone {
		t.Fatalf("после lock state «пауза» не должен запирать второй раз: %s", stateActionRu[act])
	}
}

// К4-и3: полный каталог агент просит явно, токен — заголовком.
func TestCatalogFullURLAndHeaders(t *testing.T) {
	if catalogFullURL("http://s/api/v1/catalog") != "http://s/api/v1/catalog?full=1" ||
		catalogFullURL("http://s/api/v1/catalog?x=1") != "http://s/api/v1/catalog?x=1&full=1" {
		t.Error("catalogFullURL")
	}
	a := &agent{cfg: &Config{Token: "secret-1"}}
	if a.agentHeaders().Get("X-Agent-Token") != "secret-1" {
		t.Error("токен должен уходить заголовком X-Agent-Token")
	}
	a2 := &agent{cfg: &Config{}}
	if len(a2.agentHeaders()) != 0 {
		t.Error("без токена заголовков нет")
	}
}
