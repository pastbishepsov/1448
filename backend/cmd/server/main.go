package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

var (
	db        *gorm.DB
	jwtSecret []byte
	accessTTL time.Duration
	hub       *websocket.Hub
)

// @title           14:48 API
// @version         0.1.0
// @description     Геймифицированная экосистема управления компьютерными клубами
// @host            localhost:8080
// @BasePath        /api/v1
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env не найден — использую переменные окружения")
	}

	jwtSecret = []byte(getenv("JWT_SECRET", "dev_insecure_secret_change_me"))
	accessTTL = parseDuration(os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute)

	if os.Getenv("SERVER_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// ── Подключение к PostgreSQL ────────────────────────────────────────────
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = buildDSN()
	}
	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	log.Println("Подключение к PostgreSQL установлено")

	// ── WebSocket Hub (PC Shell) ────────────────────────────────────────────
	hub = websocket.NewHub()
	go hub.Run()

	// ── Роуты ───────────────────────────────────────────────────────────────
	r := gin.Default()
	r.Use(corsMiddleware()) // разрешаем запросы из браузера (веб-приложение)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": "0.1.0",
			"service": "14:48 Backend",
		})
	})

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		auth.POST("/register", handleRegister) // ← настоящий
		auth.POST("/login", handleLogin)        // ← настоящий
		auth.POST("/otp/send", stub("SendOTP"))
		auth.POST("/otp/verify", stub("VerifyOTP"))
		auth.POST("/refresh", stub("RefreshToken"))
		auth.POST("/logout", stub("Logout"))

		me := v1.Group("/me")
		me.Use(authMiddleware()) // всё под /me требует JWT
		me.GET("", handleGetMe)   // ← настоящий
		me.PATCH("", stub("UpdateProfile"))
		me.GET("/cases", handleGetMyCases)         // ← настоящий
		me.POST("/cases/:id/open", handleOpenCase) // ← настоящий
		me.GET("/talents", stub("GetTalents"))
		me.POST("/talents/invest", stub("InvestSP"))
		me.GET("/achievements", stub("GetAchievements"))
		me.GET("/sessions", handleGetMySessions)     // ← настоящий
		me.POST("/sessions/start", handleStartSession) // ← настоящий
		me.POST("/sessions/end", handleEndSession)     // ← настоящий

		clubs := v1.Group("/clubs")
		clubs.GET("", stub("GetClubs"))
		clubs.GET("/:id", stub("GetClub"))
		clubs.GET("/:id/computers", stub("GetComputers"))
		clubs.POST("/:id/bookings", stub("CreateBooking"))

		v1.GET("/ws/shell", handleShellWS) // ← настоящий
	}

	port := getenv("SERVER_PORT", "8080")
	log.Printf("14:48 Backend запущен → http://localhost:%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Ошибка запуска: %v", err)
	}
}

// ───────────────────────────── AUTH ─────────────────────────────

type registerRequest struct {
	Nickname string  `json:"nickname" binding:"required,min=3,max=32"`
	Password string  `json:"password" binding:"required,min=6,max=72"`
	Email    *string `json:"email"    binding:"omitempty,email"`
	Phone    *string `json:"phone"    binding:"omitempty"`
}

type loginRequest struct {
	Login    string `json:"login"    binding:"required"` // nickname или email
	Password string `json:"password" binding:"required"`
}

type authResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int64        `json:"expires_in"`
	User        *models.User `json:"user"`
}

func handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "hash_error", "message": "Не удалось обработать пароль"})
		return
	}

	now := time.Now()
	user := models.User{
		Nickname:     req.Nickname,
		Email:        req.Email,
		Phone:        req.Phone,
		PasswordHash: string(hash),
		Status:       models.UserStatusActive,
		Level:        1,
		RegisteredAt: now,
		LastActiveAt: now,
	}

	if err := db.Create(&user).Error; err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_exists", "message": "Никнейм, email или телефон уже заняты"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Приветственный кейс новичку.
	_ = grantCase(db, user.ID, nil, models.CaseTierLight, models.CaseSourceAdminGrant)

	writeAuth(c, http.StatusCreated, &user)
}

func handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	var user models.User
	if err := db.Where("nickname = ? OR email = ?", req.Login, req.Login).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "message": "Неверный логин или пароль"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "message": "Неверный логин или пароль"})
		return
	}

	if user.Status == models.UserStatusBanned {
		c.JSON(http.StatusForbidden, gin.H{"code": "banned", "message": "Аккаунт заблокирован"})
		return
	}

	db.Model(&user).Update("last_active_at", time.Now())
	writeAuth(c, http.StatusOK, &user)
}

func writeAuth(c *gin.Context, status int, user *models.User) {
	token, err := signToken(user.ID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "token_error", "message": "Не удалось создать токен"})
		return
	}
	c.JSON(status, authResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(accessTTL.Seconds()),
		User:        user,
	})
}

func signToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(accessTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// authMiddleware — проверяет JWT из заголовка Authorization: Bearer <token>
// и кладёт user_id в контекст.
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "missing_token", "message": "Требуется авторизация"})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "invalid_token_format", "message": "Формат: Bearer <token>"})
			return
		}

		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("неожиданный метод подписи")
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "invalid_token", "message": "Токен недействителен или истёк"})
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "invalid_claims", "message": "Некорректные данные токена"})
			return
		}

		c.Set("user_id", sub)
		c.Next()
	}
}

// handleShellWS — апгрейд соединения PC Shell до WebSocket.
// dev: токен ПК пока не проверяем (TODO: аутентификация по MAC + токену).
func handleShellWS(c *gin.Context) {
	computerID := c.Query("computer_id")
	if computerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "missing_computer_id", "message": "Параметр computer_id обязателен"})
		return
	}

	clubID := ""
	var computer models.Computer
	if err := db.First(&computer, "id = ?", computerID).Error; err == nil {
		clubID = computer.ClubID.String()
	}

	if err := hub.ServeShell(c.Writer, c.Request, computerID, clubID); err != nil {
		log.Printf("WS upgrade error: %v", err)
	}
}

// notifyShell — best-effort команда на ПК (если он подключён).
func notifyShell(computerID string, t websocket.MessageType, payload any) {
	if hub == nil {
		return
	}
	_ = hub.Send(computerID, t, payload)
}

// handleGetMe — профиль текущего игрока по токену.
func handleGetMe(c *gin.Context) {
	userID := c.GetString("user_id")

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "Пользователь не найден"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ─────────────────────────── helpers ───────────────────────────

func stub(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"status":  "not_implemented",
			"handler": name,
			"message": "TODO: реализовать " + name,
		})
	}
}

// corsMiddleware — разрешает запросы из браузера (dev: любой источник).
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique") ||
		strings.Contains(msg, "23505")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func buildDSN() string {
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	user := getenv("POSTGRES_USER", "1448_user")
	pass := getenv("POSTGRES_PASSWORD", "change_me_in_production")
	name := getenv("POSTGRES_DB", "1448_db")
	return "host=" + host + " port=" + port + " user=" + user +
		" password=" + pass + " dbname=" + name +
		" sslmode=disable TimeZone=Europe/Warsaw"
}
