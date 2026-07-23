package main

// Таймерная логика heartbeat'а на виртуальном времени (Go 1.25+,
// testing/synctest): часы «пузыря» идут только когда все горутины спят,
// поэтому минуты проходят мгновенно — без flaky-sleep'ов и реального WS.

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestHeartbeatTicksEveryMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			mu    sync.Mutex
			ticks []time.Time
		)
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			runHeartbeat(60*time.Second, stop, func() error {
				mu.Lock()
				ticks = append(ticks, time.Now())
				mu.Unlock()
				return nil
			})
		}()

		start := time.Now()
		time.Sleep(5*time.Minute + time.Second)
		synctest.Wait()
		close(stop)
		<-done

		if len(ticks) != 5 {
			t.Fatalf("за 5 минут должно быть 5 heartbeat'ов, получено %d", len(ticks))
		}
		for i, at := range ticks {
			want := start.Add(time.Duration(i+1) * 60 * time.Second)
			if !at.Equal(want) {
				t.Errorf("тик %d в %v, ожидался ровно в %v", i+1, at, want)
			}
		}
	})
}

func TestHeartbeatStopsOnSendError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			runHeartbeat(60*time.Second, stop, func() error {
				calls++
				if calls == 3 {
					return errors.New("связь упала")
				}
				return nil
			})
		}()

		// stop не закрываем: после ошибки отправки цикл обязан встать сам
		// (соединение мёртвое, новый heartbeat поднимет runWS при реконнекте).
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		<-done

		if calls != 3 {
			t.Fatalf("после ошибки отправки цикл должен встать: 3 вызова, получено %d", calls)
		}
	})
}

func TestHeartbeatStopBeforeFirstTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			runHeartbeat(60*time.Second, stop, func() error { calls++; return nil })
		}()

		time.Sleep(30 * time.Second) // до первого тика
		close(stop)
		<-done
		time.Sleep(5 * time.Minute) // после остановки тиков быть не может

		if calls != 0 {
			t.Fatalf("stop до первого тика: heartbeat'ов быть не должно, получено %d", calls)
		}
	})
}
