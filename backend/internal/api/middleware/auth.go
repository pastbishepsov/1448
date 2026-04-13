package middleware

import (
	"net/http"
	"strings"

	"github.com/1448-project/backend/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Auth — JWT middleware для защищённых эндпоинтов
func Auth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "missing_token",
				"message": "Требуется авторизация",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "invalid_token_format",
				"message": "Формат: Bearer <token>",
			})
			return
		}

		tokenString := parts[1]
		claims := &jwt.MapClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "invalid_token",
				"message": "Токен недействителен или истёк",
			})
			return
		}

		userID, ok := (*claims)["sub"].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "invalid_claims",
				"message": "Некорректные данные токена",
			})
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// StaffAuth — middleware для Admin Panel (по PIN персонала)
func StaffAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: реализовать аутентификацию персонала
		// Пока пропускаем в dev-режиме
		if cfg.Env == "development" {
			c.Set("staff_id", "dev-staff")
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    "staff_auth_required",
			"message": "Требуется авторизация персонала",
		})
	}
}

// ShellAuth — middleware для WebSocket PC Shell (по MAC-адресу + токену)
func ShellAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: аутентификация PC Shell по MAC + токену
		// В dev-режиме принимаем любые подключения
		computerID := c.Query("computer_id")
		if computerID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    "missing_computer_id",
				"message": "Параметр computer_id обязателен",
			})
			return
		}
		c.Set("computer_id", computerID)
		c.Next()
	}
}

// CORS — Cross-Origin Resource Sharing
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
