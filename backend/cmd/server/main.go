package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// @title           14:48 API
// @version         0.1.0
// @description     Геймифицированная экосистема управления компьютерными клубами
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	if os.Getenv("SERVER_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": "0.1.0",
			"service": "14:48 Backend",
		})
	})

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		auth.POST("/register", stub("Register"))
		auth.POST("/login", stub("Login"))
		auth.POST("/otp/send", stub("SendOTP"))
		auth.POST("/otp/verify", stub("VerifyOTP"))
		auth.POST("/refresh", stub("RefreshToken"))
		auth.POST("/logout", stub("Logout"))

		me := v1.Group("/me")
		me.GET("", stub("GetProfile"))
		me.PATCH("", stub("UpdateProfile"))
		me.GET("/cases", stub("GetCases"))
		me.POST("/cases/:id/open", stub("OpenCase"))
		me.GET("/talents", stub("GetTalents"))
		me.POST("/talents/invest", stub("InvestSP"))
		me.GET("/achievements", stub("GetAchievements"))
		me.GET("/sessions", stub("GetSessions"))

		clubs := v1.Group("/clubs")
		clubs.GET("", stub("GetClubs"))
		clubs.GET("/:id", stub("GetClub"))
		clubs.GET("/:id/computers", stub("GetComputers"))
		clubs.POST("/:id/bookings", stub("CreateBooking"))

		v1.GET("/ws/shell", stub("WebSocketShell"))
	}

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("14:48 Backend запущен → http://localhost:%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Ошибка запуска: %v", err)
	}
}

func stub(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(501, gin.H{
			"status":  "not_implemented",
			"handler": name,
			"message": "TODO: реализовать " + name,
		})
	}
}
