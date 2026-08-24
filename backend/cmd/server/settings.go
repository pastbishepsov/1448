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
	"report_hour":               {0, 23},   // Б10-и1: граница клубных суток для сводки смены
	"seg_new_days":              {1, 365},  // Б10-и2: сегмент «новые» — окно первого визита
	"seg_lost_days":             {2, 730},  // Б10-и2: сегмент «пропавшие» — дней без визита
	"coins_per_pln_spend":       {1, 1000}, // В4-2: монет за 1 zł ПРИ СПИСАНИИ (курс погашения)
	"coin_spend_max_min":        {0, 1440}, // В4-2: потолок минут за одну выдачу, 0 = без лимита
	"coin_idle_days":            {0, 3650}, // В4-3: дней без сессии до начала таяния, 0 = не жечь
	"coin_burn_pct_week":        {0, 100},  // В4-3: процент баланса в неделю, 0 = не жечь
	"coin_burn_warn_days":       {0, 365},  // В4-3: за сколько дней предупредить гостя
	"min_start_minutes":         {0, 1440}, // Г1-и3: порог старта сессии в минутах, 0 = пускать всех
	"zero_grace_min":            {0, 60},   // Г1-и2: грейс на нуле кошелька до автозавершения
	"pause_limit_min":           {0, 240},  // Г2-и1: лимит паузы на сессию, 0 = пауза выключена
	"afk_stop_min":              {0, 240},  // Г2-и2: порог простоя до AFK-реакции, 0 = выкл
}

func currentSettings() gin.H {
	return gin.H{
		"xp_per_min":                settingInt64("xp_per_min", xpPerMinute),
		"coins_per_min":             settingInt64("coins_per_min", coinsPerMinute),
		"coins_per_pln":             settingInt64("coins_per_pln", coinsPerPLN),
		"admin_day_xp_cap":          settingInt64("admin_day_xp_cap", 0),
		"admin_day_deposit_cap_pln": settingInt64("admin_day_deposit_cap_pln", 0),
		"report_hour":               settingInt64("report_hour", 8),
		"seg_new_days":              settingInt64("seg_new_days", 14),
		"seg_lost_days":             settingInt64("seg_lost_days", 21),
		"coins_per_pln_spend":       settingInt64("coins_per_pln_spend", coinsPerPLNSpend),
		"coin_spend_max_min":        settingInt64("coin_spend_max_min", coinSpendMaxMin),
		"coin_idle_days":            settingInt64("coin_idle_days", coinIdleDays),
		"coin_burn_pct_week":        settingInt64("coin_burn_pct_week", coinBurnPctWeek),
		"coin_burn_warn_days":       settingInt64("coin_burn_warn_days", coinBurnWarnDays),
		"min_start_minutes":         settingInt64("min_start_minutes", minStartMinutesDef),
		"zero_grace_min":            settingInt64("zero_grace_min", zeroGraceMinDef),
		"pause_limit_min":           settingInt64("pause_limit_min", pauseLimitMinDef),
		"afk_stop_min":              settingInt64("afk_stop_min", afkStopMinDef),
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
