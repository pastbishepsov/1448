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

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// signToken подписывает JWT заданного типа (access/refresh) и времени жизни.
// role едет в claims — по ней adminMiddleware пускает в /admin/*.
func signToken(userID, role, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"typ":  typ,
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

	db.Model(&user).Update("last_active_at", time.Now())
	writeAuth(c, http.StatusOK, &user)
}
