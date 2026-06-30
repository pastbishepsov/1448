package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const leaderboardTopN = 50

// computeRank — место игрока: 1 + число тех, у кого опыт строго больше.
// Чистая функция (тест в leaderboard_test.go). Та же семантика, что и SQL-COUNT в хендлере.
func computeRank(scores []int64, myScore int64) int {
	rank := 1
	for _, s := range scores {
		if s > myScore {
			rank++
		}
	}
	return rank
}

type leaderRow struct {
	Rank     int       `json:"rank"`
	UserID   uuid.UUID `json:"user_id"`
	Nickname string    `json:"nickname"`
	Level    int       `json:"level"`
	XPTotal  int64     `json:"xp_total"`
	AvatarID int       `json:"avatar_id"`
	IsMe     bool      `json:"is_me"`
}

// GET /leaderboard — топ игроков по опыту + место текущего игрока.
func handleLeaderboard(c *gin.Context) {
	userID := c.GetString("user_id")

	var top []models.User
	db.Order("xp_total DESC, level DESC").Limit(leaderboardTopN).Find(&top)

	rows := make([]leaderRow, 0, len(top))
	for i, u := range top {
		rows = append(rows, leaderRow{
			Rank:     i + 1,
			UserID:   u.ID,
			Nickname: u.Nickname,
			Level:    u.Level,
			XPTotal:  u.XPTotal,
			AvatarID: u.AvatarID,
			IsMe:     u.ID.String() == userID,
		})
	}

	// Место текущего игрока (через COUNT, без загрузки всех).
	me := gin.H{"rank": 0, "level": 0, "xp_total": int64(0)}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err == nil {
		var higher int64
		db.Model(&models.User{}).Where("xp_total > ?", user.XPTotal).Count(&higher)
		me = gin.H{"rank": int(higher) + 1, "level": user.Level, "xp_total": user.XPTotal}
	}

	c.JSON(http.StatusOK, gin.H{"top": rows, "me": me})
}
