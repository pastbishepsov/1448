package main

// З20-и2 (OWNER.md, трек З; №57 «Где не установлено»): агент сообщает серверу, какие приложения каталога у него
// запустятся. Тот же снимок, что у GET /apps-status, уходит по WS типом apps_status (сервер хранит его с З20-и1):
// сразу после (пере)подключения, потом — как только снимок изменился (сверка раз в 15 с, без похода в сеть: снимок
// берётся из кэша доступности) и без изменений — не реже раза в 10 минут. Компактно: только available и reason; kind,
// checked_at и запущенное серверу не нужны. Связь упала — цикл умирает вместе с соединением, runWS поднимет новый.

import (
	"bytes"
	"encoding/json"
	"time"
)

const (
	appsReportCheck = 15 * time.Second // как часто сверять снимок с отправленным
	appsReportEvery = 10 * time.Minute // без изменений — не реже
)

// compactApp — приложение в снимке для сервера.
type compactApp struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// compactApps — снимок для сервера: только available и reason.
func compactApps(snap map[string]appStatus) map[string]compactApp {
	out := make(map[string]compactApp, len(snap))
	for id, st := range snap {
		out[id] = compactApp{Available: st.Available, Reason: st.Reason}
	}
	return out
}

// appsReportPayload — {"apps": {...}}; ключи map encoding/json пишет по порядку — одинаковый снимок даёт одинаковые
// байты, по ним и сверяемся.
func appsReportPayload(apps map[string]compactApp) json.RawMessage {
	b, _ := json.Marshal(struct {
		Apps map[string]compactApp `json:"apps"`
	}{apps})
	return b
}

// runAppsReport — цикл отчёта: сразу, потом раз в check — если снимок изменился, и не реже every. Чистая функция над
// snapshot и send — тестируется на виртуальном времени (appsreport_test.go, testing/synctest).
func runAppsReport(check, every time.Duration, stop <-chan struct{}, snapshot func() map[string]compactApp,
	send func(json.RawMessage) error) {
	var last json.RawMessage
	var lastAt time.Time
	report := func(force bool) error {
		payload := appsReportPayload(snapshot())
		if !force && bytes.Equal(payload, last) && time.Since(lastAt) < every {
			return nil
		}
		if err := send(payload); err != nil {
			return err
		}
		last, lastAt = payload, time.Now()
		return nil
	}
	if err := report(true); err != nil {
		return
	}
	t := time.NewTicker(check)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := report(false); err != nil {
				return
			}
		}
	}
}

// appsReport — цикл отчёта на одно соединение (запускает runWS рядом с heartbeat).
func (a *agent) appsReport(stop <-chan struct{}) {
	runAppsReport(appsReportCheck, appsReportEvery, stop,
		func() map[string]compactApp { return compactApps(a.avail.snapshot(a.allApps())) },
		func(p json.RawMessage) error { return a.wsSend(wsMessage{Type: "apps_status", Payload: p}) })
}
