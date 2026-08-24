package main

// Достижения: движок условий и наград (спринт 4 + трек Г/Г5).
//
// Г5 оживил схему, лежавшую в БД с миграции 005: категории daily/weekly/
// monthly с period_key. Периодическая ачивка выдаётся один раз ЗА ПЕРИОД
// (ключ — periodKeyFor, фирменный резет 14:48, periods.go) и снова в
// следующем; lifetime — один раз навсегда, как раньше. Топливо условий —
// user_progress (progress.go): минуты/активные минуты/визиты по ачивочным
// суткам, стрик визитов. Награды прежним путём: очки навыков + кейс
// (CaseSourceAchievement).

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// playerStats — статистика игрока для проверки условий достижений.
type playerStats struct {
	// lifetime
	HoursPlayed   int
	LoginCount    int
	DepositCount  int
	BookingsCount int
	// периоды (Г5): ачивочные сутки/неделя/месяц от 14:48
	MinutesToday int
	ActiveToday  int
	VisitsToday  int
	KitchenToday int
	NightToday   int
	MinutesWeek  int
	ActiveWeek   int
	VisitsWeek   int
	VisitStreak  int
	VisitsMonth  int
	MinutesMonth int
	ActiveMonth  int
	// анкета (Г8): бинарные факты профиля, profileStats() в profile.go
	ProfileBirth    int
	ProfileGames    int
	ProfileDiscord  int
	ProfileTelegram int
	ProfileSource   int
	ProfileComplete int
}

// achCondition — разобранное условие ({"min":10} / {"count":1} / {"days":7}).
type achCondition struct {
	Min   *int `json:"min"`
	Count *int `json:"count"`
	Days  *int `json:"days"`
}

func (c achCondition) threshold() int {
	switch {
	case c.Min != nil:
		return *c.Min
	case c.Count != nil:
		return *c.Count
	case c.Days != nil:
		return *c.Days
	}
	return 1
}

// conditionValue — текущее значение игрока для типа условия; ok=false — тип
// ещё не поддержан механикой (win_streak и будущие).
func conditionValue(condType string, s playerStats) (int, bool) {
	switch condType {
	case "hours_played":
		return s.HoursPlayed, true
	case "login_count":
		return s.LoginCount, true
	case "deposit_count":
		return s.DepositCount, true
	case "bookings_count":
		return s.BookingsCount, true
	case "daily_visit":
		return s.VisitsToday, true
	case "visit_streak":
		return s.VisitStreak, true
	case "minutes_today":
		return s.MinutesToday, true
	case "active_minutes_today":
		return s.ActiveToday, true
	case "minutes_week":
		return s.MinutesWeek, true
	case "active_minutes_week":
		return s.ActiveWeek, true
	case "visits_week":
		return s.VisitsWeek, true
	case "visits_month":
		return s.VisitsMonth, true
	case "minutes_month":
		return s.MinutesMonth, true
	case "active_minutes_month":
		return s.ActiveMonth, true
	case "kitchen_today":
		return s.KitchenToday, true
	case "night_session_today":
		return s.NightToday, true
	case "profile_birth":
		return s.ProfileBirth, true
	case "profile_games":
		return s.ProfileGames, true
	case "profile_discord":
		return s.ProfileDiscord, true
	case "profile_telegram":
		return s.ProfileTelegram, true
	case "profile_source":
		return s.ProfileSource, true
	case "profile_complete":
		return s.ProfileComplete, true
	}
	return 0, false
}

// conditionMet — выполнено ли условие достижения (чистая функция, тесты в
// achievements_test.go).
func conditionMet(condType, condValueJSON string, s playerStats) bool {
	var cv achCondition
	_ = json.Unmarshal([]byte(condValueJSON), &cv)
	v, ok := conditionValue(condType, s)
	return ok && v >= cv.threshold()
}

// userHoursPlayed — суммарные часы игрока по завершённым сессиям.
func userHoursPlayed(userID string) int {
	var totalMin int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", userID, models.SessionStatusCompleted).
		Select("COALESCE(SUM(minutes_used), 0)").
		Scan(&totalMin)
	return int(totalMin / 60)
}

// userBookingsCount — сколько броней гость создавал за всё время (Г6: «первая бронь»).
func userBookingsCount(userID string) int {
	var n int64
	db.Model(&models.Booking{}).Where("user_id = ?", userID).Count(&n)
	return int(n)
}

// checkAchievements — выдать заработанные достижения и награды (best-effort).
// Lifetime — один раз навсегда; периодические (Г5) — один раз за период
// periodKeyFor (фирменный резет 14:48), в новом периоде выдаются заново.
func checkAchievements(userID uuid.UUID, stats playerStats) {
	now := time.Now()

	var earned []models.UserAchievement
	db.Where("user_id = ?", userID).Find(&earned)
	have := map[string]bool{}
	for _, e := range earned {
		pk := ""
		if e.PeriodKey != nil {
			pk = *e.PeriodKey
		}
		have[e.AchievementID+"|"+pk] = true
		have[e.AchievementID+"|"] = have[e.AchievementID+"|"] || pk == "" // lifetime-метка
	}

	var defs []models.Achievement
	db.Where("is_active = ?", true).Find(&defs)

	for _, a := range defs {
		pk := periodKeyFor(a.Category, now)
		key := a.ID + "|"
		if pk != nil {
			key = a.ID + "|" + *pk
		}
		if have[key] {
			continue
		}
		if !conditionMet(a.ConditionType, a.ConditionValue, stats) {
			continue
		}
		tier := a.RewardCaseTier
		sp := a.RewardSkillpoints
		xp := a.RewardXP
		_ = db.Transaction(func(tx *gorm.DB) error {
			ua := models.UserAchievement{UserID: userID, AchievementID: a.ID, EarnedAt: now, PeriodKey: pk}
			if err := tx.Create(&ua).Error; err != nil {
				return err
			}
			if sp > 0 {
				if err := tx.Model(&models.User{}).Where("id = ?", userID).
					Update("skillpoints_available", gorm.Expr("skillpoints_available + ?", sp)).Error; err != nil {
					return err
				}
			}
			if xp > 0 { // Г6: XP-награда через общий applyXP (левел-апы честные)
				if err := awardAchievementXP(tx, userID, int64(xp)); err != nil {
					return err
				}
			}
			if tier != nil {
				if err := grantCase(tx, userID, nil, *tier, models.CaseSourceAchievement); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

// awardAchievementXP — XP-награда достижения (Г6, Р5): тот же applyXP, что и
// у сессий/грантов — с левел-апами, очками за уровень и кейсами за уровень.
func awardAchievementXP(tx *gorm.DB, userID uuid.UUID, amount int64) error {
	var user models.User
	if err := tx.First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	levels := applyXP(&user, amount)
	if err := tx.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]any{
			"level":                 user.Level,
			"xp_current":            user.XPCurrent,
			"xp_total":              user.XPTotal,
			"skillpoints_available": user.SkillpointsAvailable,
		}).Error; err != nil {
		return err
	}
	for i := 0; i < levels; i++ {
		if err := grantCase(tx, user.ID, nil, tierForLevel(user.Level), models.CaseSourceLevelUp); err != nil {
			return err
		}
	}
	return nil
}

// GET /me/achievements — достижения с прогрессом и временем до резета (Г5).
func handleGetMyAchievements(c *gin.Context) {
	userID := c.GetString("user_id")
	uid, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	now := time.Now()

	stats := gatherPeriodicStats(uid, now)
	stats.HoursPlayed = userHoursPlayed(userID)
	stats.LoginCount = 1
	stats.DepositCount = userDepositCount(userID)
	stats.BookingsCount = userBookingsCount(userID)

	var me models.User
	db.First(&me, "id = ?", uid) // Г6-и4: запас заморозок стрика — рядом со стриком

	var defs []models.Achievement
	db.Where("is_active = ?", true).Order("category").Find(&defs)

	var earned []models.UserAchievement
	db.Where("user_id = ?", userID).Find(&earned)
	earnedAt := map[string]time.Time{} // ключ id|period
	for _, e := range earned {
		pk := ""
		if e.PeriodKey != nil {
			pk = *e.PeriodKey
		}
		earnedAt[e.AchievementID+"|"+pk] = e.EarnedAt
	}

	out := make([]gin.H, 0, len(defs))
	for _, a := range defs {
		pk := periodKeyFor(a.Category, now)
		key := a.ID + "|"
		if pk != nil {
			key = a.ID + "|" + *pk
		}
		item := gin.H{
			"id":                 a.ID,
			"title":              a.Title,
			"description":        a.Description,
			"category":           a.Category,
			"reward_skillpoints": a.RewardSkillpoints,
			"reward_case_tier":   a.RewardCaseTier,
			"reward_xp":          a.RewardXP,
			"earned":             false, // для периодических — в ТЕКУЩЕМ периоде
		}
		if pk != nil {
			item["period_key"] = *pk
		}
		if t, ok := earnedAt[key]; ok {
			item["earned"] = true
			item["earned_at"] = t
		}
		// прогресс к цели — если механика уже считает этот тип
		var cv achCondition
		_ = json.Unmarshal([]byte(a.ConditionValue), &cv)
		if v, ok := conditionValue(a.ConditionType, stats); ok {
			target := cv.threshold()
			if v > target {
				v = target
			}
			item["progress"] = gin.H{"value": v, "target": target}
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"hours_played": stats.HoursPlayed,
		"earned_count": len(earned),
		"achievements": out,
		// Г5: фирменный резет — все периоды перещёлкиваются в 14:48 клуба
		"resets_at":   nextAchReset(now),
		"day_key":     achDayKey(now),
		// Г6-и4: стрик и заморозки — клиенты рисуют карточку рядом с ачивками
		"streak":      gin.H{"days": stats.VisitStreak, "freeze": streakInfo(&me)},
		"period_keys": gin.H{"daily": periodKeyFor("daily", now), "weekly": periodKeyFor("weekly", now), "monthly": periodKeyFor("monthly", now)},
	})
}
