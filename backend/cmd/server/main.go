package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
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

	// Часовой пояс процесса = часовой пояс клуба (ревью 26.08). Контейнер
	// живёт в UTC, из-за чего «сегодня» в отчётах, дневные потолки админа и
	// ночная скидка таланта night_mode считались со сдвигом на 2 часа: скидка
	// «22:00–07:59» реально действовала утром, а не ночью. Все расчёты в коде
	// идут от time.Now().Location(), поэтому одна эта строка выравнивает их
	// разом. Дни и часы в SQL отчётов считает база — ей тот же пояс ставит
	// withClubTimeZone при подключении (З2-и1).
	time.Local = clubLocation
	models.ClubLocation = clubLocation // З2-и2: колонки date из базы — полночь клуба (models.ClubDay)
	log.Printf("Часовой пояс клуба: %s", clubLocation)

	jwtSecret = []byte(getenv("JWT_SECRET", devJWTSecret))
	loadAgentToken() // К4-и3: AGENT_TOKEN — /ws/shell и полный /catalog только агентам
	accessTTL = parseDuration(os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute)
	refreshTTL = parseDuration(os.Getenv("JWT_REFRESH_TTL"), 30*24*time.Hour)

	if os.Getenv("SERVER_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
		// Роль берётся из claims и в БД не перепроверяется, поэтому известный
		// секрет = любой желающий подписывает себе токен владельца и получает
		// депозиты, отчёты и настройки. В проде это не предупреждение, а стоп
		// (ревью 26.08: Dockerfile.railway/railway.json переменную не задают).
		if string(jwtSecret) == devJWTSecret {
			log.Fatal("JWT_SECRET не задан: в production запуск с дефолтным секретом " +
				"означает, что любой может подписать себе токен владельца. " +
				"Задайте JWT_SECRET (например: openssl rand -hex 32).")
		}
		if len(jwtSecret) < 32 {
			log.Fatalf("JWT_SECRET слишком короткий (%d символов): нужно не меньше 32.", len(jwtSecret))
		}
	}

	// ── Подключение к PostgreSQL ────────────────────────────────────────────
	// З2-и1 (OWNER.md, №4): пояс клуба — и соединению с базой. Отчёты делят
	// время на дни (::date) и часы (EXTRACT) в поясе СЕССИИ, а он по умолчанию
	// Etc/UTC и у postgres:16-alpine, и у Railway: утро пропадало из графика
	// дня, пик по часам съезжал на 2 часа.
	clubTZ := clubLocation.String()
	if clubTZ == "Local" { // CLUB_TZ не распознан: базе такой пояс не назвать
		log.Fatalf("CLUB_TZ=%q — такого часового пояса нет; нужен, например, Europe/Warsaw", os.Getenv("CLUB_TZ"))
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = buildDSN()
	}
	var err error
	db, err = gorm.Open(postgres.Open(withClubTimeZone(dsn, clubTZ)), &gorm.Config{})
	if err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	var dbTZ string
	if err := db.Raw("SHOW TimeZone").Scan(&dbTZ).Error; err != nil {
		log.Fatalf("База не ответила, в каком она часовом поясе: %v", err)
	}
	if !strings.EqualFold(dbTZ, clubTZ) {
		log.Fatalf("База считает время в поясе %s, а клуб живёт в %s — отчёты по дням и часам соврут. "+
			"В строке подключения (DATABASE_URL) задан другой пояс: уберите из неё timezone/TimeZone "+
			"или поставьте %s.", dbTZ, clubTZ, clubTZ)
	}
	log.Printf("Подключение к PostgreSQL установлено, часовой пояс соединения: %s", dbTZ)

	// ── WebSocket Hub (PC Shell) ────────────────────────────────────────────
	hub = websocket.NewHub()
	hub.OnAdminCall = agentAdminCall       // Б2: admin_call агента → чат + админки
	hub.StateFor = shellStateFor           // К4-и1: state в ответ на session_tick
	hub.OnShellOffline = agentLostOpen     // К8-и2: обрыв агента во время сессии → автозапись в журнал проблем
	hub.OnShellOnline = onShellOnline      // К8-и2: агент вернулся → длительность обрыва; Ж4: посадка ждала шелл → окно [Готов!]
	hub.OnSensResult = sensResultFromShell // С2-и4: агент применил сенсу → уведомление гостю
	go hub.Run()

	// ── Роуты ───────────────────────────────────────────────────────────────
	// Ж0-и5: логгер не пишет строки /ws/* — там в query живут тикет админки и
	// (у старых агентов) AGENT_TOKEN; gin.Default() писал их в лог целиком.
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/api/v1/ws/admin", "/api/v1/ws/shell"}}))
	r.Use(gin.Recovery())
	// К0-и5: кому верить про адрес клиента. gin по умолчанию доверяет ЛЮБОМУ
	// прокси, и X-Forwarded-For от самого клиента обходил лимитер по IP. Пусто
	// (клуб: ПК ходят напрямую) — не верить никому; список CIDR/IP через
	// запятую — для стенда за реальным прокси (Railway).
	if err := r.SetTrustedProxies(trustedProxies(os.Getenv("TRUSTED_PROXIES"))); err != nil {
		log.Fatalf("TRUSTED_PROXIES: %v", err)
	}
	r.Use(corsMiddleware())        // разрешаем запросы из браузера (веб-приложение)
	r.Use(bodyLimitMiddleware())   // потолок тела запроса — защита от OOM (ревью 26.08)
	r.Use(crossOriginMiddleware()) // CSRF-защита мутаций (Go 1.25+), GET/агент не трогает
	r.Use(rateLimitMiddleware())   // ~10 rps с IP (ТЗ 10.1), кроме /health и статики

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
		// К6-и1: вход по QR — код для экрана входа киоска (6/мин с IP), поллинг
		// киоска (pending | approved + токены один раз), «Войти на PC-01 в …?» для телефона
		auth.POST("/qr", handleQRCreate)
		auth.GET("/qr/:code", handleQRPoll)
		auth.GET("/qr/:code/info", handleQRInfo)

		me := v1.Group("/me")
		me.Use(authMiddleware())                                          // всё под /me требует JWT
		me.GET("", handleGetMe)                                           // ← настоящий
		me.PATCH("", handlePatchMe)                                       // ← настоящий
		me.POST("/password", handleChangeMyPassword)                      // смена своего пароля (Е0-и2)
		me.POST("/auth/qr/approve", handleQRApprove)                      // К6-и1: телефон подтверждает вход на ПК
		me.GET("/cases", handleGetMyCases)                                // ← настоящий
		me.POST("/cases/:id/open", handleOpenCase)                        // ← настоящий
		me.GET("/talents", handleGetMyTalents)                            // ← настоящий
		me.POST("/talents/invest", handleInvestTalent)                    // ← настоящий
		me.GET("/achievements", handleGetMyAchievements)                  // ← настоящий
		me.GET("/packages/catalog", handleMyPackageCatalog)               // пакеты глазами гостя (Е2-и1, Р12)
		me.GET("/packages", handleMyPackages)                             // свои пакеты и живые минуты (Е2-и2)
		me.POST("/packages", handleMyPackageBuy)                          // купить пакет с кошелька (Е2-и2, Р12)
		me.GET("/sessions", handleGetMySessions)                          // ← настоящий
		me.POST("/sessions/start", handleStartSession)                    // ← настоящий
		me.POST("/sessions/end", handleEndSession)                        // ← настоящий
		me.POST("/sessions/ready", handleSessionReady)                    // [Готов!] — точка отсчёта денег (Е1)
		me.POST("/sessions/pause", handlePauseSession)                    // пауза: время и деньги стоят (Г2)
		me.POST("/sessions/resume", handleResumeSession)                  // снять паузу (Г2)
		me.GET("/bookings", handleGetMyBookings)                          // ← настоящий
		me.DELETE("/bookings/:id", handleCancelBooking)                   // ← настоящий
		me.GET("/deposits", handleGetMyDeposits)                          // ← настоящий
		me.GET("/wallet", handleGetMyWallet)                              // кошелёк: баланс + журнал (трек Г, Г0)
		me.GET("/economy", handleGetEconomy)                              // ← настоящий (калькулятор + ранг)
		me.GET("/sensitivity", handleGetSensitivity)                      // ← настоящий (профиль сенсы)
		me.PUT("/sensitivity", handlePutSensitivity)                      // ← настоящий
		me.GET("/aim-sessions", handleGetAimSessions)                     // история калибровок + тёплый старт (С4-и1)
		me.POST("/aim-sessions", handlePostAimSession)                    // прогон AI-калибратора (С4-и1)
		me.POST("/chat", handleGuestChatPost)                             // чат/вызов админа (Б2)
		me.GET("/chat", handleGuestChatList)                              // переписка, поллинг (Б3)
		me.GET("/chat/unread", handleGuestChatUnread)                     // бэйдж непрочитанного (Б3)
		me.GET("/notifications", handleGetMyNotifications)                // тосты о действиях админа (Б4)
		me.GET("/waitlist", handleGetMyWaitlist)                          // своя позиция в очереди (Б9, задел PWA)
		me.POST("/waitlist", handleJoinWaitlist)                          // встать в очередь самому (Б9)
		me.DELETE("/waitlist", handleLeaveWaitlist)                       // выйти из очереди (Б9)
		me.GET("/menu", handleGetMyMenu)                                  // меню кухни (Г7)
		me.GET("/kitchen/orders", handleGetMyKitchenOrders)               // мои заказы (Г7)
		me.POST("/kitchen/orders", handleMyKitchenOrderCreate)            // заказ: кошелёк или у стойки (Г7)
		me.POST("/kitchen/orders/:id/cancel", handleMyKitchenOrderCancel) // передумал, пока не приняли (Г7)
		me.POST("/streak/freeze", handleBuyStreakFreeze)                  // заморозка стрика за монеты (Г6-и4)

		// Лидерборд (за JWT)
		v1.GET("/leaderboard", authMiddleware(), handleLeaderboard)
		v1.GET("/aim-rating", authMiddleware(), handleAimRating) // аим-рейтинг клуба по throughput (С4-и4)

		clubs := v1.Group("/clubs")
		clubs.GET("", handleGetClubs)                                      // ← настоящий
		clubs.GET("/:id", handleGetClub)                                   // ← настоящий
		clubs.GET("/:id/computers", handleGetClubComputers)                // ← настоящий
		clubs.POST("/:id/bookings", authMiddleware(), handleCreateBooking) // ← настоящий

		v1.GET("/board/state", handleBoardState)         // Ж8-и3: табло — без логина, по board_token
		v1.GET("/ws/shell", handleShellWS)               // ← настоящий
		v1.GET("/ws/admin", handleAdminWS)               // live-канал админки (спринт А6)
		v1.GET("/catalog", handleGetCatalog)             // ← настоящий (гостевой экран + агент)
		v1.GET("/catalog/:id/cover", handleCatalogCover) // обложка плитки из БД (П1)
		v1.GET("/cases/odds", handleGetCaseOdds)         // таблица шансов кейсов (прозрачность, RESEARCH §4)
		v1.GET("/goods/:id/photo", handleGoodPhoto)      // фото позиции кухни — как статика обложек (Г7)

		// Админка (role=admin/owner из users.role, миграция 008)
		adm := v1.Group("/admin")
		adm.Use(authMiddleware(), adminMiddleware())
		adm.GET("/overview", handleAdminOverview)
		adm.GET("/users", handleAdminUsers)
		adm.GET("/users/:id", handleAdminUserCard)                      // карточка гостя (спринт А2)
		adm.PATCH("/users/:id", handleAdminGuestUpdate)                 // правка данных гостя (Е0-и1)
		adm.POST("/users/:id/password/reset", handleAdminPasswordReset) // временный пароль (Е0-и2)
		adm.POST("/users/:id/ban", handleAdminBan)
		adm.POST("/users/:id/unban", handleAdminUnban)
		adm.POST("/users/:id/deposit", handleAdminDeposit)
		adm.POST("/deposits/:id/void", handleAdminDepositVoid) // Ж1: отмена ошибочного депозита по правилам продаж
		adm.POST("/users/:id/grant", handleAdminGrant)
		adm.GET("/grants", handleAdminGrants)
		adm.GET("/audit", handleAdminAudit)         // единая лента действий (спринт А4)
		adm.GET("/incidents", handleAdminIncidents) // К8: журнал проблем (?open=1)
		adm.POST("/incidents", handleAdminIncidentCreate)
		adm.POST("/incidents/:id/compensate", handleAdminIncidentCompensate) // минуты/деньги гостю
		adm.POST("/incidents/:id/resolve", handleAdminIncidentResolve)       // закрыть без компенсации
		adm.GET("/chat/pending", handleAdminChatPending)                     // очередь вызовов/сообщений (Б2)
		adm.POST("/chat/:id/ack", handleAdminChatAck)                        // «принял» (Б2)
		adm.GET("/chats", handleAdminChatThreads)                            // треды чата (Б3)
		adm.GET("/chats/:id", handleAdminChatThread)                         // переписка с гостем (Б3)
		adm.POST("/chats/:id", handleAdminChatPost)                          // ответ гостю (Б3)
		adm.GET("/computers", handleAdminComputers)
		adm.PATCH("/computers/:id/status", handleAdminSetComputerStatus) // ремонт (спринт А1)
		adm.POST("/computers/:id/power", handleAdminPCPower)             // вкл/перезагрузка/выкл (Б8)
		adm.POST("/computers/:id/release", handleAdminPCRelease)         // Ж4-и3 (Ж-Р9): «Освободить ПК» — занят без сессии
		adm.POST("/computers/:id/session", handleAdminSeatGuest)         // посадить гостя (Б8)
		adm.POST("/computers/:id/hold", handleAdminComputerHold)         // придержать под регистрацию (Е1-и5)
		adm.DELETE("/computers/:id/hold", handleAdminComputerUnhold)     // снять придержание (Е1-и5)
		adm.GET("/shifts", handleAdminShifts)                            // шаблоны смен — сетка графика (Б11)
		adm.GET("/shifts/schedule", handleAdminShiftSchedule)            // график персонала (Б11)
		adm.GET("/shifts/now", handleAdminShiftsNow)                     // кто сейчас на смене (Б11)
		adm.GET("/waitlist", handleAdminWaitlist)                        // очередь-вейтлист (Б9)
		adm.POST("/waitlist", handleAdminWaitlistAdd)                    // поставить по нику (Б9)
		adm.DELETE("/waitlist/:id", handleAdminWaitlistRemove)           // снять из очереди (Б9)
		adm.GET("/sessions/active", handleAdminActiveSessions)
		adm.POST("/sessions/:id/end", handleAdminEndSession)
		adm.POST("/sessions/:id/move", handleAdminMoveSession)       // пересадка на другой ПК (Г2-и3)
		adm.POST("/sessions/:id/seat-cancel", handleAdminSeatCancel) // отмена посадки до [Готов!] (Е1)
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
		adm.POST("/work/stop", handleWorkStop) // Ж6-и4: + {counted_pln, note} — сдача кассы
		adm.GET("/work/me", handleWorkMe)
		adm.GET("/work/closeout", handleWorkCloseoutPreview) // Ж6-и4: сколько должно быть в ящике
		adm.POST("/cash/events", handleCashEventCreate)      // З8-и2: инкассация; коррекция — владелец (проверка в ручке)
		adm.GET("/staff-chat", handleStaffChatList)          // Е6: канал админ ↔ владелец (Р7)
		adm.POST("/staff-chat", handleStaffChatPost)
		adm.POST("/staff-chat/read", handleStaffChatRead)
		adm.POST("/ws-ticket", handleAdminWSTicket)      // Ж0-и5: одноразовый тикет для /ws/admin
		adm.GET("/onboarding", handleOnboardingState)    // Е5: нужен ли тур (Р9)
		adm.POST("/onboarding", handleOnboardingDone)    // тур показан
		adm.DELETE("/onboarding", handleOnboardingReset) // пройти заново

		adm.GET("/zones", handleAdminZones) // В4-1: цену часа видно и у стойки
		// В4-2: обмен монет на время — операция смены, делает админ у стойки
		adm.POST("/users/:id/redeem", handleAdminRedeemCoins)
		adm.POST("/coin-redemptions/:id/void", handleCoinRedeemVoid) // В4-4: откат ошибочной выдачи

		adm.GET("/packages", handleAdminPackages)                   // каталог пакетов времени (Е2-и1)
		adm.POST("/users/:id/packages", handleAdminPackageIssue)    // выдать пакет гостю (Е2-и2)
		adm.GET("/users/:id/packages", handleAdminUserPackages)     // пакеты гостя (Е2-и2)
		adm.POST("/user-packages/:id/void", handleAdminPackageVoid) // отмена ошибочной выдачи (Е2-и2)
		adm.GET("/goods", handleAdminGoods)
		adm.POST("/goods/:id/stock", handleAdminGoodStock)
		adm.GET("/supplies", handleSupplies)                 // З10-и4: поставки — ждут цен
		adm.POST("/supplies", handleSupplyCreate)            // приёмка строками «позиция × штук»
		adm.PATCH("/supplies/:id", handleSupplyUpdate)       // правка штук, пока ждёт цен (с пометкой)
		adm.POST("/supplies/:id/cancel", handleSupplyCancel) // отмена целиком, если ничего не ушло
		adm.GET("/sales", handleAdminSales)
		adm.POST("/sales", handleAdminSaleCreate) // Ж6: корзина items[] одним чеком, method=later — в долг гостю за ПК
		adm.POST("/sales/:id/void", handleAdminSaleVoid)
		adm.POST("/receipts/:id/void", handleAdminReceiptVoid)           // Ж6-и2: отменить чек целиком
		adm.GET("/users/:id/debt", handleAdminUserDebt)                  // Ж6-и1: долг гостя — кухня + бар, позиции
		adm.POST("/users/:id/settle", handleAdminUserSettle)             // Ж6-и1 (Ж-Р8): «Рассчитать» — одной операцией
		adm.GET("/seat-group/suggest", handleAdminSeatGroupSuggest)      // Ж8-и2 (Ж-Р13): соседние свободные ПК под компанию
		adm.POST("/seat-group", handleAdminSeatGroup)                    // Ж8-и2: посадить компанию одним запросом
		adm.GET("/kitchen/orders", handleAdminKitchenOrders)             // очередь кухни (Г7)
		adm.POST("/kitchen/orders", handleAdminKitchenOrderCreate)       // Ж6-и3: заказ от стойки (гость через резолвер)
		adm.POST("/kitchen/orders/:id/status", handleAdminKitchenStatus) // принял→готовит→несут→выдан (Г7)
		adm.POST("/kitchen/orders/:id/pay", handleAdminKitchenPay)       // расчёт у стойки после игры (Р10)
		adm.POST("/kitchen/orders/:id/cancel", handleAdminKitchenCancel) // отмена с возвратом склада (Г7)

		// Owner-only: статистика и настройки экономики (спринт А5)
		own := adm.Group("")
		own.Use(ownerMiddleware())
		// Каталог: правка/удаление — конфигурация запуска на гостевых ПК (Б0-и3)
		own.POST("/catalog", handleAdminCatalogUpsert)
		own.DELETE("/catalog/:id", handleAdminCatalogDelete)
		// П1: наполнение каталога владельцем — обложки, корень игр, импорт/экспорт
		own.GET("/catalog/config", handleAdminCatalogConfigGet) // корень игр для {GAMES}
		own.PUT("/catalog/config", handleAdminCatalogConfigPut)
		own.GET("/catalog/export", handleAdminCatalogExport)      // перенос каталога между клубами
		own.POST("/catalog/import", handleAdminCatalogImport)     // пачкой, dry_run — превью
		own.PUT("/catalog/:id/cover", handleAdminCatalogCoverPut) // обложка плитки (как фото кухни)
		own.DELETE("/catalog/:id/cover", handleAdminCatalogCoverDelete)
		// Отчёты за произвольный период (спринт В1): пресеты + «с…по…»,
		// сравнение с предыдущим периодом, четыре разреза.
		own.POST("/packages", handleAdminPackageCreate) // конструктор пакетов (Е2-и1)
		own.PATCH("/packages/:id", handleAdminPackageUpdate)
		own.DELETE("/packages/:id", handleAdminPackageDelete)
		own.GET("/packages/expiring", handleAdminPackagesExpiring) // кого предупредим (Е2-и5)
		own.POST("/packages/warn", handleAdminPackagesWarnRun)     // предупредить сейчас
		own.POST("/goods", handleAdminGoodCreate)
		own.PATCH("/goods/:id", handleAdminGoodUpdate)
		own.DELETE("/goods/:id", handleAdminGoodDelete)
		own.GET("/goods/:id/moves", handleAdminGoodMoves)
		own.GET("/goods/lots/check", handleGoodsLotsCheck)       // З10-и3: сумма партий = остаток
		own.GET("/supplies/:id/pricing", handleSupplyPricing)    // З10-и5: поставка с ценами
		own.PUT("/supplies/:id/pricing", handleSupplyPricingPut) // оценить или переоценить: цены строк, документ, расход
		own.GET("/purchases", handlePurchases)                   // закупки: ждут цен и оценённые — для «Закупок» (З10-и6)
		own.GET("/goods/:id/lots", handleGoodLots)               // З10-и7: партии позиции — «бой» из выбранной
		own.PUT("/lots/:id/price", handleLotPrice)               // цена начального остатка и «нашлось»
		own.PUT("/goods/:id/photo", handleAdminGoodPhotoPut)     // фото позиции (Г7)
		own.DELETE("/goods/:id/photo", handleAdminGoodPhotoDelete)
		own.GET("/reports/coins", handleReportCoins)  // В4-2: эмиссия и обязательства
		own.GET("/coins/burn", handleCoinBurnPreview) // В4-3: кто потеряет монеты сейчас
		own.POST("/coins/burn", handleCoinBurnRun)
		own.GET("/reports/money", handleReportMoney)
		own.GET("/reports/guests", handleReportGuests)
		own.GET("/reports/load", handleReportLoad)
		own.GET("/reports/staff", handleReportStaff)
		own.GET("/reports/returns", handleReportReturns)       // З11-и3: возвраты месяца по дню отмены
		own.POST("/cash/events/:id/void", handleCashEventVoid) // З8-и2: отмена события ящика
		own.GET("/cash/book", handleCashBook)                  // З8-и3: кассовая книга по клубным дням
		// З12-и3: «Прибыль» за период — доходы, возвраты, себестоимость, потери, расходы, результат
		own.GET("/reports/profit", handleReportProfit)
		// З13-и3: продажи по ставкам VAT за период — по настройкам «Бухгалтерии», по дням — для сверки с фискальной кассой
		own.GET("/reports/vat", handleReportVat)
		// З13-и4: выгрузка бухгалтеру — XLSX или архив CSV, заголовки PL или RU
		own.GET("/export", handleExport)
		// З9-и1: расходы, категории и контрагенты — только владелец (З-Р29)
		own.GET("/expense-categories", handleExpenseCategories)
		own.POST("/expense-categories", handleExpenseCategoryCreate)      // З9-и4: своя категория
		own.PATCH("/expense-categories/:id", handleExpenseCategoryUpdate) // переименовать, скрыть, закупка
		own.GET("/counterparties", handleCounterparties)
		own.POST("/counterparties", handleCounterpartyCreate)
		own.PATCH("/counterparties/:id", handleCounterpartyUpdate) // З9-и4: правка, скрыть
		own.GET("/expense-methods", handleExpenseMethods)          // З9-и4: способы оплаты вкл/выкл
		own.PUT("/expense-methods", handleExpenseMethodsPut)
		own.GET("/expense-templates", handleExpenseTemplates) // З9-и5: регулярные расходы
		own.POST("/expense-templates", handleExpenseTemplateCreate)
		own.PATCH("/expense-templates/:id", handleExpenseTemplateUpdate) // правка, выключить, включить
		own.GET("/expenses/due", handleExpensesDue)                      // ждут внесения
		own.POST("/expenses/due/remind", handleExpensesDueRemind)        // напомнить сейчас — как фоновый прогон
		own.GET("/expenses", handleExpenseList)
		own.POST("/expenses", handleExpenseCreate)
		own.PATCH("/expenses/:id", handleExpenseUpdate)
		own.POST("/expenses/:id/storno", handleExpenseStorno) // удаления нет — сторно
		own.GET("/expenses/:id/photo", handleExpensePhoto)    // фото документа — только владельцу
		own.PUT("/expenses/:id/photo", handleExpensePhotoPut)
		own.DELETE("/expenses/:id/photo", handleExpensePhotoDelete)
		own.GET("/owner-debt", handleOwnerDebt)                      // З9-и3б: клуб должен владельцу и возвраты
		own.POST("/owner-repayments", handleOwnerRepay)              // З9-и3б: вернули владельцу — из кассы или с банка
		own.POST("/owner-repayments/:id/void", handleOwnerRepayVoid) // З9-и3б: отмена ошибочного возврата
		// З11-и4: долг гостя — списать одной кнопкой и вернуть списанный
		own.POST("/users/:id/debt/write-off", handleDebtWriteOff)
		own.POST("/debt-write-offs/:id/cancel", handleDebtWriteOffCancel)
		// Смены и график персонала (Б11): правит только owner
		own.POST("/shifts", handleAdminShiftCreate)
		own.PATCH("/shifts/:id", handleAdminShiftUpdate)
		own.DELETE("/shifts/:id", handleAdminShiftDelete)
		own.POST("/shifts/:id/assign", handleAdminShiftAssign)
		own.DELETE("/shift-assignments/:id", handleAdminShiftUnassign)
		own.GET("/stats/shift", handleAdminStatsShift) // сводка смены (Б10)
		own.GET("/settings", handleAdminSettingsGet)
		own.PUT("/settings", handleAdminSettingsPut)
		// З13-и1: ставки VAT — список для экрана и ставка всей категории позиций одной кнопкой
		own.GET("/vat/rates", handleVatRates)
		own.PUT("/vat/goods", handleGoodsVat)
		// З13-и6: профили учёта — ставки ryczałtu и колонки KPiR для экрана, ставка ryczałtu всей категории позиций,
		// реквизиты фирмы для выгрузки
		own.GET("/ryczalt/rates", handleRyczaltRates)
		own.PUT("/ryczalt/goods", handleGoodsRyczalt)
		own.GET("/kpir/columns", handleKpirColumns)
		own.GET("/export/requisites", handleRequisites)
		own.PUT("/export/requisites", handleRequisitesPut)
		own.GET("/board/token", handleAdminBoardTokenGet)       // Ж8-и3 (Ж-Р12): токен и ссылка табло
		own.POST("/board/token", handleAdminBoardTokenRotate)   // выпустить новый
		own.DELETE("/board/token", handleAdminBoardTokenDelete) // выключить табло
		// Персонал (спринт Б5): назначение/снятие админов вместо SQL
		own.GET("/staff", handleAdminStaffList)
		// Кадровая карточка (В3-этап 2, миграция 025): личные данные сотрудника
		// видит и правит только владелец.
		own.POST("/zones", handleAdminZoneCreate)
		own.PATCH("/zones/:id", handleAdminZoneUpdate)
		own.DELETE("/zones/:id", handleAdminZoneDelete)
		own.GET("/work", handleWorkList)
		own.POST("/work", handleWorkCreate)
		own.PATCH("/work/:id", handleWorkUpdate)
		own.DELETE("/work/:id", handleWorkDelete)
		own.POST("/staff/hire", handleAdminStaffHire)           // В3-этап 3: наём с нуля
		own.POST("/staff/:id/dismiss", handleAdminStaffDismiss) // увольнение как событие
		// З14-и1: открытая смена сотрудника — что останется за ним, если уволить или снять права сейчас
		own.GET("/staff/:id/shift", handleAdminStaffShift)
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
		// страница регистрации (Р9, миграция 043): same-origin с /app —
		// токены в localStorage общие, после реги PWA уже «вошло»
		r.StaticFile("/register", filepath.Join(webDir, "register.html"))
		r.StaticFile("/sw.js", filepath.Join(webDir, "sw.js"))
		r.Static("/icons", filepath.Join(webDir, "icons"))
		// .webmanifest нет во встроенной mime-таблице Go — тип ставим сами
		manifest := filepath.Join(webDir, "app.webmanifest")
		r.GET("/app.webmanifest", func(c *gin.Context) {
			c.Header("Content-Type", "application/manifest+json; charset=utf-8")
			c.File(manifest)
		})
		// Стойка/демо: админка и обложки кейсов с того же origin, что и API
		// (same-origin — CrossOriginProtection пропускает без допусков).
		r.StaticFile("/admin", filepath.Join(webDir, "admin.html"))
		// Ж8-и3 (Ж-Р12): табло зала под ТВ — своя страница, ходит в
		// /api/v1/board/state с токеном из URL, логина нет.
		r.StaticFile("/board", filepath.Join(webDir, "board.html"))
		// Витрина гостевого экрана (демо 27.08): тот же shell.html, что в
		// киоске; запуск игр тут не работает — им занимается агент на ПК.
		r.StaticFile("/shell", filepath.Join(webDir, "shell.html"))
		// AI-калибратор сенсы: шелл открывает его относительной ссылкой
		// aim.html?dpi=… — с /shell это /aim.html (мост через localStorage,
		// API странице не нужен).
		r.StaticFile("/aim.html", filepath.Join(webDir, "aim.html"))
		r.Static("/covers", filepath.Join(webDir, "covers"))
		log.Printf("PWA: раздаю /app из %s", webDir)
	} else {
		log.Println("PWA: папка web с app.html не найдена — /app отключён (подскажи путь через WEB_DIR)")
	}

	// В4-3: таяние монет у неактивных гостей. Фоновый прогон безопасен при
	// перезапусках — жечь чаще раза в неделю на гостя правило не даёт.
	startCoinBurnJob()
	startPackageWarnJob() // Е2-и5: сказать, что пакет вот-вот сгорит
	startExpenseDueJob()  // З9-и5: регулярный расход ждёт — одно напоминание владельцу в день срока

	// Г1: поминутный биллинг кошелька активных сессий. Вся память — в БД,
	// перезапуск догоняет прошедшие минуты одним доначислением.
	startWalletBillingJob()

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
	// З3-и1 (№19): кто входит — "admin" шлёт админка; киоск и PWA не шлют ничего.
	// Смену открывает только вход в админку: вход из дома в PWA часов не рождает.
	Client string `json:"client"`
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
		respondBindError(c, err)
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
		respondDBError(c, err)
		return
	}

	// Достижение «Добро пожаловать»: +5 очков навыка и Light-кейс.
	checkAchievements(user.ID, playerStats{LoginCount: 1})

	writeAuth(c, http.StatusCreated, &user)
}

func handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	// К0-и5: перебор пароля режем по аккаунту, а не по IP (IP подделывается
	// заголовком X-Forwarded-For); несуществующий логин считается так же, как
	// гостевой. З0-и3 (№28): сперва — кто это: персоналу после серии не замок,
	// а темп (loginguard.go), серия неудач — строка login_failed владельцу.
	now := time.Now()
	var user models.User
	found := db.Where("nickname = ? OR email = ?", req.Login, req.Login).First(&user).Error == nil
	staff := found && roleIsStaff(string(user.Role))
	if locked, wait := loginGuardian.locked(req.Login, staff, now); locked {
		if staff {
			secs := int((wait + time.Second - 1) / time.Second)
			c.Header("Retry-After", fmt.Sprint(secs))
			c.JSON(http.StatusTooManyRequests, gin.H{"code": "login_paced", "retry_after": secs,
				"message": fmt.Sprintf("Много неверных паролей подряд — следующая попытка через %d с", secs)})
			return
		}
		c.JSON(http.StatusTooManyRequests, gin.H{"code": "login_locked",
			"message": fmt.Sprintf("Слишком много неверных попыток. Подожди %d мин или подойди к стойке — админ сбросит пароль",
				int(wait.Minutes())+1)})
		return
	}

	if !found {
		loginGuardian.fail(req.Login, false, now)
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "message": "Неверный логин или пароль"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		if n, report := loginGuardian.fail(req.Login, staff, now); report {
			logLoginFailed(&user, n, c.ClientIP())
		}
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "message": "Неверный логин или пароль"})
		return
	}
	loginGuardian.reset(req.Login)

	if user.Status == models.UserStatusBanned {
		c.JSON(http.StatusForbidden, gin.H{"code": "banned", "message": "Аккаунт заблокирован"})
		return
	}

	db.Model(&user).Update("last_active_at", time.Now())
	checkBirthdayGift(user.ID) // Г8-и4: вдруг сегодня день рождения — подарок при входе
	if req.Client == "admin" {
		openWorkOnLogin(&user, time.Now()) // Е3-и1 (Р8), З3-и1: смену открывает вход в админку
	}
	writeAuth(c, http.StatusOK, &user)
}

func writeAuth(c *gin.Context, status int, user *models.User) {
	writeAuthAt(c, status, user, time.Now())
}

// writeAuthAt — пара токенов с явным моментом выдачи (Е0-и2: смена пароля
// подписывает пару моментом собственной отсечки).
func writeAuthAt(c *gin.Context, status int, user *models.User, issued time.Time) {
	resp, err := buildAuth(user, issued)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "token_error", "message": "Не удалось создать токен"})
		return
	}
	c.JSON(status, resp)
}

// buildAuth — пара токенов гостя (вход, регистрация, смена пароля, QR-вход К6).
func buildAuth(user *models.User, issued time.Time) (authResponse, error) {
	access, err := signTokenAt(user.ID.String(), string(user.Role), tokenTypeAccess, accessTTL, issued)
	if err != nil {
		return authResponse{}, err
	}
	refresh, err := signTokenAt(user.ID.String(), string(user.Role), tokenTypeRefresh, refreshTTL, issued)
	if err != nil {
		return authResponse{}, err
	}
	return authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(accessTTL.Seconds()),
		User:         user,
	}, nil
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
// умеет заголовков, поэтому в URL приходит одноразовый тикет (Ж0-и5,
// wsticket.go), взятый секундой раньше обычным запросом с Authorization.
// Access-токен в query больше не принимается — он оседал в логах.
func handleAdminWS(c *gin.Context) {
	tk, ok := wsTickets.redeem(c.Query("ticket"), time.Now())
	if !ok || !roleIsStaff(tk.Role) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "Нужен свежий тикет персонала (POST /admin/ws-ticket)"})
		return
	}
	// З0-и1: тикет живёт 30 с — за это время человека могли уволить; роль — из базы в момент апгрейда.
	if role, active, err := staffRoleNow(tk.UserID); err != nil || !active || !roleIsStaff(role) {
		c.JSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "Нужны права администратора"})
		return
	}
	if err := hub.ServeAdmin(c.Writer, c.Request, tk.UserID); err != nil {
		log.Printf("WS admin: ошибка апгрейда: %v", err)
	}
}

// handleShellWS — апгрейд соединения PC Shell до WebSocket.
// dev: токен ПК пока не проверяем (TODO: аутентификация по MAC + токену).
func handleShellWS(c *gin.Context) {
	// К4-и3: без верного токена агента сокет не поднимаем (при заданном AGENT_TOKEN)
	if !agentAuthorized(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "agent_token_invalid", "message": "Нужен токен агента (AGENT_TOKEN сервера в agent.json / appsettings.json)"})
		return
	}
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

// onShellOnline — агент вышел на связь: закрыть автозапись обрыва (К8-и2) и,
// если на этом ПК посадка ждала шелл, открыть окно [Готов!] (Ж4-и1).
func onShellOnline(computerID string) {
	agentLostClose(computerID)
	shellArrivedForPC(computerID)
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

// devJWTSecret — заведомо небезопасный дефолт для локальной разработки.
// В production сервер с ним не стартует (см. main).
const devJWTSecret = "dev_insecure_secret_change_me"

// maxRequestBody — потолок тела запроса. ShouldBindJSON разбирает JSON в
// память ДО любой валидации, поэтому без потолка публичный /auth/login с
// гигабайтным телом укладывает контейнер в OOM (ревью 26.08).
// З0-и2 (№32): исключений нет. Раньше из-под потолка по префиксу выпадали все
// /admin/goods*, и 3 МБ JSON в приход склада разбирался целиком и ложился в
// журнал. Фото товара и обложка режутся своими LimitReader на 500 КБ — это
// меньше общего потолка, отдельный путь им не нужен.
const maxRequestBody = 1 << 20 // 1 МБ

func bodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		// заявленный размер больше потолка — отказ сразу: тело не читаем и не разбираем
		if c.Request.ContentLength > maxRequestBody {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"code": "body_too_large",
				"message": "Запрос больше 1 МБ — сервер такие не принимает"})
			return
		}
		// размер не заявлен (chunked) или занижен — чтение оборвётся на потолке
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBody)
		c.Next()
	}
}

// safely — обёртка одного прогона фоновой задачи. Паника в горутине НЕ
// перехватывается gin.Recovery и роняет весь процесс: один сбойный тик
// биллинга не должен закрывать клуб (ревью 26.08).
func safely(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ПАНИКА в фоновой задаче %s: %v\n%s", name, r, debug.Stack())
		}
	}()
	fn()
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

// trustedProxies — список из TRUSTED_PROXIES; пусто = nil (никому не верить).
func trustedProxies(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
		" password=" + pass + " dbname=" + name + " sslmode=disable" // пояс добавит withClubTimeZone
}

// withClubTimeZone — строка подключения с поясом клуба (З2-и1, чистая, тест):
// URL получает параметр timezone, key=value — ещё одну пару. Пояс в строке уже
// есть (timezone, TimeZone, time_zone) — она не меняется: верен ли он, скажет
// SHOW TimeZone сразу после подключения. Имя пояса — как есть, без %2F: драйвер
// gorm берёт его из строки сырым и сам зовёт time.LoadLocation.
func withClubTimeZone(dsn, tz string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn // что не так с адресом, скажет драйвер
		}
		for k := range u.Query() {
			if dsnTimeZoneKey(k) {
				return dsn
			}
		}
		sep := "?"
		switch {
		case strings.HasSuffix(dsn, "?") || strings.HasSuffix(dsn, "&"):
			sep = ""
		case u.RawQuery != "":
			sep = "&"
		}
		return dsn + sep + "timezone=" + tz
	}
	for _, f := range strings.Fields(dsn) {
		if k, _, ok := strings.Cut(f, "="); ok && dsnTimeZoneKey(k) {
			return dsn
		}
	}
	return strings.TrimSpace(dsn + " timezone=" + tz)
}

func dsnTimeZoneKey(k string) bool {
	k = strings.ToLower(strings.TrimSpace(k))
	return k == "timezone" || k == "time_zone"
}
