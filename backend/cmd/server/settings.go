package main

// Настройки экономики клуба (спринт А5, ADMIN.md): хранятся в таблице
// settings (миграция 017), редактируются владельцем из админки. Движок
// читает через settingInt64 с дефолтом — пустая таблица ничего не ломает.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

// settingString — строковая настройка из БД или дефолт. PUT /admin/settings
// принимает только числа (settingBounds), поэтому строковые настройки живут в
// своих ручках рядом с тем, чем управляют: games_root — в /admin/catalog/config.
func settingString(key, def string) string {
	var s models.Setting
	if err := db.First(&s, "key = ?", key).Error; err != nil {
		return def
	}
	return s.Value
}

// settingSetString — сохранить строковую настройку (upsert по ключу).
func settingSetString(key, val string) error {
	var existing models.Setting
	if err := db.First(&existing, "key = ?", key).Error; err == nil {
		return db.Model(&existing).Update("value", val).Error
	}
	return db.Create(&models.Setting{Key: key, Value: val}).Error
}

// settingBounds — допустимые границы значений (валидация PUT).
// Лимиты admin (Б0-и4, решение №6 в ADMIN.md): 0 = без лимита.
var settingBounds = map[string][2]int64{
	"xp_per_min":                {1, 1000},
	"coins_per_min":             {0, 1000},
	"coins_per_pln":             {1, 100},
	"admin_day_xp_cap":          {0, 10000000},
	"admin_day_deposit_cap_pln": {0, 1000000},
	"admin_day_comp_cap_pln":    {0, 1000000}, // Ж0-и2 (Ж-Р2): дневной потолок компенсаций админа деньгами, 0 = без лимита
	"admin_day_comp_cap_min":    {0, 100000},  // Ж0-и2 (Ж-Р2): то же минутами, 0 = без лимита
	"admin_day_case_cap":        {0, 1000},    // З0-и4 (№9 «а»): кейсов в клубные сутки на админа, 0 = без лимита
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
	"booking_buffer_min":        {0, 240},  // Г3 (Р3): буфер до чужой брони при посадке
	"booking_lock_min":          {0, 120},  // Г3 (Р3): за столько минут ПК придержан под бронь
	"booking_min_level":         {0, 100},  // Г4-и1: бронь с этого уровня, 0 = всем
	"max_active_bookings":       {0, 20},   // Г4-и2: живых броней на гостя, 0 = без лимита
	"no_show_min":               {0, 120},  // Г4-и3: минут от начала брони до сгорания, 0 = выкл
	"streak_freeze_cost":        {0, 100000}, // Г6-и4: цена заморозки стрика в монетах, 0 = механика выкл
	"streak_freeze_max":         {0, 10},     // сколько заморозок держать в запасе
	"streak_freeze_max_row":     {1, 7},      // сколько дней подряд можно прикрыть
	"birthday_case_tier":        {0, 5},    // Г8-и4: ДР-кейс (0=выкл, 1=light … 5=gods)
	"ready_wait_min":            {0, 30},   // Е1: сколько ждём [Готов!], мин; 0 = окно выключено
	"mouse_dpi":                 {0, 32000}, // С2-и1 (SENS.md, Р2): DPI мыши клуба; 0 = не задан, сенсу не пересчитываем
	"mousepad_cm":               {0, 200},   // С4-и3 (SENS.md, №13): ширина коврика клуба, см — эталон для замера DPI свайпом; 0 = не задана
	"pack_warn_days":            {0, 30},   // Е2-и5: за сколько дней сказать, что пакет сгорает; 0 = молчать
}

// settingDefaults — значение каждой числовой настройки, пока у неё нет строки
// в settings. Ключи — те же, что в settingBounds (тест в settings_test.go):
// новая настройка — строка здесь, строка там и поле в «Экономике».
var settingDefaults = map[string]int64{
	"xp_per_min":                xpPerMinute,
	"coins_per_min":             coinsPerMinute,
	"coins_per_pln":             coinsPerPLN,
	"admin_day_xp_cap":          0,
	"admin_day_deposit_cap_pln": 0,
	"admin_day_comp_cap_pln":    0, // Ж0-и2
	"admin_day_comp_cap_min":    0, // Ж0-и2
	"admin_day_case_cap":        0, // З0-и4
	"report_hour":               8,
	"seg_new_days":              14,
	"seg_lost_days":             21,
	"coins_per_pln_spend":       coinsPerPLNSpend,
	"coin_spend_max_min":        coinSpendMaxMin,
	"coin_idle_days":            coinIdleDays,
	"coin_burn_pct_week":        coinBurnPctWeek,
	"coin_burn_warn_days":       coinBurnWarnDays,
	"min_start_minutes":         minStartMinutesDef,
	"pack_warn_days":            packWarnDaysDef, // Е2-и5
	"zero_grace_min":            zeroGraceMinDef,
	"pause_limit_min":           pauseLimitMinDef,
	"afk_stop_min":              afkStopMinDef,
	"booking_buffer_min":        bookingBufferMinDef,
	"booking_lock_min":          bookingLockMinDef,
	"booking_min_level":         bookingMinLevelDef,
	"max_active_bookings":       maxActiveBookingsDef,
	"no_show_min":               noShowMinDef,
	"streak_freeze_cost":        streakFreezeCostDef,
	"streak_freeze_max":         streakFreezeMaxDef,
	"streak_freeze_max_row":     streakFreezeMaxRowDef,
	"birthday_case_tier":        1,
	"ready_wait_min":            readyWaitMinDef,
	"mouse_dpi":                 mouseDPIDef,   // С2-и1
	"mousepad_cm":               mousepadCmDef, // С4-и3
}

// currentSettings — действующие значения: строка из settings или значение по
// умолчанию. Из неё же PUT берёт «было» (З7-и1).
func currentSettings() gin.H {
	out := gin.H{}
	for key, def := range settingDefaults {
		out[key] = settingInt64(key, def)
	}
	return out
}

// GET /admin/settings — текущая экономика (owner).
func handleAdminSettingsGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"settings": currentSettings()})
}

// PUT /admin/settings — обновить значения (owner). Изменения — в аудит.
//
// З7-и1 (№41; из №40): «было» — действующее значение из currentSettings(), в
// том числе значение по умолчанию у настройки без строки в settings. Раньше у
// такой настройки «было» не находилось, и «Сохранить» без правок писал её в
// журнал «изменённой» и без стрелки. Значение, равное действующему, не пишется
// ни в базу, ни в журнал. Все строки — одной транзакцией: сбой на середине не
// оставляет часть настроек сохранённой.
func handleAdminSettingsPut(c *gin.Context) {
	var req map[string]int64
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
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
	keys, changes := settingChanges(currentSettings(), req)
	if len(keys) > 0 {
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, key := range keys {
				row := models.Setting{Key: key, Value: strconv.FormatInt(req[key], 10)}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "key"}},
					DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
				}).Create(&row).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			respondDBError(c, err)
			return
		}
		logAdminAction(c, "settings_update", nil, strings.Join(changes, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"settings": currentSettings()})
}

// settingChanges — что из запроса действительно меняется: ключи по алфавиту и
// строки журнала «ключ: было→стало». Чистая функция (тест).
func settingChanges(was gin.H, req map[string]int64) (keys, changes []string) {
	all := make([]string, 0, len(req))
	for key := range req {
		all = append(all, key)
	}
	sort.Strings(all)
	for _, key := range all {
		old, _ := was[key].(int64)
		if old == req[key] {
			continue
		}
		keys = append(keys, key)
		changes = append(changes, fmt.Sprintf("%s: %d→%d", key, old, req[key]))
	}
	return keys, changes
}
