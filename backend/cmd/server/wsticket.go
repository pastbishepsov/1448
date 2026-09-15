package main

// Ж0-и5 (ADMIN.md, трек Ж): одноразовый тикет для /ws/admin.
//
// Браузерный WebSocket не умеет заголовков, поэтому админка передавала
// access-токен query-параметром — и gin-логгер писал его целиком в лог
// сервера (на Railway — в логи платформы): кто читает логи, тот 15 минут
// админ (ревью 14.09, №8). Теперь админка сперва берёт тикет обычным
// запросом с заголовком Authorization, а в URL сокета уходит только он:
// случайный, живёт wsTicketTTL, гасится первым же использованием. Попавший в
// лог тикет бесполезен уже через полминуты, а чаще — сразу.
//
// Хранилище в памяти процесса: тикет живёт секунды, и переживать рестарт
// ему незачем — админка после реконнекта берёт новый.

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const wsTicketTTL = 30 * time.Second

type wsTicket struct {
	UserID string
	Role   string
	Exp    time.Time
}

// wsTicketStore — чистое хранилище без HTTP (тест): выдать, погасить, подмести.
type wsTicketStore struct {
	mu sync.Mutex
	m  map[string]wsTicket
}

func newWSTicketStore() *wsTicketStore { return &wsTicketStore{m: map[string]wsTicket{}} }

// issue — новый тикет; заодно выметает протухшие, чтобы карта не росла.
func (s *wsTicketStore) issue(userID, role string, now time.Time) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	t := base64.RawURLEncoding.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if !now.Before(v.Exp) {
			delete(s.m, k)
		}
	}
	s.m[t] = wsTicket{UserID: userID, Role: role, Exp: now.Add(wsTicketTTL)}
	return t, nil
}

// redeem — погасить: второй раз и после срока не проходит.
func (s *wsTicketStore) redeem(t string, now time.Time) (wsTicket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[t]
	if !ok {
		return wsTicket{}, false
	}
	delete(s.m, t)
	if !now.Before(v.Exp) {
		return wsTicket{}, false
	}
	return v, true
}

var wsTickets = newWSTicketStore()

// POST /admin/ws-ticket — тикет для живого канала (за authMiddleware +
// adminMiddleware: роль уже проверена).
func handleAdminWSTicket(c *gin.Context) {
	t, err := wsTickets.issue(c.GetString("user_id"), c.GetString("user_role"), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "ticket_error", "message": "Не удалось выдать тикет"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": t, "expires_in": int(wsTicketTTL.Seconds())})
}
