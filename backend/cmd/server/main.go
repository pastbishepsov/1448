package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

var (
	db         *gorm.DB
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	hub        *websocket.Hub
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
	refreshTTL = parseDuration(os.Getenv("JWT_REFRESH_TTL"), 30*24*time.Hour)

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
	hub.OnAdminCall = agentAdminCall // Б2: admin_call агента → чат + админки
	go hub.Run()

	// ── Роуты ───────────────────────────────────────────────────────────────
	r := gin.Default()
	r.Use(corsMiddleware())        // разрешаем запросы из браузера (веб-приложение)
	r.Use(crossOriginMiddleware()) // CSRF-защита мутаций (Go 1.25+), GET/агент не трогает
	r.Use(rateLimitMiddleware())   // ~10 rps с IP (ТЗ 10.1), кроме /health

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
		auth.POST("/login", handleLogin)       // ← настоящий
		auth.POST("/otp/send", stub("SendOTP"))
		auth.POST("/otp/verify", stub("VerifyOTP"))
		auth.POST("/refresh", handleRefresh) // ← настоящий
		auth.POST("/logout", handleLogout)   // ← настоящий

		me := v1.Group("/me")
		me.Use(authMiddleware())                           // всё под /me требует JWT
		me.GET("", handleGetMe)                            // ← настоящий
		me.PATCH("", handlePatchMe)                        // ← настоящий
		me.GET("/cases", handleGetMyCases)                 // ← настоящий
		me.POST("/cases/:id/open", handleOpenCase)         // ← настоящий
		me.GET("/talents", handleGetMyTalents)             // ← настоящий
		me.POST("/talents/invest", handleInvestTalent)     // ← настоящий
		me.GET("/achievements", handleGetMyAchievements)   // ← настоящий
		me.GET("/sessions", handleGetMySessions)           // ← настоящий
		me.POST("/sessions/start", handleStartSession)     // ← настоящий
		me.POST("/sessions/end", handleEndSession)         // ← настоящий
		me.GET("/bookings", handleGetMyBookings)           // ← настоящий
		me.DELETE("/bookings/:id", handleCancelBooking)    // ← настоящий
		me.GET("/deposits", handleGetMyDeposits)           // ← настоящий
		me.GET("/economy", handleGetEconomy)               // ← настоящий (калькулятор + ранг)
		me.GET("/sensitivity", handleGetSensitivity)       // ← настоящий (профиль сенсы)
		me.PUT("/sensitivity", handlePutSensitivity)       // ← настоящий
		me.POST("/chat", handleGuestChatPost)              // чат/вызов админа (Б2)
		me.GET("/chat", handleGuestChatList)               // переписка, поллинг (Б3)
		me.GET("/chat/unread", handleGuestChatUnread)      // бэйдж непрочитанного (Б3)
		me.GET("/notifications", handleGetMyNotifications) // тосты о действиях админа (Б4)
		me.GET("/waitlist", handleGetMyWaitlist)           // своя позиция в очереди (Б9, задел PWA)
		me.POST("/waitlist", handleJoinWaitlist)           // встать в очередь самому (Б9)
		me.DELETE("/waitlist", handleLeaveWaitlist)        // выйти из очереди (Б9)

		// Лидерборд (за JWT)
		v1.GET("/leaderboard", authMiddleware(), handleLeaderboard)

		clubs := v1.Group("/clubs")
		clubs.GET("", handleGetClubs)                                      // ← настоящий
		clubs.GET("/:id", handleGetClub)                                   // ← настоящий
		clubs.GET("/:id/computers", handleGetClubComputers)                // ← настоящий
		clubs.POST("/:id/bookings", authMiddleware(), handleCreateBooking) // ← настоящий

		v1.GET("/ws/shell", handleShellWS)       // ← настоящий
		v1.GET("/ws/admin", handleAdminWS)       // live-канал админки (спринт А6)
		v1.GET("/catalog", handleGetCatalog)     // ← настоящий (гостевой экран + агент)
		v1.GET("/cases/odds", handleGetCaseOdds) // таблица шансов кейсов (прозрачность, RESEARCH §4)

		// Админка (role=admin/owner из users.role, миграция 008)
		adm := v1.Group("/admin")
		adm.Use(authMiddleware(), adminMiddleware())
		adm.GET("/overview", handleAdminOverview)
		adm.GET("/users", handleAdminUsers)
		adm.GET("/users/:id", handleAdminUserCard) // карточка гостя (спринт А2)
		adm.POST("/users/:id/ban", handleAdminBan)
		adm.POST("/users/:id/unban", handleAdminUnban)
		adm.POST("/users/:id/deposit", handleAdminDeposit)
		adm.POST("/users/:id/grant", handleAdminGrant)
		adm.GET("/grants", handleAdminGrants)
		adm.GET("/audit", handleAdminAudit)              // единая лента действий (спринт А4)
		adm.GET("/chat/pending", handleAdminChatPending) // очередь вызовов/сообщений (Б2)
		adm.POST("/chat/:id/ack", handleAdminChatAck)    // «принял» (Б2)
		adm.GET("/chats", handleAdminChatThreads)        // треды чата (Б3)
		adm.GET("/chats/:id", handleAdminChatThread)     // переписка с гостем (Б3)
		adm.POST("/chats/:id", handleAdminChatPost)      // ответ гостю (Б3)
		adm.GET("/computers", handleAdminComputers)
		adm.PATCH("/computers/:id/status", handleAdminSetComputerStatus) // ремонт (спринт А1)
		adm.POST("/computers/:id/power", handleAdminPCPower)             // вкл/перезагрузка/выкл (Б8)
		adm.POST("/computers/:id/session", handleAdminSeatGuest)         // посадить гостя (Б8)
		adm.GET("/shifts", handleAdminShifts)                            // шаблоны смен — сетка графика (Б11)
		adm.GET("/shifts/schedule", handleAdminShiftSchedule)            // график персонала (Б11)
		adm.GET("/shifts/now", handleAdminShiftsNow)                     // кто сейчас на смене (Б11)
		adm.GET("/waitlist", handleAdminWaitlist)                        // очередь-вейтлист (Б9)
		adm.POST("/waitlist", handleAdminWaitlistAdd)                    // поставить по нику (Б9)
		adm.DELETE("/waitlist/:id", handleAdminWaitlistRemove)           // снять из очереди (Б9)
		adm.GET("/sessions/active", handleAdminActiveSessions)
		adm.POST("/sessions/:id/end", handleAdminEndSession)
		adm.GET("/bookings", handleAdminBookings)
		adm.POST("/bookings", handleAdminCreateBooking) // walk-in (спринт А3)
		adm.POST("/bookings/:id/cancel", handleAdminCancelBooking)
		adm.POST("/bookings/:id/restore", handleAdminRestoreBooking) // undo отмены (спринт А3)
		adm.GET("/catalog", handleAdminCatalog)
		adm.POST("/catalog/:id/toggle", handleAdminCatalogToggle)
		adm.POST("/catalog/:id/sort", handleAdminCatalogSort) // Б0-и3: admin — только порядок

		// Товары и продажи (В2, миграция 024): ценник правит owner, а продажа,
		// приём поставки и корректировка остатка — операции смены.
		// Табель (В3-этап 4, миграция 026): отметиться может каждый за себя,
		// правит записи владелец.
		adm.POST("/work/start", handleWorkStart)
		adm.POST("/work/stop", handleWorkStop)
		adm.GET("/work/me", handleWorkMe)

		adm.GET("/goods", handleAdminGoods)
		adm.POST("/goods/:id/stock", handleAdminGoodStock)
		adm.GET("/sales", handleAdminSales)
		adm.POST("/sales", handleAdminSaleCreate)
		adm.POST("/sales/:id/void", handleAdminSaleVoid)

		// Owner-only: статистика и настройки экономики (спринт А5)
		own := adm.Group("")
		own.Use(ownerMiddleware())
		// Каталог: правка/удаление — конфигурация запуска на гостевых ПК (Б0-и3)
		own.POST("/catalog", handleAdminCatalogUpsert)
		own.DELETE("/catalog/:id", handleAdminCatalogDelete)
		// Отчёты за произвольный период (спринт В1): пресеты + «с…по…»,
		// сравнение с предыдущим периодом, четыре разреза.
		own.POST("/goods", handleAdminGoodCreate)
		own.PATCH("/goods/:id", handleAdminGoodUpdate)
		own.DELETE("/goods/:id", handleAdminGoodDelete)
		own.GET("/goods/:id/moves", handleAdminGoodMoves)
		own.GET("/reports/money", handleReportMoney)
		own.GET("/reports/guests", handleReportGuests)
		own.GET("/reports/load", handleReportLoad)
		own.GET("/reports/staff", handleReportStaff)
		// Смены и график персонала (Б11): правит только owner
		own.POST("/shifts", handleAdminShiftCreate)
		own.PATCH("/shifts/:id", handleAdminShiftUpdate)
		own.DELETE("/shifts/:id", handleAdminShiftDelete)
		own.POST("/shifts/:id/assign", handleAdminShiftAssign)
		own.DELETE("/shift-assignments/:id", handleAdminShiftUnassign)
		own.GET("/stats/shift", handleAdminStatsShift) // сводка смены (Б10)
		own.GET("/settings", handleAdminSettingsGet)
		own.PUT("/settings", handleAdminSettingsPut)
		// Персонал (спринт Б5): назначение/снятие админов вместо SQL
		own.GET("/staff", handleAdminStaffList)
		// Кадровая карточка (В3-этап 2, миграция 025): личные данные сотрудника
		// видит и правит только владелец.
		own.GET("/work", handleWorkList)
		own.POST("/work", handleWorkCreate)
		own.PATCH("/work/:id", handleWorkUpdate)
		own.DELETE("/work/:id", handleWorkDelete)
		own.POST("/staff/hire", handleAdminStaffHire)           // В3-этап 3: наём с нуля
		own.POST("/staff/:id/dismiss", handleAdminStaffDismiss) // увольнение как событие
		own.GET("/staff/archive", handleAdminStaffArchive)
		own.GET("/staff/:id/profile", handleAdminStaffProfileGet)
		own.PUT("/staff/:id/profile", handleAdminStaffProfilePut)
		own.POST("/staff", handleAdminStaffPromote)
		own.DELETE("/staff/:id", handleAdminStaffDemote)
		// Конструктор зала (спринт А8)
		own.POST("/computers", handleAdminCreateComputer)
		own.PATCH("/computers/:id", handleAdminUpdateComputer)
		own.DELETE("/computers/:id", handleAdminDeleteComputer)
		own.PATCH("/clubs/:id/layout", handleAdminClubLayout)
	}

	// ── Мобильное PWA (трек М, MOBILE.md): раздача статики ────────────────
	// GET /app — гостевое приложение web/app.html (телефон по Wi-Fi,
	// same-origin — CORS не нужен, решение №10). Плюс манифест, SW и иконки.
	if webDir := findWebDir(); webDir != "" {
		r.StaticFile("/app", filepath.Join(webDir, "app.html"))
		r.StaticFile("/sw.js", filepath.Join(webDir, "sw.js"))
		r.Static("/icons", filepath.Join(webDir, "icons"))
		// .webmanifest нет во встроенной mime-таблице Go — тип ставим сами
		manifest := filepath.Join(webDir, "app.webmanifest")
		r.GET("/app.webmanifest", func(c *gin.Context) {
			c.Header("Content-Type", "application/manifest+json; charset=utf-8")
			c.File(manifest)
		})
		log.Printf("PWA: раздаю /app из %s", webDir)
	} else {
		log.Println("PWA: папка web с app.html не найдена — /app отключён (подскажи путь через WEB_DIR)")
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
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"` // срок access-токена, сек
	User         *models.User `json:"user"`
}

func handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	// QA Б9–Б11: символьный инвариант ника (кавычки/скобки/бэктик — XSS-вектор
	// в разметку админки), та же проверка, что и при смене ника (profile.go)
	if !nicknameSafe(req.Nickname) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nickname_charset",
			"message": "В нике нельзя кавычки, угловые скобки и спецсимволы"})
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

	// Достижение «Добро пожаловать»: +5 очков навыка и Light-кейс.
	checkAchievements(user.ID, playerStats{LoginCount: 1})

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
	access, err := signToken(user.ID.String(), string(user.Role), tokenTypeAccess, accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "token_error", "message": "Не удалось создать токен"})
		return
	}
	refresh, err := signToken(user.ID.String(), string(user.Role), tokenTypeRefresh, refreshTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "token_error", "message": "Не удалось создать токен"})
		return
	}
	c.JSON(status, authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(accessTTL.Seconds()),
		User:         user,
	})
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

		claims, err := parseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "invalid_token", "message": "Токен недействителен или истёк"})
			return
		}

		// Refresh-токен на защищённые роуты не пускаем — только access.
		if tokenType(claims) != tokenTypeAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "wrong_token_type", "message": "Требуется access-токен"})
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "invalid_claims", "message": "Некорректные данные токена"})
			return
		}

		c.Set("user_id", sub)
		role, _ := claims["role"].(string)
		c.Set("user_role", role)
		c.Next()
	}
}

// handleAdminWS — live-канал админки (спринт А6). Браузерный WebSocket не
// умеет заголовки, поэтому access-токен приходит query-параметром и
// проверяется здесь вручную (typ=access + роль персонала).
func handleAdminWS(c *gin.Context) {
	claims, err := parseToken(c.Query("token"))
	if err != nil || tokenType(claims) != "access" || !roleIsStaff(claimString(claims, "role")) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "Нужен access-токен персонала"})
		return
	}
	if err := hub.ServeAdmin(c.Writer, c.Request); err != nil {
		log.Printf("WS admin: ошибка апгрейда: %v", err)
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

// crossOriginMiddleware — CSRF-защита стандартной библиотеки (Go 1.25+,
// http.CrossOriginProtection): небезопасные кросс-доменные браузерные запросы
// (POST/PUT/PATCH/DELETE с чужого origin) получают 403 по Sec-Fetch-Site /
// Origin. Безопасные методы (GET/HEAD/OPTIONS) и клиенты без браузерных
// заголовков (shell-agent, C#-сервис, curl) проходят всегда — поведение API
// для легитимных клиентов не меняется. Origin «null» пропускаем явно: киоск
// WebView2 и admin.html открываются с file:// до HTTPS-домена (спринт 6);
// после переезда на домен этот допуск убрать, а домен вписать в
// TRUSTED_ORIGINS (список через запятую: scheme://host[:port]).
func crossOriginMiddleware() gin.HandlerFunc {
	cop := http.NewCrossOriginProtection()
	for _, origin := range strings.Split(os.Getenv("TRUSTED_ORIGINS"), ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if err := cop.AddTrustedOrigin(origin); err != nil {
			log.Printf("TRUSTED_ORIGINS: пропускаю %q: %v", origin, err)
		}
	}
	return func(c *gin.Context) {
		if err := cop.Check(c.Request); err != nil && c.GetHeader("Origin") != "null" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "cross_origin", "message": "Кросс-доменный запрос отклонён"})
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

// findWebDir — где лежит web/ с app.html: WEB_DIR из окружения, /web
// (volume в docker-compose.yml), ../web (go run из backend/), ./web (бинарь
// рядом с web/). Пусто — раздача /app отключается, API работает как раньше.
func findWebDir() string {
	for _, dir := range []string{os.Getenv("WEB_DIR"), "/web", "../web", "./web"} {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "app.html")); err == nil {
			return dir
		}
	}
	return ""
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
