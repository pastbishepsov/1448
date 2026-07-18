package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageType — типы событий между сервером и PC Shell.
type MessageType string

const (
	MsgSessionStart MessageType = "session_start" // сервер → shell: началась сессия
	MsgSessionTick  MessageType = "session_tick"  // shell → сервер: heartbeat
	MsgSessionEnd   MessageType = "session_end"   // сервер → shell: завершить
	MsgXPUpdate     MessageType = "xp_update"      // сервер → shell: новый XP (для оверлея C#-киоска)
	MsgAdminCall    MessageType = "admin_call"     // shell → сервер: кнопка вызова
	MsgPCPower      MessageType = "pc_power"       // сервер → shell: restart|shutdown (Б8)
	MsgWOL          MessageType = "wol"            // сервер → shell: разбудить соседа по MAC (Б8)
	// force_unlock удалён (Б7-и3): сервер его никогда не слал — мёртвый тип.
)

// Message — стандартная обёртка для всех WS-сообщений.
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// upgrader — HTTP→WebSocket. В dev разрешаем любые источники.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Client — одно подключение PC Shell.
type Client struct {
	ComputerID string
	ClubID     string
	conn       *websocket.Conn
	send       chan []byte
	hub        *Hub
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("WS write error (computer=%s): %v", c.ComputerID, err)
			return
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("WS parse error: %v", err)
			continue
		}
		c.hub.handleMessage(c, msg)
	}
}

// Hub — менеджер всех подключённых PC Shell + live-канал админки (А6).
type Hub struct {
	clients    map[string]*Client // computerID → client
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex

	admins   map[*websocket.Conn]bool // подключённые админки (спринт А6)
	adminsMu sync.Mutex

	// OnAdminCall — колбэк на admin_call от агента (спринт Б2): пакет
	// websocket не знает о БД, main.go подставляет обработчик, который
	// пишет вызов в чат и рассылает админкам.
	OnAdminCall func(computerID string)
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		admins:     make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ComputerID] = client
			h.mu.Unlock()
			log.Printf("WS: PC Shell подключён (computer=%s, club=%s)", client.ComputerID, client.ClubID)
			h.AdminBroadcast("shell_online", map[string]any{"computer_id": client.ComputerID})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ComputerID]; ok {
				delete(h.clients, client.ComputerID)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WS: PC Shell отключён (computer=%s)", client.ComputerID)
			h.AdminBroadcast("shell_offline", map[string]any{"computer_id": client.ComputerID})
		}
	}
}

// ── Админ-канал (спринт А6, ADMIN.md): live-события для админки ──

// AdminEvent — обёртка события для админ-клиентов.
type AdminEvent struct {
	Type string         `json:"type"`
	At   time.Time      `json:"at"`
	Data map[string]any `json:"data,omitempty"`
}

// ServeAdmin — апгрейд соединения админки; read-loop нужен только для
// обнаружения разрыва (админка ничего не шлёт).
func (h *Hub) ServeAdmin(w http.ResponseWriter, r *http.Request) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	h.adminsMu.Lock()
	h.admins[conn] = true
	n := len(h.admins)
	h.adminsMu.Unlock()
	log.Printf("WS: админка подключена (всего: %d)", n)
	go func() {
		defer func() {
			h.adminsMu.Lock()
			delete(h.admins, conn)
			h.adminsMu.Unlock()
			conn.Close()
			log.Printf("WS: админка отключена")
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return nil
}

// AdminBroadcast — рассылка события всем подключённым админкам (best-effort:
// мёртвые соединения выбрасываются, ошибка никого не роняет).
func (h *Hub) AdminBroadcast(evType string, data map[string]any) {
	msg, err := json.Marshal(AdminEvent{Type: evType, At: time.Now(), Data: data})
	if err != nil {
		return
	}
	h.adminsMu.Lock()
	defer h.adminsMu.Unlock()
	for conn := range h.admins {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			delete(h.admins, conn)
			conn.Close()
		}
	}
}

// ServeShell — апгрейд соединения до WebSocket и регистрация PC Shell в хабе.
func (h *Hub) ServeShell(w http.ResponseWriter, r *http.Request, computerID, clubID string) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	client := &Client{
		ComputerID: computerID,
		ClubID:     clubID,
		conn:       conn,
		send:       make(chan []byte, 32),
		hub:        h,
	}
	h.register <- client
	go client.writePump()
	go client.readPump()
	return nil
}

// IsConnected — подключён ли указанный компьютер.
func (h *Hub) IsConnected(computerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[computerID]
	return ok
}

// AnyClientInClub — id любого живого агента клуба (Б8: WoL-прокси —
// magic-пакет выключенному ПК шлёт сосед по LAN).
func (h *Hub) AnyClientInClub(clubID string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for id, c := range h.clients {
		if c.ClubID == clubID {
			return id, true
		}
	}
	return "", false
}

// Send — отправить сообщение конкретному компьютеру.
// Возвращает ошибку, если ПК не подключён (для server→shell это «best-effort»).
func (h *Hub) Send(computerID string, msgType MessageType, payload any) error {
	h.mu.RLock()
	client, ok := h.clients[computerID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("компьютер %s не подключён", computerID)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msg, _ := json.Marshal(Message{Type: msgType, Payload: payloadBytes})

	select {
	case client.send <- msg:
		return nil
	default:
		return fmt.Errorf("очередь отправки переполнена (computer=%s)", computerID)
	}
}

// handleMessage — обработка входящих сообщений от Shell.
func (h *Hub) handleMessage(c *Client, msg Message) {
	switch msg.Type {
	case MsgSessionTick:
		log.Printf("Heartbeat от computer=%s", c.ComputerID)
	case MsgAdminCall:
		log.Printf("Вызов администратора от computer=%s", c.ComputerID)
		if h.OnAdminCall != nil {
			h.OnAdminCall(c.ComputerID) // Б2: вызов идёт в чат и админкам
		}
	default:
		log.Printf("Неизвестный тип сообщения: %s от computer=%s", msg.Type, c.ComputerID)
	}
}
