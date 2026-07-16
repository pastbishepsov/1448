package main

// Настройки экономики клуба (спринт А5, ADMIN.md): хранятся в таблице
// settings (миграция 017), редактируются владельцем из админки. Движок
// читает через settingInt64 с дефолтом — пустая таблица ничего не ломает.

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ownerMiddleware — пускает только владельца. Ставится ПОСЛЕ adminMiddleware.
func ownerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("user_role") != string(models.UserRoleOwner) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "owner_only", "message": "Только для владельца"})
			return
		}
		c.Next()
	}
}

// settingInt64 — значение настройки из БД или дефолт.
func settingInt64(key string, def int64) int64 {
	var s models.Setting
	if err := db.First(&s, "key = ?", key).Error; err != nil {
		return def
	}
	v, err := strconv.ParseInt(s.Value, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// settingBounds — допустимые границы значений (валидация PUT).
// Лимиты admin (Б0-и4, решение №6 в ADMIN.md): 0 = без лимита.
var settingBounds = map[string][2]int64{
	"xp_per_min":                {1, 1000},
	"coins_per_min":             {0, 1000},
	"coins_per_pln":             {1, 100},
	"admin_day_xp_cap":          {0, 10000000},
	"admin_day_deposit_cap_pln": {0, 1000000},
}

func currentSettings() gin.H {
	return gin.H{
		"xp_per_min":                settingInt64("xp_per_min", xpPerMinute),
		"coins_per_min":             settingInt64("coins_per_min", coinsPerMinute),
		"coins_per_pln":             settingInt64("coins_per_pln", coinsPerPLN),
		"admin_day_xp_cap":          settingInt64("admin_day_xp_cap", 0),
		"admin_day_deposit_cap_pln": settingInt64("admin_day_deposit_cap_pln", 0),
	}
}

// GET /admin/settings — текущая экономика (owner).
func handleAdminSettingsGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"settings": currentSettings()})
}

// PUT /admin/settings — обновить значения (owner). Изменения — в аудит.
func handleAdminSettingsPut(c *gin.Context) {
	var req map[string]int64
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	for key, val := range req {
		bounds, ok := settingBounds[key]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_key", "message": "Неизвестная настройка: " + key})
			return
		}
		if val < bounds[0] || val > bounds[1] {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_value",
				"message": fmt.Sprintf("%s: значение от %d до %d", key, bounds[0], bounds[1])})
			return
		}
	}
	changes := ""
	for key, val := range req {
		old := settingInt64(key, -1)
		if old == val {
			continue
		}
		strVal := strconv.FormatInt(val, 10)
		var existing models.Setting
		if err := db.First(&existing, "key = ?", key).Error; err == nil {
			if err := db.Model(&existing).Update("value", strVal).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
				return
			}
		} else if err := db.Create(&models.Setting{Key: key, Value: strVal}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
			return
		}
		if changes != "" {
			changes += ", "
		}
		if old >= 0 {
			changes += fmt.Sprintf("%s: %d→%d", key, old, val)
		} else {
			changes += fmt.Sprintf("%s: %d", key, val)
		}
	}
	if changes != "" {
		logAdminAction(c, "settings_update", nil, changes)
	}
	c.JSON(http.StatusOK, gin.H{"settings": currentSettings()})
}
