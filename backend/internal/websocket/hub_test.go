package websocket

import (
	"sync"
	"testing"
	"time"
)

// Тесты хаба (ревью 26.08). Клиенты создаются без conn: register/unregister/Send
// сетевых вызовов не делают, а pump'ы мы не запускаем.

func newTestClient(h *Hub, computerID string) *Client {
	return &Client{
		ComputerID: computerID,
		ClubID:     "club-1",
		send:       make(chan []byte, 32),
		hub:        h,
	}
}

// waitConnected ждёт, пока Run() разгребёт свои каналы.
func waitConnected(t *testing.T, h *Hub, id string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.IsConnected(id) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("IsConnected(%s) != %v после ожидания", id, want)
}

// Переподключение агента: старый клиент отваливается ПОСЛЕ того, как новый уже
// зарегистрировался под тем же computer_id. Живой клиент обязан остаться в хабе.
func TestReconnectDoesNotEvictLiveClient(t *testing.T) {
	h := NewHub()
	go h.Run()

	const id = "pc-1"
	oldClient := newTestClient(h, id)
	h.register <- oldClient
	waitConnected(t, h, id, true)

	// Агент переподключился: новое соединение под тем же id.
	newClient := newTestClient(h, id)
	h.register <- newClient
	waitConnected(t, h, id, true)

	// Только теперь дохлый TCP старого соединения закрывается.
	h.unregister <- oldClient

	// Дать Run() время обработать unregister.
	time.Sleep(100 * time.Millisecond)

	if !h.IsConnected(id) {
		t.Fatal("живой клиент выброшен из хаба отвалившимся старым соединением: " +
			"ПК считается офлайн, session_start/session_end до него не доходят")
	}

	// И сообщения должны идти именно новому клиенту.
	if err := h.Send(id, MsgSessionStart, map[string]any{"x": 1}); err != nil {
		t.Fatalf("Send живому клиенту вернул ошибку: %v", err)
	}
	select {
	case <-newClient.send:
	default:
		t.Fatal("сообщение не попало в канал живого клиента")
	}
}

// Вытесненный клиент должен получить закрытый канал, иначе его writePump
// висит вечно (утечка горутины на каждый реконнект агента).
func TestEvictedClientChannelIsClosed(t *testing.T) {
	h := NewHub()
	go h.Run()

	const id = "pc-2"
	oldClient := newTestClient(h, id)
	h.register <- oldClient
	waitConnected(t, h, id, true)

	newClient := newTestClient(h, id)
	h.register <- newClient
	waitConnected(t, h, id, true)

	select {
	case _, ok := <-oldClient.send:
		if ok {
			t.Fatal("в канале вытесненного клиента данные, а ожидалось закрытие")
		}
	case <-time.After(time.Second):
		t.Fatal("канал вытесненного клиента не закрыт — writePump течёт на каждом реконнекте")
	}
}

// Гонка Send/close: биллинг шлёт wallet_update из фоновой горутины, пока агент
// отваливается. Паника «send on closed channel» здесь роняет весь процесс.
func TestSendDuringChurnDoesNotPanic(t *testing.T) {
	h := NewHub()
	go h.Run()

	const id = "pc-3"
	var wg sync.WaitGroup
	stop := make(chan struct{})

	panics := make(chan any, 64)

	// Churn: агент подключается и отваливается по кругу.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			c := newTestClient(h, id)
			h.register <- c
			h.unregister <- c
		}
	}()

	// Биллинг: шлёт в тот же ПК без остановки.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = h.Send(id, MsgWalletUpdate, map[string]any{"grosz": 100})
			}
		}()
	}

	time.Sleep(700 * time.Millisecond)
	close(stop)
	wg.Wait()

	select {
	case p := <-panics:
		t.Fatalf("паника при отправке во время переподключения агента: %v "+
			"(в билинг-горутине это падение всего сервера)", p)
	default:
	}
}

// AdminBroadcast не должен блокироваться навсегда на мёртвой админке:
// он вызывается из Run(), и залипание останавливает весь хаб.
func TestAdminBroadcastUsesWriteDeadline(t *testing.T) {
	h := NewHub()
	done := make(chan struct{})
	go func() {
		h.AdminBroadcast("ping", map[string]any{"a": 1})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AdminBroadcast завис")
	}
}
