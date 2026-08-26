package main

// Онбординг персонала (спринт Е5, OPERATOR.md, этап VI; решение Р9).
//
// «Инструкция при первом входе» с подсвечиванием кнопок. Новый админ должен
// увидеть, ГДЕ у стойки что нажимается, а не читать PDF, которого никто не
// открывает: обучение внутри интерфейса, на настоящих кнопках.
//
// Флаг прохождения живёт на СЕРВЕРЕ. В клубе десяток машин, админка
// открывается то с одной, то с другой, и localStorage означал бы тур на
// каждой новой — вместо помощи вышло бы раздражение. Заодно владелец видит,
// кто обучение уже прошёл.
//
// Версия тура: после крупной правки интерфейса её поднимают в коде, и тур
// показывается заново тем, у кого записана старая. Сбрасывать флаги руками
// по всей таблице для этого не нужно.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// tourVersion — версия тура по интерфейсу. Поднимать, когда шаги перестали
// соответствовать кнопкам: иначе человек пройдёт экскурсию по тому, чего нет.
const tourVersion = 1

// needsTour — показывать ли тур. Чистая функция (тест).
// Гостю тур не нужен: он живёт в шелле и PWA, а это админка.
func needsTour(role models.UserRole, seenVersion int, seenAt *time.Time, current int) bool {
	if role != models.UserRoleAdmin && role != models.UserRoleOwner {
		return false
	}
	if seenAt == nil {
		return true
	}
	return seenVersion < current
}

// GET /admin/onboarding — нужен ли тур этому человеку и какой.
func handleOnboardingState(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.GetString("user_id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"needs_tour":   needsTour(user.Role, user.OnboardedVersion, user.OnboardedAt, tourVersion),
		"tour_version": tourVersion,
		"seen_version": user.OnboardedVersion,
		"onboarded_at": user.OnboardedAt,
		"first_time":   user.OnboardedAt == nil,
	})
}

type onboardingRequest struct {
	// skipped — человек закрыл тур, не досмотрев. Записываем так же, как
	// пройденный: навязывать экскурсию тому, кто её закрыл, — грубость;
	// повторить он может из F.A.Q. в любой момент (и2).
	Skipped bool `json:"skipped"`
}

// POST /admin/onboarding — отметить, что тур показан (пройден или пропущен).
func handleOnboardingDone(c *gin.Context) {
	var req onboardingRequest
	_ = c.ShouldBindJSON(&req) // тело необязательно: пустое = «прошёл»

	now := time.Now()
	if err := db.Model(&models.User{}).Where("id = ?", c.GetString("user_id")).
		Updates(map[string]any{"onboarded_at": now, "onboarded_version": tourVersion}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	what := "тур пройден"
	if req.Skipped {
		what = "тур пропущен"
	}
	logAdminAction(c, "onboarding", nil, what)
	c.JSON(http.StatusOK, gin.H{"onboarded_at": now, "tour_version": tourVersion})
}

// DELETE /admin/onboarding — пройти тур заново (кнопка в F.A.Q., и2).
// Своё обучение человек повторяет сам, без владельца: спрашивать разрешения
// на то, чтобы ещё раз посмотреть, где кнопка, — абсурд.
func handleOnboardingReset(c *gin.Context) {
	if err := db.Model(&models.User{}).Where("id = ?", c.GetString("user_id")).
		Updates(map[string]any{"onboarded_at": nil, "onboarded_version": 0}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"needs_tour": true, "tour_version": tourVersion})
}
