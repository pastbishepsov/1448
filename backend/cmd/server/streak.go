package main

// Заморозка стрика за монеты (трек Г, спринт Г6-и4; миграция 042).
//
// Решение Р11 (GUEST.md): заморозка покупается ЗАРАНЕЕ и сама прикрывает
// пропущенный день. «Восстановить стрик задним числом за монеты» не делаем
// сознательно — это связка «потерял → доплати», от которой ушли в RESEARCH §4.
//
// Замороженный день цепочку не рвёт, но визитом НЕ считается: «Неделя без
// пропусков» по-прежнему требует семи реальных приходов. Заморозка бережёт
// накопленное, а не покупает награду.
//
// Латаем дыры в момент, когда гость вернулся (старт сессии): считаем разрыв
// между последним отмеченным днём и сегодняшними ачивочными сутками. Правило
// «всё или ничего» — если заморозок на весь разрыв не хватает или разрыв
// длиннее лимита подряд, не тратим ни одной: сгореть впустую они не должны.

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	streakFreezeCostDef   = 150 // монет за штуку (~1,5 часа игры; 20 монет = 1 zł по spend-курсу)
	streakFreezeMaxDef    = 3   // сколько держать в запасе
	streakFreezeMaxRowDef = 2   // сколько дней подряд можно прикрыть
)

func streakFreezeCost() int64   { return settingInt64("streak_freeze_cost", streakFreezeCostDef) }
func streakFreezeMax() int64    { return settingInt64("streak_freeze_max", streakFreezeMaxDef) }
func streakFreezeMaxRow() int64 { return settingInt64("streak_freeze_max_row", streakFreezeMaxRowDef) }

// lastProgressDay — последний день (ачивочные сутки), который держит цепочку:
// был визит или он прикрыт заморозкой. Пустая строка — цепочки ещё нет.
func lastProgressDay(userID uuid.UUID, before time.Time) (time.Time, bool) {
	var row struct{ DayKey string }
	db.Model(&models.UserProgress{}).
		Select("day_key").
		Where("user_id = ? AND day_key < ? AND (sessions > 0 OR frozen)", userID, before.Format("2006-01-02")).
		Order("day_key DESC").Limit(1).Scan(&row)
	if row.DayKey == "" {
		return time.Time{}, false
	}
	d, err := time.ParseInLocation("2006-01-02", row.DayKey, clubLocation)
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

// freezesForGap — сколько заморозок потратить на разрыв (чистая, тест).
// Правило «всё или ничего»: если на весь разрыв не хватает или он длиннее
// лимита подряд, не тратим ни одной — сгореть впустую они не должны.
func freezesForGap(gap, stock, maxRow int) int {
	if gap <= 0 || maxRow <= 0 || stock <= 0 {
		return 0
	}
	if gap > maxRow || gap > stock {
		return 0
	}
	return gap
}

// applyStreakFreezes — прикрыть пропущенные дни запасом заморозок гостя.
// Возвращает, сколько штук потрачено (0 — ничего не делали).
func applyStreakFreezes(userID uuid.UUID, now time.Time) int {
	maxRow := int(streakFreezeMaxRow())
	if maxRow <= 0 || streakFreezeCost() <= 0 { // механика выключена владельцем
		return 0
	}
	var user models.User
	if db.First(&user, "id = ?", userID).Error != nil || user.StreakFreezes <= 0 {
		return 0
	}
	today := achDayStart(now)
	last, ok := lastProgressDay(userID, today)
	if !ok {
		return 0 // цепочки нет — беречь нечего
	}
	gap := freezesForGap(int(today.Sub(last).Hours()/24)-1, user.StreakFreezes, maxRow)
	if gap == 0 {
		return 0
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.User{}).
			Where("id = ? AND streak_freezes >= ?", userID, gap).
			UpdateColumn("streak_freezes", gorm.Expr("streak_freezes - ?", gap))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNoFreezes
		}
		for i := 1; i <= gap; i++ {
			day := last.AddDate(0, 0, i).Format("2006-01-02")
			if err := tx.Exec(`INSERT INTO user_progress (user_id, day_key, frozen)
				VALUES (?, ?, TRUE)
				ON CONFLICT (user_id, day_key) DO UPDATE SET frozen = TRUE`, userID, day).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0
	}
	notifyUser(userID, "streak_frozen", map[string]any{"days": gap, "left": user.StreakFreezes - gap})
	log.Printf("стрик: гостю %s заморозка прикрыла %d дн (осталось %d)", userID, gap, user.StreakFreezes-gap)
	return gap
}

var errNoFreezes = fmt.Errorf("no_freezes")

// streakInfo — состояние заморозок для клиентов (вкладка ачивок).
func streakInfo(u *models.User) gin.H {
	cost := streakFreezeCost()
	return gin.H{
		"freezes": u.StreakFreezes,
		"cost":    cost,
		"max":     streakFreezeMax(),
		"max_row": streakFreezeMaxRow(),
		"enabled": cost > 0,
	}
}

// POST /me/streak/freeze — купить одну заморозку за монеты.
func handleBuyStreakFreeze(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	cost, max := streakFreezeCost(), streakFreezeMax()
	if cost <= 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "freeze_off", "message": "Заморозки стрика сейчас выключены"})
		return
	}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "Пользователь не найден"})
		return
	}
	if max > 0 && int64(user.StreakFreezes) >= max {
		c.JSON(http.StatusConflict, gin.H{"code": "freeze_full", "message": fmt.Sprintf(
			"В запасе уже %d заморозки — больше не берём", user.StreakFreezes)})
		return
	}
	if user.CoinsBalance < cost {
		c.JSON(http.StatusConflict, gin.H{"code": "coins_low", "message": fmt.Sprintf(
			"Нужно %d монет, у тебя %d. Монеты капают за каждую минуту игры", cost, user.CoinsBalance)})
		return
	}

	var after int64
	err = db.Transaction(func(tx *gorm.DB) error {
		// CAS по балансу: две вкладки не купят на одни и те же монеты
		res := tx.Model(&models.User{}).
			Where("id = ? AND coins_balance >= ?", userID, cost).
			Updates(map[string]any{
				"coins_balance":  gorm.Expr("coins_balance - ?", cost),
				"streak_freezes": gorm.Expr("streak_freezes + 1"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errCoinsLow
		}
		var fresh models.User
		if err := tx.First(&fresh, "id = ?", userID).Error; err != nil {
			return err
		}
		after = fresh.CoinsBalance
		return tx.Create(&models.CoinSpend{
			UserID: userID, Kind: models.CoinSpendStreakFreeze, Coins: cost,
			BalanceAfter: after, Note: "заморозка стрика",
		}).Error
	})
	if err == errCoinsLow {
		c.JSON(http.StatusConflict, gin.H{"code": "coins_low", "message": "Монет не хватило — попробуй ещё раз"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "Не получилось, попробуй ещё раз"})
		return
	}
	db.First(&user, "id = ?", userID)
	c.JSON(http.StatusOK, gin.H{"streak": streakInfo(&user), "coins_balance": after})
}

var errCoinsLow = fmt.Errorf("coins_low")
