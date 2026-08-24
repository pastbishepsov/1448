package main

// Refresh-токены: stateless, тот же HS256-секрет, но claim typ=refresh
// и долгий TTL. Ротация: /auth/refresh выдаёт новую пару access+refresh.
// Отзыв конкретного токена без хранилища невозможен — logout с blacklist
// в Redis остаётся будущей задачей (см. STATUS.md).

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// signToken подписывает JWT заданного типа (access/refresh) и времени жизни.
// role едет в claims — по ней adminMiddleware пускает в /admin/*.
func signToken(userID, role, typ string, ttl time.Duration) (string, error) {
	return signTokenAt(userID, role, typ, ttl, time.Now())
}

// signTokenAt — то же, но с явным моментом выдачи. Нужен смене пароля (Е0-и2):
// отсечка tokens_valid_from округляется ВВЕРХ до секунды, и свежая пара
// подписывается ровно этим моментом — она переживает отсечку по построению,
// а не потому, что успела в ту же секунду.
func signTokenAt(userID, role, typ string, ttl time.Duration, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"typ":  typ,
		"jti":  uuid.NewString(), // для отзыва (logout / ротация refresh)
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// parseToken проверяет подпись HS256 и срок действия, возвращает claims.
func parseToken(raw string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("неожиданный метод подписи")
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("токен недействителен")
	}
	return claims, nil
}

// tokenType достаёт claim typ ("" для старых токенов без типа).
func tokenType(claims jwt.MapClaims) string {
	typ, _ := claims["typ"].(string)
	return typ
}

// claimString — строковый claim ("" если отсутствует).
func claimString(claims jwt.MapClaims, key string) string {
	v, _ := claims[key].(string)
	return v
}

// claimIssuedAt — момент выдачи из iat (0, если прочитать не вышло).
// Нужен отсечке tokens_valid_from при сбросе пароля (Е0-и2).
func claimIssuedAt(claims jwt.MapClaims) int64 {
	if v, ok := claims["iat"].(float64); ok {
		return int64(v)
	}
	return 0
}

// claimExpiry — срок токена из exp.
func claimExpiry(claims jwt.MapClaims) time.Time {
	if v, ok := claims["exp"].(float64); ok {
		return time.Unix(int64(v), 0)
	}
	return time.Now() // не смогли прочитать — считаем «живой сейчас», запись подчистится
}

// ── Отзыв refresh-токенов (таблица revoked_tokens, миграция 010) ──
// Access-токены не отзываем: они живут ≤15 минут — стандартный компромисс.

func isRevoked(jti string) bool {
	var n int64
	db.Model(&models.RevokedToken{}).Where("jti = ?", jti).Count(&n)
	return n > 0
}

func revokeToken(jti, userID string, expiresAt time.Time) error {
	jid, err := uuid.Parse(jti)
	if err != nil {
		return errors.New("некорректный jti")
	}
	uid, _ := uuid.Parse(userID)
	// Попутная уборка просроченных записей — дешёвая и держит таблицу маленькой.
	db.Where("expires_at < ?", time.Now()).Delete(&models.RevokedToken{})
	err = db.Create(&models.RevokedToken{JTI: jid, UserID: uid, ExpiresAt: expiresAt}).Error
	if err != nil && isDuplicate(err) {
		return nil // уже отозван — не ошибка
	}
	return err
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// handleRefresh — POST /auth/refresh: обмен refresh-токена на новую пару токенов.
func handleRefresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	claims, err := parseToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_token", "message": "Refresh-токен недействителен или истёк"})
		return
	}
	if tokenType(claims) != tokenTypeRefresh {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "wrong_token_type", "message": "Ожидается refresh-токен"})
		return
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_claims", "message": "Некорректные данные токена"})
		return
	}
	jti := claimString(claims, "jti")
	if jti == "" || isRevoked(jti) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "token_revoked", "message": "Refresh-токен отозван — войди заново"})
		return
	}

	// Пользователь должен существовать и не быть в бане — бан отсекает
	// обновление токенов, даже если refresh ещё формально жив.
	var user models.User
	if err := db.First(&user, "id = ?", sub).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if user.Status == models.UserStatusBanned {
		c.JSON(http.StatusForbidden, gin.H{"code": "banned", "message": "Аккаунт заблокирован"})
		return
	}
	// Е0-и2: смена пароля рвёт ВСЕ живые refresh'ы гостя, включая те, что мы
	// никогда не видели, — отзыв по jti (миграция 010) их не достаёт.
	// Проверка стоит здесь, потому что пользователь уже прочитан из базы;
	// в authMiddleware её нет сознательно — он не ходит в БД, а access живёт
	// ≤15 минут (принятый компромисс проекта, STATUS.md).
	if tokenIssuedBefore(claimIssuedAt(claims), user.TokensValidFrom) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "token_revoked",
			"message": "Пароль менялся — войди заново"})
		return
	}

	// Ротация: старый refresh отзываем — повторно им воспользоваться нельзя.
	_ = revokeToken(jti, sub, claimExpiry(claims))

	db.Model(&user).Update("last_active_at", time.Now())
	writeAuth(c, http.StatusOK, &user)
}

// handleLogout — POST /auth/logout: отозвать refresh-токен.
// Идемпотентен: невалидный/уже отозванный токен — это тоже успешный выход.
func handleLogout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "нужен refresh_token"})
		return
	}

	claims, err := parseToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true}) // уже мёртв — выход состоялся
		return
	}
	if tokenType(claims) != tokenTypeRefresh {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "wrong_token_type", "message": "Ожидается refresh-токен"})
		return
	}
	if jti := claimString(claims, "jti"); jti != "" {
		if err := revokeToken(jti, claimString(claims, "sub"), claimExpiry(claims)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
