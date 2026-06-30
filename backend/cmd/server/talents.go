package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// canInvestTalent — чистая проверка правил инвестиции очка навыка.
// Вынесена отдельно ради тестируемости (см. talents_test.go).
func canInvestTalent(userLevel, skillpoints, currentLevel, maxLevel, minUserLevel int) (ok bool, code string) {
	switch {
	case skillpoints < 1:
		return false, "no_skillpoints"
	case userLevel < minUserLevel:
		return false, "level_locked"
	case currentLevel >= maxLevel:
		return false, "maxed"
	default:
		return true, ""
	}
}

// GET /me/talents — дерево талантов с текущими уровнями и эффектами игрока.
func handleGetMyTalents(c *gin.Context) {
	userID := c.GetString("user_id")

	var defs []models.TalentDefinition
	db.Where("is_active = ?", true).Order("branch").Order("min_user_level").Find(&defs)

	var invested []models.SkillTalent
	db.Where("user_id = ?", userID).Find(&invested)
	level := map[string]int{}
	for _, s := range invested {
		level[s.TalentID] = s.CurrentLevel
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}

	out := make([]gin.H, 0, len(defs))
	for _, d := range defs {
		cur := level[d.ID]
		out = append(out, gin.H{
			"id":             d.ID,
			"branch":         d.Branch,
			"name":           d.Name,
			"description":    d.Description,
			"effect_type":    d.EffectType,
			"current_level":  cur,
			"max_level":      d.MaxLevel,
			"min_user_level": d.MinUserLevel,
			"unlocked":       user.Level >= d.MinUserLevel,
			"effect_now":     float64(cur) * d.EffectPerLevel,
			"effect_next":    float64(cur+1) * d.EffectPerLevel,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"skillpoints_available": user.SkillpointsAvailable,
		"talents":               out,
	})
}

type investRequest struct {
	TalentID string `json:"talent_id" binding:"required"`
}

// POST /me/talents/invest — вложить одно очко навыка в талант.
func handleInvestTalent(c *gin.Context) {
	uid, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var req investRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	var def models.TalentDefinition
	if err := db.First(&def, "id = ? AND is_active = ?", req.TalentID, true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "talent_not_found", "message": "Талант не найден"})
		return
	}

	var user models.User
	if err := db.First(&user, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}

	// Текущий вложенный уровень (может ещё не быть записи).
	var st models.SkillTalent
	exists := db.Where("user_id = ? AND talent_id = ?", uid, def.ID).First(&st).Error == nil
	if !exists {
		st = models.SkillTalent{
			UserID:       uid,
			Branch:       def.Branch,
			TalentID:     def.ID,
			CurrentLevel: 0,
			MaxLevel:     def.MaxLevel,
		}
	}

	if ok, code := canInvestTalent(user.Level, user.SkillpointsAvailable, st.CurrentLevel, def.MaxLevel, def.MinUserLevel); !ok {
		msg := map[string]string{
			"no_skillpoints": "Нет свободных очков навыков",
			"level_locked":   "Талант откроется на более высоком уровне",
			"maxed":          "Талант уже прокачан до максимума",
		}[code]
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
		return
	}

	st.CurrentLevel++
	user.SkillpointsAvailable--

	err = db.Transaction(func(tx *gorm.DB) error {
		if exists {
			if e := tx.Save(&st).Error; e != nil {
				return e
			}
		} else if e := tx.Create(&st).Error; e != nil {
			return e
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"talent_id":             def.ID,
		"current_level":         st.CurrentLevel,
		"max_level":             def.MaxLevel,
		"effect_now":            float64(st.CurrentLevel) * def.EffectPerLevel,
		"skillpoints_available": user.SkillpointsAvailable,
	})
}
