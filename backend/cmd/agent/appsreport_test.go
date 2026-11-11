package main

// З20-и2: отчёт о доступности приложений серверу — формат и ритм на виртуальном времени (Go 1.25+, testing/synctest),
// как у heartbeat: минуты проходят мгновенно, без реального WS.

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestCompactAppsPayload(t *testing.T) {
	got := string(appsReportPayload(compactApps(map[string]appStatus{
		"gta5": {Available: false, Reason: reasonUnreachable, Kind: kindFile, CheckedAt: "2026-11-09T10:00:00Z"},
		"cs2":  {Available: true, Kind: kindProtocol},
		"new":  {Available: true, Reason: reasonChecking},
	})))
	want := `{"apps":{"cs2":{"available":true},"gta5":{"available":false,"reason":"path_unreachable"},` +
		`"new":{"available":true,"reason":"checking"}}}`
	if got != want {
		t.Fatalf("снимок для сервера:\n%s\nждали\n%s", got, want)
	}
	if got := string(appsReportPayload(compactApps(nil))); got != `{"apps":{}}` {
		t.Fatalf("пустой снимок: %s, ждали {\"apps\":{}}", got)
	}
}

func TestAppsReportOnConnectChangeAndEvery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		snap := map[string]compactApp{"cs2": {Available: true}}
		var sent []string
		var at []time.Duration
		start := time.Now()
		stop, done := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			runAppsReport(15*time.Second, 10*time.Minute, stop,
				func() map[string]compactApp {
					mu.Lock()
					defer mu.Unlock()
					out := map[string]compactApp{}
					for k, v := range snap {
						out[k] = v
					}
					return out
				},
				func(p json.RawMessage) error {
					mu.Lock()
					sent, at = append(sent, string(p)), append(at, time.Since(start))
					mu.Unlock()
					return nil
				})
		}()
		count := func() int { mu.Lock(); defer mu.Unlock(); return len(sent) }
		synctest.Wait()
		if count() != 1 {
			t.Fatalf("сразу после подключения отправлено %d, ждали 1", count())
		}
		time.Sleep(2 * time.Minute) // снимок не менялся — молчим
		synctest.Wait()
		if count() != 1 {
			t.Fatalf("без изменений за 2 минуты отправлено %d, ждали 1", count())
		}
		mu.Lock()
		snap["cs2"] = compactApp{Available: false, Reason: reasonMissing}
		mu.Unlock()
		time.Sleep(15 * time.Second) // изменился — уходит на ближайшей сверке
		synctest.Wait()
		if count() != 2 {
			t.Fatalf("после изменения отправлено %d, ждали 2", count())
		}
		time.Sleep(10*time.Minute + 15*time.Second) // без изменений — не реже раза в 10 минут
		synctest.Wait()
		close(stop)
		<-done
		mu.Lock()
		defer mu.Unlock()
		if len(sent) != 3 || sent[1] != `{"apps":{"cs2":{"available":false,"reason":"target_missing"}}}` || sent[2] != sent[1] {
			t.Fatalf("отправлено %v", sent)
		}
		if at[0] != 0 || at[1] != 2*time.Minute+15*time.Second || at[2]-at[1] < 10*time.Minute || at[2]-at[1] > 10*time.Minute+15*time.Second {
			t.Fatalf("моменты отправки %v", at)
		}
	})
}

func TestAppsReportStopsOnSendError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		done := make(chan struct{})
		go func() {
			defer close(done)
			runAppsReport(15*time.Second, 10*time.Minute, make(chan struct{}),
				func() map[string]compactApp { return map[string]compactApp{"cs2": {Available: true}} },
				func(json.RawMessage) error { calls++; return errors.New("нет связи с бэкендом") })
		}()
		select {
		case <-done:
		case <-time.After(time.Hour):
			t.Fatal("связь упала, а цикл отчёта жив")
		}
		if calls != 1 {
			t.Fatalf("попыток отправки %d, ждали 1", calls)
		}
	})
}
