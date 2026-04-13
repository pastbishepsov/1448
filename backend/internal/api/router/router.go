package router

import (
	"github.com/1448-project/backend/internal/api/handlers"
	"github.com/1448-project/backend/internal/api/middleware"
	"github.com/1448-project/backend/internal/config"
	"github.com/gin-gonic/gin"
)

func New(cfg *config.Config) *gin.Engine {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "14:48 API"})
	})

	// API v1
	v1 := r.Group("/api/v1")
	{
		// ── AUTH ─────────────────────────────────────────────────────
		auth := v1.Group("/auth")
		{
			auth.POST("/register", handlers.Register)
			auth.POST("/login", handlers.Login)
			auth.POST("/otp/send", handlers.SendOTP)
			auth.POST("/otp/verify", handlers.VerifyOTP)
			auth.POST("/refresh", handlers.RefreshToken)
			auth.POST("/logout", middleware.Auth(cfg), handlers.Logout)
		}

		// ── PROTECTED (требует JWT) ──────────────────────────────────
		protected := v1.Group("")
		protected.Use(middleware.Auth(cfg))
		{
			// Профиль и прогрессия
			protected.GET("/me", handlers.GetMe)
			protected.PATCH("/me", handlers.UpdateMe)
			protected.GET("/me/achievements", handlers.GetMyAchievements)
			protected.GET("/me/cases", handlers.GetMyCases)
			protected.POST("/me/cases/:id/open", handlers.OpenCase)
			protected.GET("/me/talents", handlers.GetMyTalents)
			protected.POST("/me/talents/invest", handlers.InvestTalent)
			protected.GET("/me/sessions", handlers.GetMySessions)

			// Клубы и бронь
			protected.GET("/clubs", handlers.GetClubs)
			protected.GET("/clubs/:id", handlers.GetClub)
			protected.GET("/clubs/:id/computers", handlers.GetClubComputers)
			protected.POST("/clubs/:id/bookings", handlers.CreateBooking)
			protected.GET("/me/bookings", handlers.GetMyBookings)
			protected.DELETE("/me/bookings/:id", handlers.CancelBooking)
		}

		// ── WEBSOCKET (PC Shell) ─────────────────────────────────────
		v1.GET("/ws/shell", middleware.ShellAuth(cfg), handlers.ShellWebSocket)

		// ── ADMIN (требует staff PIN) ────────────────────────────────
		admin := v1.Group("/admin")
		admin.Use(middleware.StaffAuth(cfg))
		{
			admin.GET("/guests", handlers.AdminGetGuests)
			admin.GET("/guests/:id", handlers.AdminGetGuest)
			admin.POST("/guests/:id/xp", handlers.AdminGrantXP)
			admin.POST("/guests/:id/case", handlers.AdminGrantCase)
			admin.POST("/guests/:id/ban", handlers.AdminBanGuest)

			admin.GET("/computers", handlers.AdminGetComputers)
			admin.POST("/computers/:id/session/start", handlers.AdminStartSession)
			admin.POST("/computers/:id/session/end", handlers.AdminEndSession)
			admin.POST("/computers/:id/lock", handlers.AdminLockPC)
			admin.POST("/computers/:id/unlock", handlers.AdminUnlockPC)

			admin.GET("/bookings", handlers.AdminGetBookings)
			admin.PATCH("/bookings/:id", handlers.AdminUpdateBooking)
		}
	}

	return r
}
