package main

// Вход по QR с телефона (KIOSK.md, К6-и1; решение Р6: автологина при посадке
// нет, вход по QR — да).
//
// Как это работает:
//   - киоск (экран входа) просит код: POST /auth/qr {computer_id} — публично,
//     не чаще qrCreatePerMin с одного IP; код — qrCodeLen символов из
//     crypto/rand по алфавиту без похожих знаков, живёт qrTTL, привязан к ПК;
//   - киоск рисует QR с адресом {origin}/app#qr=CODE и опрашивает
//     GET /auth/qr/:code раз в 2 с: pending → {status:"pending"}, после
//     подтверждения → {status:"approved"} + пара токенов РОВНО ОДИН РАЗ
//     (следующий запрос — 404: код сгорел);
//   - телефон (PWA) открывает /app#qr=CODE, показывает «Войти на PC-01 в
//     14:48 Demo Club?» по GET /auth/qr/:code/info и подтверждает
//     POST /me/auth/qr/approve {code} со своим JWT — код связывается с гостем;
//   - гость с временным паролем не подтверждает (409 password_change_required,
//     как на старте сессии), бан — 409 banned.
//
// Хранилище — память процесса: код живёт две минуты, бэкенд — один инстанс
// (как лимитер и loginguard); рестарт сервера лишь заставит киоск взять
// новый код. now везде параметр — тесты без ожиданий.

import (
	"crypto/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	qrCodeLen      = 8
	qrCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 символа (без смещения при %32), без 0/O и 1/I
	qrTTL          = 120 * time.Second
	qrCreatePerMin = 6 // POST /auth/qr с одного IP в минуту
)

// qrLogin — один код входа.
type qrLogin struct {
	Code       string
	ComputerID uuid.UUID
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UserID     *uuid.UUID // кто подтвердил (nil — ждём)
	ApprovedAt *time.Time
	Consumed   bool // токены уже выданы киоску
}

func (q *qrLogin) pending(now time.Time) bool { return q.UserID == nil && now.Before(q.ExpiresAt) }
func (q *qrLogin) approved(now time.Time) bool {
	return q.UserID != nil && !q.Consumed && now.Before(q.ExpiresAt)
}
func (q *qrLogin) alive(now time.Time) bool { return !q.Consumed && now.Before(q.ExpiresAt) }

// qrStore — коды в памяти процесса + окно запросов на создание по IP.
type qrStore struct {
	mu     sync.Mutex
	codes  map[string]*qrLogin
	byIP   map[string][]time.Time
	random func(n int) (string, error) // подменяется в тестах
}

func newQRStore() *qrStore {
	return &qrStore{codes: map[string]*qrLogin{}, byIP: map[string][]time.Time{}, random: qrRandomCode}
}

var qrLogins = newQRStore()

// qrRandomCode — код из crypto/rand по алфавиту без похожих символов.
func qrRandomCode(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = qrCodeAlphabet[int(b)%len(qrCodeAlphabet)]
	}
	return string(out), nil
}

// sweep — выбросить сгоревшие коды и старые отметки IP (под мьютексом).
func (s *qrStore) sweep(now time.Time) {
	for k, q := range s.codes {
		if q.Consumed || !now.Before(q.ExpiresAt) {
			delete(s.codes, k)
		}
	}
	for ip, ts := range s.byIP {
		keep := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < time.Minute {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			delete(s.byIP, ip)
		} else {
			s.byIP[ip] = keep
		}
	}
}

// allowCreate — не чаще qrCreatePerMin кодов в минуту с одного IP.
func (s *qrStore) allowCreate(ip string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	if len(s.byIP[ip]) >= qrCreatePerMin {
		return false
	}
	s.byIP[ip] = append(s.byIP[ip], now)
	return true
}

// create — новый код для ПК.
func (s *qrStore) create(computerID uuid.UUID, now time.Time) (*qrLogin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < 5; i++ { // коллизия при 32^8 — почти невозможна, но проверяем
		code, err := s.random(qrCodeLen)
		if err != nil {
			return nil, err
		}
		if _, dup := s.codes[code]; dup {
			continue
		}
		q := &qrLogin{Code: code, ComputerID: computerID, CreatedAt: now, ExpiresAt: now.Add(qrTTL)}
		s.codes[code] = q
		return q, nil
	}
	return nil, errQRCollision
}

// get — копия живого кода (не сгоревшего и не использованного); nil — нет.
func (s *qrStore) get(code string, now time.Time) *qrLogin {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.codes[code]
	if !ok || !q.alive(now) {
		return nil
	}
	cp := *q
	return &cp
}

// approve — связать ожидающий код с гостем; false — кода нет или он не ждёт.
func (s *qrStore) approve(code string, userID uuid.UUID, now time.Time) (*qrLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.codes[code]
	if !ok || !q.pending(now) {
		return nil, false
	}
	uid := userID
	t := now
	q.UserID, q.ApprovedAt = &uid, &t
	cp := *q
	return &cp, true
}

// consume — забрать подтверждённый код ровно один раз; false — не готов/нет.
func (s *qrStore) consume(code string, now time.Time) (*qrLogin, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.codes[code]
	if !ok || !q.approved(now) {
		return nil, false
	}
	q.Consumed = true
	delete(s.codes, code)
	cp := *q
	return &cp, true
}

type qrError string

func (e qrError) Error() string { return string(e) }

const errQRCollision qrError = "qr_collision"

func normalizeQRCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// qrPlace — ПК и клуб для подтверждения на телефоне.
func qrPlace(computerID uuid.UUID) (computer models.Computer, club models.Club, ok bool) {
	if err := db.First(&computer, "id = ?", computerID).Error; err != nil {
		return computer, club, false
	}
	_ = db.First(&club, "id = ?", computer.ClubID).Error
	return computer, club, true
}

func qrPlaceJSON(q *qrLogin, now time.Time) gin.H {
	out := gin.H{"expires_in": int(q.ExpiresAt.Sub(now).Seconds())}
	if pc, club, ok := qrPlace(q.ComputerID); ok {
		out["computer"] = gin.H{"id": pc.ID, "name": pc.Name}
		out["club"] = gin.H{"id": club.ID, "name": club.Name}
	}
	return out
}

func respondQRNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"code": "qr_code_not_found", "message": "Код устарел или уже использован — обнови QR на экране"})
}

type qrCreateRequest struct {
	ComputerID string `json:"computer_id" binding:"required"`
}

// POST /auth/qr {computer_id} — киоск просит код для QR (публично, 6/мин с IP).
func handleQRCreate(c *gin.Context) {
	now := time.Now()
	if !qrLogins.allowCreate(c.ClientIP(), now) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": "qr_rate_limited", "message": "Слишком много кодов — подожди минуту"})
		return
	}
	var req qrCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	pcID, err := uuid.Parse(req.ComputerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "computer_id_invalid", "message": "Некорректный идентификатор компьютера"})
		return
	}
	var n int64
	db.Model(&models.Computer{}).Where("id = ?", pcID).Count(&n)
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "Компьютер не найден"})
		return
	}
	q, err := qrLogins.create(pcID, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "qr_error", "message": "Не удалось создать код — попробуй ещё раз"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": q.Code, "expires_in": int(qrTTL.Seconds())})
}

// GET /auth/qr/:code — поллинг киоска: pending | approved + токены один раз.
func handleQRPoll(c *gin.Context) {
	now := time.Now()
	code := normalizeQRCode(c.Param("code"))
	q := qrLogins.get(code, now)
	if q == nil {
		respondQRNotFound(c)
		return
	}
	if q.UserID == nil {
		c.JSON(http.StatusOK, gin.H{"status": "pending", "expires_in": int(q.ExpiresAt.Sub(now).Seconds())})
		return
	}
	q, ok := qrLogins.consume(code, now)
	if !ok { // гонка двух опросов — токены получил первый
		respondQRNotFound(c)
		return
	}
	var user models.User
	if err := db.First(&user, "id = ?", *q.UserID).Error; err != nil {
		respondQRNotFound(c)
		return
	}
	if user.Status == models.UserStatusBanned { // забанили между approve и опросом
		c.JSON(http.StatusForbidden, gin.H{"code": "banned", "message": "Аккаунт заблокирован"})
		return
	}
	auth, err := buildAuth(&user, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "token_error", "message": "Не удалось создать токен"})
		return
	}
	db.Model(&user).Update("last_active_at", now)
	c.JSON(http.StatusOK, gin.H{
		"status": "approved", "access_token": auth.AccessToken, "refresh_token": auth.RefreshToken,
		"token_type": auth.TokenType, "expires_in": auth.ExpiresIn, "user": auth.User,
	})
}

// GET /auth/qr/:code/info — что подтверждаем: ПК и клуб (для телефона).
func handleQRInfo(c *gin.Context) {
	now := time.Now()
	q := qrLogins.get(normalizeQRCode(c.Param("code")), now)
	if q == nil {
		respondQRNotFound(c)
		return
	}
	out := qrPlaceJSON(q, now)
	out["status"] = "pending"
	if q.UserID != nil {
		out["status"] = "approved"
	}
	c.JSON(http.StatusOK, out)
}

type qrApproveRequest struct {
	Code string `json:"code" binding:"required"`
}

// POST /me/auth/qr/approve {code} — гость с телефона подтверждает вход на ПК.
func handleQRApprove(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var req qrApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	now := time.Now()
	code := normalizeQRCode(req.Code)
	if qrLogins.get(code, now) == nil {
		respondQRNotFound(c)
		return
	}
	// Е0-и2: с временным паролем гость на ПК не входит — его знает и админ,
	// который пароль выдал (та же проверка, что на старте сессии).
	var u models.User
	if db.Select("must_change_password").First(&u, "id = ?", userID).Error == nil && u.MustChangePassword {
		c.JSON(http.StatusConflict, gin.H{"code": "password_change_required",
			"message": "Сначала задай свой пароль вместо временного"})
		return
	}
	if rejectBanned(c, userID) { // К0-и4: бан действует сразу
		return
	}
	q, ok := qrLogins.approve(code, userID, now)
	if !ok { // сгорел или уже подтверждён с другого телефона
		respondQRNotFound(c)
		return
	}
	checkBirthdayGift(userID) // Г8-и4: вход — повод для подарка в день рождения
	out := qrPlaceJSON(q, now)
	out["status"] = "approved"
	c.JSON(http.StatusOK, out)
}
