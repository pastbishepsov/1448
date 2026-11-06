package main

// Rate limiting: ~10 запросов/сек с одного IP (ТЗ 10.1), burst 20.
// In-memory token bucket — для одного инстанса бэкенда достаточно;
// при горизонтальном масштабировании переедет в Redis.

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	rlRatePerSec = 10.0
	rlBurst      = 20.0
	rlSweepEvery = 5 * time.Minute
)

type rlBucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*rlBucket
	now       func() time.Time // подменяется в тестах
	lastSweep time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]*rlBucket{}, now: time.Now, lastSweep: time.Now()}
}

// Allow — пропустить ли запрос с этого ключа (IP).
func (r *rateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()

	// Редкая уборка простаивающих вёдер, чтобы карта не росла вечно.
	if now.Sub(r.lastSweep) > rlSweepEvery {
		for k, b := range r.buckets {
			if now.Sub(b.last) > time.Minute {
				delete(r.buckets, k)
			}
		}
		r.lastSweep = now
	}

	b, ok := r.buckets[key]
	if !ok {
		r.buckets[key] = &rlBucket{tokens: rlBurst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * rlRatePerSec
	if b.tokens > rlBurst {
		b.tokens = rlBurst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimitExempt — пути, которые лимитер не режет: мониторинг и статика.
// Холодный старт PWA — это страница, манифест, service worker и десяток
// иконок ещё ДО первого запроса к API; вместе с залпом загрузчиков экрана они
// пробивали burst, и гость видел «слишком много запросов» на ровном месте, а
// упавший /auth/refresh выбрасывал его на экран входа (ревью 26.08).
func rateLimitExempt(path string) bool {
	switch path {
	case "/health", "/app", "/register", "/admin", "/shell", "/aim.html", "/sw.js", "/app.webmanifest":
		return true
	}
	return strings.HasPrefix(path, "/icons") || strings.HasPrefix(path, "/covers")
}

// rateLimitImage — GET картинок из базы (обложка плитки, фото позиции кухни) лимитер не режет, как статику /covers
// (З18-и3, ревью 23.09 №35): киоск грузит 30+ обложек разом, а при облачном сервере весь клуб за одним NAT-адресом
// делит 10 rps — треть плиток приходила пустой (429). Нагрузку держит кэш: по адресу с версией — сутки, повтор с
// ETag — 304 без чтения байтов (images.go). Фото документа расхода — только владельцу и по одному: под лимитером.
func rateLimitImage(method, route string) bool {
	return method == http.MethodGet && (route == "/api/v1/catalog/:id/cover" || route == "/api/v1/goods/:id/photo")
}

// rateLimitMiddleware — глобальный лимитер (кроме /health и статики).
func rateLimitMiddleware() gin.HandlerFunc {
	rl := newRateLimiter()
	return func(c *gin.Context) {
		route := c.FullPath()
		if rateLimitExempt(route) || rateLimitExempt(c.Request.URL.Path) || rateLimitImage(c.Request.Method, route) {
			c.Next()
			return
		}
		if !rl.Allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": "rate_limited", "message": "Слишком много запросов — подожди секунду"})
			return
		}
		c.Next()
	}
}
