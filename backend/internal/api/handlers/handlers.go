package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// Заглушки — каждый разработчик реализует свою часть
// Go-разработчик: заменяет TODO на реальную логику

func Register(c *gin.Context)       { c.JSON(http.StatusNotImplemented, todo("Register")) }
func Login(c *gin.Context)          { c.JSON(http.StatusNotImplemented, todo("Login")) }
func SendOTP(c *gin.Context)        { c.JSON(http.StatusNotImplemented, todo("SendOTP")) }
func VerifyOTP(c *gin.Context)      { c.JSON(http.StatusNotImplemented, todo("VerifyOTP")) }
func RefreshToken(c *gin.Context)   { c.JSON(http.StatusNotImplemented, todo("RefreshToken")) }
func Logout(c *gin.Context)         { c.JSON(http.StatusNotImplemented, todo("Logout")) }

func GetMe(c *gin.Context)          { c.JSON(http.StatusNotImplemented, todo("GetMe")) }
func UpdateMe(c *gin.Context)       { c.JSON(http.StatusNotImplemented, todo("UpdateMe")) }
func GetMyAchievements(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("GetMyAchievements")) }
func GetMyCases(c *gin.Context)     { c.JSON(http.StatusNotImplemented, todo("GetMyCases")) }
func OpenCase(c *gin.Context)       { c.JSON(http.StatusNotImplemented, todo("OpenCase")) }
func GetMyTalents(c *gin.Context)   { c.JSON(http.StatusNotImplemented, todo("GetMyTalents")) }
func InvestTalent(c *gin.Context)   { c.JSON(http.StatusNotImplemented, todo("InvestTalent")) }
func GetMySessions(c *gin.Context)  { c.JSON(http.StatusNotImplemented, todo("GetMySessions")) }

func GetClubs(c *gin.Context)       { c.JSON(http.StatusNotImplemented, todo("GetClubs")) }
func GetClub(c *gin.Context)        { c.JSON(http.StatusNotImplemented, todo("GetClub")) }
func GetClubComputers(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("GetClubComputers")) }
func CreateBooking(c *gin.Context)  { c.JSON(http.StatusNotImplemented, todo("CreateBooking")) }
func GetMyBookings(c *gin.Context)  { c.JSON(http.StatusNotImplemented, todo("GetMyBookings")) }
func CancelBooking(c *gin.Context)  { c.JSON(http.StatusNotImplemented, todo("CancelBooking")) }

func AdminGetGuests(c *gin.Context)   { c.JSON(http.StatusNotImplemented, todo("AdminGetGuests")) }
func AdminGetGuest(c *gin.Context)    { c.JSON(http.StatusNotImplemented, todo("AdminGetGuest")) }
func AdminGrantXP(c *gin.Context)     { c.JSON(http.StatusNotImplemented, todo("AdminGrantXP")) }
func AdminGrantCase(c *gin.Context)   { c.JSON(http.StatusNotImplemented, todo("AdminGrantCase")) }
func AdminBanGuest(c *gin.Context)    { c.JSON(http.StatusNotImplemented, todo("AdminBanGuest")) }
func AdminGetComputers(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("AdminGetComputers")) }
func AdminStartSession(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("AdminStartSession")) }
func AdminEndSession(c *gin.Context)  { c.JSON(http.StatusNotImplemented, todo("AdminEndSession")) }
func AdminLockPC(c *gin.Context)      { c.JSON(http.StatusNotImplemented, todo("AdminLockPC")) }
func AdminUnlockPC(c *gin.Context)    { c.JSON(http.StatusNotImplemented, todo("AdminUnlockPC")) }
func AdminGetBookings(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("AdminGetBookings")) }
func AdminUpdateBooking(c *gin.Context) { c.JSON(http.StatusNotImplemented, todo("AdminUpdateBooking")) }

func ShellWebSocket(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, todo("ShellWebSocket — реализует Go-разработчик"))
}

func todo(name string) gin.H {
	return gin.H{
		"status":  "not_implemented",
		"handler": name,
		"note":    "Этот эндпоинт реализуется командой",
	}
}
