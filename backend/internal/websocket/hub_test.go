package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

// З0-и1 (№3): увольнение рвёт live-канал именно этого человека — кадром close с
// кодом CloseAccessRevoked; каналы остальных живут и получают события.
func TestDropAdminClosesOnlyThatUser(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h.ServeAdmin(w, r, r.URL.Query().Get("u"))
	}))
	defer srv.Close()
	dial := func(u string) *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/?u="+u, nil)
		if err != nil {
			t.Fatalf("канал %s: %v", u, err)
		}
		return c
	}
	fired, other := dial("fired"), dial("other")
	defer other.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.adminsMu.Lock()
		n := len(h.admins)
		h.adminsMu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("хаб видит %d админок, ждали 2", n)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if n := h.DropAdmin("fired"); n != 1 {
		t.Fatalf("DropAdmin закрыл %d каналов, ждали 1", n)
	}
	_ = fired.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := fired.ReadMessage()
	if !websocket.IsCloseError(err, CloseAccessRevoked) {
		t.Fatalf("уволенный получил %v, ждали close %d", err, CloseAccessRevoked)
	}

	h.AdminBroadcast("ping", map[string]any{"a": 1})
	_ = other.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, msg, err := other.ReadMessage(); err != nil || !strings.Contains(string(msg), `"ping"`) {
		t.Fatalf("чужой канал задет: %v %s", err, msg)
	}
	if n := h.DropAdmin("nobody"); n != 0 {
		t.Fatalf("DropAdmin без каналов вернул %d", n)
	}
}

// З1-и2 (№59): событие строки «только владельцу» уходит только в каналы из
// списка; пустой список — никому, nil — всем.
func TestAdminBroadcastToOnlyListed(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h.ServeAdmin(w, r, r.URL.Query().Get("u"))
	}))
	defer srv.Close()
	dial := func(u string) *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/?u="+u, nil)
		if err != nil {
			t.Fatalf("канал %s: %v", u, err)
		}
		return c
	}
	owner, admin := dial("owner"), dial("admin")
	defer owner.Close()
	defer admin.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.adminsMu.Lock()
		n := len(h.admins)
		h.adminsMu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("хаб видит %d админок, ждали 2", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.AdminBroadcastTo("secret", map[string]any{"a": 1}, map[string]bool{"owner": true})
	h.AdminBroadcastTo("nobody", map[string]any{"a": 2}, map[string]bool{})
	h.AdminBroadcast("public", map[string]any{"a": 3})
	read := func(c *websocket.Conn) string {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("чтение: %v", err)
		}
		return string(msg)
	}
	if got := read(owner); !strings.Contains(got, `"secret"`) {
		t.Fatalf("владелец первым получил %s, ждали secret", got)
	}
	if got := read(owner); !strings.Contains(got, `"public"`) {
		t.Fatalf("владелец вторым получил %s, ждали public", got)
	}
	if got := read(admin); !strings.Contains(got, `"public"`) {
		t.Fatalf("админ получил %s — событие не из его списка", got)
	}
}

// З20-и1 (№57): apps_status агента доходит до OnAppsStatus как есть; незнакомый тип и битый JSON цикл чтения
// пропускает — следующее сообщение того же сокета обрабатывается, агент остаётся на связи (порядок выкатки: сервер,
// потом агенты; старый сервер с новым агентом только пишет строку в лог).
func TestAppsStatusReachesCallbackUnknownSkipped(t *testing.T) {
	h := NewHub()
	go h.Run()
	got := make(chan string, 4)
	h.OnAppsStatus = func(computerID string, payload json.RawMessage) { got <- computerID + " " + string(payload) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h.ServeShell(w, r, "pc-z20", "club-1")
	}))
	defer srv.Close()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/", nil)
	if err != nil {
		t.Fatalf("сокет агента: %v", err)
	}
	defer c.Close()
	for _, m := range []string{`{"type": "z20_unknown", "payload": {"x": 1}}`, `не JSON`,
		`{"type": "apps_status", "payload": {"apps": {"cs2": {"available": true}}}}`} {
		if err := c.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
			t.Fatalf("запись %s: %v", m, err)
		}
	}
	select {
	case s := <-got:
		if s != `pc-z20 {"apps": {"cs2": {"available": true}}}` {
			t.Fatalf("колбэк получил %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("apps_status после незнакомого типа и битого JSON не дошёл до колбэка")
	}
	if !h.IsConnected("pc-z20") {
		t.Fatal("после незнакомого типа агент отключён")
	}
}
