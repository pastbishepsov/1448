package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"github.com/gorilla/websocket"
)

// MessageType — типы событий между сервером и PC Shell
type MessageType string

const (
	MsgSessionStart  MessageType = "session_start"   // сервер → shell: начало сессии
	MsgSessionTick   MessageType = "session_tick"    // shell → сервер: heartbeat
	MsgSessionEnd    MessageType = "session_end"     // сервер → shell: завершить и заблокировать
	MsgXPUpdate      MessageType = "xp_update"       // сервер → shell: новый XP
	MsgAdminCall     MessageType = "admin_call"      // shell → сервер: кнопка вызова
	MsgForceUnlock   MessageType = "force_unlock"    // сервер → shell: принудительная разблокировка
)

// Message — стандартная обёртка для всех WS-сообщений
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Client — одно подключение PC Shell
type Client struct {
	ComputerID string
	ClubID     string
	conn       *websocket.Conn
	send       chan []byte
	hub        *Hub
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
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

// Hub — менеджер всех подключённых PC Shell
type Hub struct {
	clients    map[string]*Client // computerID → client
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
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

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ComputerID]; ok {
				delete(h.clients, client.ComputerID)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WS: PC Shell отключён (computer=%s)", client.ComputerID)
		}
	}
}

// Send — отправить сообщение конкретному компьютеру
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
	client.send <- msg
	return nil
}

// handleMessage — обработка входящих сообщений от Shell
func (h *Hub) handleMessage(c *Client, msg Message) {
	switch msg.Type {
	case MsgSessionTick:
		// Обновить last_seen в Redis для данного компьютера
		log.Printf("Heartbeat от computer=%s", c.ComputerID)
	case MsgAdminCall:
		// Уведомить Admin Panel что игрок вызвал администратора
		log.Printf("Вызов администратора от computer=%s", c.ComputerID)
	default:
		log.Printf("Неизвестный тип сообщения: %s от computer=%s", msg.Type, c.ComputerID)
	}
}

// fmt нужен для Errorf — добавь импорт в реальном коде
var fmt = struct{ Errorf func(string, ...any) error }{}
