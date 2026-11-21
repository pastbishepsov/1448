package main

// Ранги аккаунта (престиж) — 7 ступеней по наигранным часам.
//
// Отличие от «уровня»: уровень (XP) растёт бесконечно по XP(n)=1000·n^1.4 и
// отражает суммарный прогресс. Ранг — пассивный статус за проведённое в клубе
// время: чем больше наиграно часов, тем выше множители XP/coins и шанс/тир
// бонусных кейсов. Это награда за лояльность (ключевая идея продукта).
//
// Бонусы ранга СТЕКАЮТСЯ с талантами:
//   XP     = базовый × (1 + xp_boost) × rank.XPMult
//   coins  = базовый × rank.CoinMult
//   шанс   = base + case_hunter + rank.CaseChanceBonus   (потолок maxSessionCaseChance)
//   тир    = luck_grade + rank.TierBoost                 (буст весов Heavy+)

import (
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

type AccountRank struct {
	Level           int
	Name            string
	MinHours        int
	XPMult          float64 // множитель опыта за сессию
	CoinMult        float64 // множитель coins за сессию
	CaseChanceBonus float64 // прибавка к шансу бонусного кейса за сессию
	TierBoost       float64 // буст тира кейса (как talent luck_grade)
}

// 7 рангов. Пороги ~×2, бонусы растут плавно.
// На максимуме: XP ×1.55, coins ×1.40, шанс кейса +25 п.п., тир Heavy+ ×1.55.
var accountRanks = []AccountRank{
	{1, "Новичок", 0, 1.00, 1.00, 0.00, 0.00},
	{2, "Завсегдатай", 10, 1.05, 1.05, 0.03, 0.05},
	{3, "Ветеран", 25, 1.10, 1.10, 0.06, 0.10},
	{4, "Мастер", 50, 1.18, 1.15, 0.10, 0.18},
	{5, "Элита", 100, 1.28, 1.22, 0.15, 0.28},
	{6, "Легенда", 200, 1.40, 1.30, 0.20, 0.40},
	{7, "Бессмертный", 400, 1.55, 1.40, 0.25, 0.55},
}

// К12-и4 (решение основателя 21.11): последний ранг — тайный. Пока гость его не взял, сервер не отдаёт его имя
// ни в next_rank, ни в списке ranks: там «***» и secret: true, часы и бонусы видны. Перевод имени живёт только
// здесь — в словарях киоска и PWA его нет; взявшему ранг сервер отдаёт names на трёх языках, клиент берёт свой.
const secretRankMask = "***"

var secretRankNames = map[string]string{"ru": "Бессмертный", "en": "Immortal", "pl": "Nieśmiertelny"}

func rankIsSecret(r AccountRank) bool { return r.Level == accountRanks[len(accountRanks)-1].Level }

// rankShown — дописывает в h имя ранга r так, как его видит гость на ранге curLevel.
func rankShown(r AccountRank, curLevel int, h gin.H) gin.H {
	switch {
	case !rankIsSecret(r):
		h["name"] = r.Name
	case curLevel >= r.Level:
		h["name"] = r.Name
		h["names"] = secretRankNames
	default:
		h["name"] = secretRankMask
		h["secret"] = true
	}
	return h
}

// accountRankFor — текущий ранг по наигранным часам и следующий (nil на максимуме).
// Чистая функция (тестируется).
func accountRankFor(hours int) (current AccountRank, next *AccountRank) {
	current = accountRanks[0]
	for i := range accountRanks {
		if hours >= accountRanks[i].MinHours {
			current = accountRanks[i]
			if i+1 < len(accountRanks) {
				n := accountRanks[i+1]
				next = &n
			} else {
				next = nil
			}
		}
	}
	return current, next
}

// sessionCaseChance — итоговый шанс бонусного кейса с талантом и рангом (с потолком).
func sessionCaseChance(caseHunter, rankBonus float64) float64 {
	c := baseSessionCaseChance + caseHunter + rankBonus
	if c > maxSessionCaseChance {
		c = maxSessionCaseChance
	}
	if c < 0 {
		c = 0
	}
	return c
}

// GET /me/economy — всё для калькулятора койнов: ставки, ранг, эффекты талантов.
func handleGetEconomy(c *gin.Context) {
	userID := c.GetString("user_id")
	hours := userHoursPlayed(userID)
	rank, next := accountRankFor(hours)

	// Г0-и3 (трек Г): кошелёк — рядом с остальной экономикой гостя.
	var me models.User
	_ = db.First(&me, "id = ?", userID).Error

	caseHunter := talentEffect(userID, "case_hunter")
	// З16-и1 (З-Р25): гость за ПК — ставки его сессии: калькулятор киоска и «≈XP» в PWA сходятся с тем, что начислит
	// finishSession, даже если владелец поменял ставки, пока гость играет. Без сессии — нынешние: по ним пойдёт следующая.
	rates := economyRates()
	var live models.Session
	if db.Select("xp_per_min", "coins_per_min").Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Limit(1).Find(&live).RowsAffected == 1 {
		rates = sessionRates(&live, rates)
	}
	resp := gin.H{
		"wallet_grosz": me.WalletGrosz,
		"wallet_pln":   models.PLNFromGrosz(me.WalletGrosz),
		// Г4: правила броней — клиенты показывают гейт и лимит честно
		"booking_min_level":   settingInt64("booking_min_level", bookingMinLevelDef),
		"max_active_bookings": settingInt64("max_active_bookings", maxActiveBookingsDef),
		// К4-и4: правила паузы и AFK — киоск и PWA прячут «Паузу» при 0
		"pause_limit_min": settingInt64("pause_limit_min", pauseLimitMinDef),
		"afk_stop_min":    settingInt64("afk_stop_min", afkStopMinDef),
		// С2-и1 (SENS.md, Р2): DPI мыши клуба — студия сенсы на клубном ПК
		// подставляет его и пересчитывает сохранённую сенсу; 0 = не задан
		"mouse_dpi":    settingInt64("mouse_dpi", mouseDPIDef),
		"mousepad_cm":  settingInt64("mousepad_cm", mousepadCmDef), // С4-и3: ширина коврика клуба — эталон для замера DPI свайпом; 0 = не задана
		"hours_played": hours,
		"rank": rankShown(rank, rank.Level, gin.H{
			"level": rank.Level, "min_hours": rank.MinHours,
			"xp_mult": rank.XPMult, "coin_mult": rank.CoinMult,
			"case_chance_bonus": rank.CaseChanceBonus, "tier_boost": rank.TierBoost,
			"max": next == nil,
		}),
		// К3-и1 (№15): ставки — из настроек клуба, а не константы: калькулятор
		// на киоске обязан сходиться с тем, что начислит finishSession (в сессии —
		// её снимок, З16-и1).
		"rates": gin.H{
			"coins_per_min": rates.CoinsPerMin, "xp_per_min": rates.XPPerMin,
			"deposit_coins_per_pln": rates.CoinsPerPLN,
		},
		"talents": gin.H{
			"xp_boost":        talentEffect(userID, "xp_boost"),
			"coin_mint":       talentEffect(userID, "coin_mint"),
			"cashback_master": talentEffect(userID, "cashback_master"),
			"case_hunter":     caseHunter,
			"luck_grade":      talentEffect(userID, "luck_grade"),
		},
		"session_case_chance": math.Round(sessionCaseChance(caseHunter, rank.CaseChanceBonus)*1000) / 1000,
	}
	if next != nil {
		resp["next_rank"] = rankShown(*next, rank.Level, gin.H{
			"level": next.Level, "min_hours": next.MinHours,
			"hours_left": next.MinHours - hours,
		})
	}
	// Все 7 рангов — чтобы игрок видел, что даёт каждый (карточка рангов в UI). Тайный — без имени, пока не взят (К12-и4).
	all := make([]gin.H, len(accountRanks))
	for i, r := range accountRanks {
		all[i] = rankShown(r, rank.Level, gin.H{
			"level": r.Level, "min_hours": r.MinHours,
			"xp_mult": r.XPMult, "coin_mult": r.CoinMult,
			"case_chance_bonus": r.CaseChanceBonus, "tier_boost": r.TierBoost,
		})
	}
	resp["ranks"] = all
	c.JSON(http.StatusOK, resp)
}
