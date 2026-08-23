package main

// Монеты: конвертер, погашение временем и обязательства клуба
// (спринт В4, этап 2; миграция 028).
//
// До этого спринта монеты были дорогой в один конец: копились из трёх
// источников (депозит по курсу, игра поминутно, дропы кейсов) и не тратились
// нигде. Владелец спрашивал «сколько денег равно монетам» — ответа не было,
// потому что не было курса погашения.
//
// Решения основателя 2026-08-18: гасим ВРЕМЕНЕМ за ПК; курс руками не задаём —
// владелец ставит цену часа зоны, а сколько это в монетах, считает конвертер
// из настройки coins_per_pln_spend; баланс один. В живые деньги монеты не
// меняются (RESEARCH §4).

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	coinsPerPLNSpend int64 = 20 // монет за 1 zł при списании (дефолт)
	coinSpendMaxMin  int64 = 0  // потолок минут за выдачу: 0 = без лимита (решение основателя)
)

// ceilTo5 — округление вверх до пятёрки. Правило цифр проекта требует шага 5,
// а вверх — потому что на округлении списания клуб терять не должен.
func ceilTo5(v float64) int64 {
	return int64(math.Ceil(v/5)) * 5
}

// coinsForMinutes — сколько монет стоит N минут в зоне с ценой ratePLN за час
// при курсе погашения spendRate монет за злотый. Чистая функция (тест).
func coinsForMinutes(minutes int, ratePLN float64, spendRate int64) int64 {
	if minutes <= 0 || ratePLN <= 0 || spendRate <= 0 {
		return 0
	}
	return ceilTo5(ratePLN * float64(minutes) / 60 * float64(spendRate))
}

// minutesForCoins — сколько минут гость может взять на свои монеты (вниз, до
// целой минуты). Чистая функция (тест).
func minutesForCoins(coins int64, ratePLN float64, spendRate int64) int {
	if coins <= 0 || ratePLN <= 0 || spendRate <= 0 {
		return 0
	}
	return int(float64(coins) / (ratePLN * float64(spendRate) / 60))
}

// coinValuePLN — во сколько злотых обходится клубу N монет по курсу погашения.
// Чистая функция (тест): по ней считаются и обязательства, и «отдано временем».
func coinValuePLN(coins int64, spendRate int64) float64 {
	if spendRate <= 0 {
		return 0
	}
	return math.Round(float64(coins)/float64(spendRate)*100) / 100
}

func spendRate() int64 { return settingInt64("coins_per_pln_spend", coinsPerPLNSpend) }

// ── Погашение ─────────────────────────────────────────────────────────

type redeemRequest struct {
	ZoneID  string `json:"zone_id"`
	Minutes int    `json:"minutes"`
}

// POST /admin/users/:id/redeem — обменять монеты гостя на время (staff).
// Цель — только player и не сам себе: тот же серверный инвариант, что у
// депозитов и начислений (решение №4 трека Б).
func handleAdminRedeemCoins(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
		return
	}
	var req redeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны зона и минуты"})
		return
	}
	if req.Minutes <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_minutes", "message": "Минут должно быть больше нуля"})
		return
	}
	if cap := settingInt64("coin_spend_max_min", coinSpendMaxMin); cap > 0 && int64(req.Minutes) > cap {
		c.JSON(http.StatusConflict, gin.H{"code": "over_cap",
			"message": fmt.Sprintf("За раз можно выдать не больше %d минут", cap)})
		return
	}

	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"code": "no_club", "message": "Клуб не найден"})
		return
	}
	rate, zoneName := club.BaseRatePLN, ""
	var zoneID *uuid.UUID
	if req.ZoneID != "" {
		var z models.Zone
		if err := db.First(&z, "id = ?", req.ZoneID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
			return
		}
		rate, zoneName, zoneID = z.RatePLN, z.Name, &z.ID
	}

	sr := spendRate()
	coins := coinsForMinutes(req.Minutes, rate, sr)
	if coins <= 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "bad_rate", "message": "Курс погашения или цена зоны не заданы"})
		return
	}
	if user.CoinsBalance < coins {
		c.JSON(http.StatusConflict, gin.H{"code": "not_enough_coins",
			"message": fmt.Sprintf("Нужно %d монет, у гостя %d", coins, user.CoinsBalance)})
		return
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	rec := models.CoinRedemption{
		ClubID: club.ID, UserID: user.ID, ZoneID: zoneID, ZoneName: zoneName,
		Minutes: req.Minutes, Coins: coins, RatePLN: rate,
		ValuePLN:  math.Round(rate*float64(req.Minutes)/60*100) / 100,
		CreatedBy: &adminID,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		// списываем условием в WHERE: две кассы не уведут баланс в минус
		res := tx.Model(&models.User{}).Where("id = ? AND coins_balance >= ?", user.ID, coins).
			UpdateColumn("coins_balance", gorm.Expr("coins_balance - ?", coins))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNotEnoughCoins
		}
		// Г1-и5: выданные минуты — в минутный запас гостя; биллинг тратит его
		// РАНЬШЕ кошелька, так «монеты гасятся только временем» перестаёт быть
		// честным словом и становится учётом.
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).
			UpdateColumn("coin_minutes", gorm.Expr("coin_minutes + ?", rec.Minutes)).Error; err != nil {
			return err
		}
		return tx.Create(&rec).Error
	})
	if err == errNotEnoughCoins {
		c.JSON(http.StatusConflict, gin.H{"code": "not_enough_coins", "message": "Монет не хватило"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Г1: свежие минуты оживляют активную сессию — предупреждения и грейс
	// начинаются заново (как у депозита).
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", user.ID, models.SessionStatusActive).
		Updates(map[string]any{"warn15_at": nil, "warn5_at": nil, "zero_since": nil})

	db.First(user, "id = ?", user.ID)
	target := user.ID
	logAdminAction(c, "coin_redeem", &target, fmt.Sprintf("%d мин%s за %d монет (%.2f zł)",
		rec.Minutes, zoneSuffix(zoneName), rec.Coins, rec.ValuePLN))
	hub.AdminBroadcast("coin_redeem", map[string]any{
		"nickname": user.Nickname, "minutes": rec.Minutes, "coins": rec.Coins, "zone": zoneName})
	c.JSON(http.StatusOK, gin.H{
		"id": rec.ID, "minutes": rec.Minutes, "coins": rec.Coins, "value_pln": rec.ValuePLN,
		"zone": zoneName, "coins_balance": user.CoinsBalance, "coin_minutes": user.CoinMinutes,
	})
}

var errNotEnoughCoins = fmt.Errorf("not_enough_coins")

func zoneSuffix(name string) string {
	if name == "" {
		return " (клубный тариф)"
	}
	return " · " + name
}

// POST /admin/coin-redemptions/:id/void — отменить выдачу времени (staff).
// Правила ровно те же, что у отмены продажи товара: свою и в пределах текущих
// клубных суток отменяет админ, чужую и вчерашнюю — владелец. Иначе «отмена»
// становится тихим способом вернуть монеты в обход сданной смены.
func handleCoinRedeemVoid(c *gin.Context) {
	var rec models.CoinRedemption
	if err := db.First(&rec, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "redeem_not_found", "message": "Выдача не найдена"})
		return
	}
	if rec.VoidedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "already_void", "message": "Выдача уже отменена"})
		return
	}
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	createdBy := ""
	if rec.CreatedBy != nil {
		createdBy = rec.CreatedBy.String()
	}
	if ok, code := canVoidSale(c.GetString("user_role"), createdBy,
		c.GetString("user_id"), rec.CreatedAt, from, to); !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": goodErrors[code]})
		return
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.CoinRedemption{}).Where("id = ?", rec.ID).
			Updates(map[string]any{"voided_at": now, "voided_by": adminID}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.User{}).Where("id = ?", rec.UserID).
			UpdateColumn("coins_balance", gorm.Expr("coins_balance + ?", rec.Coins)).Error; err != nil {
			return err
		}
		// Г1-и5: забираем обратно и минутный запас — но не больше, чем у гостя
		// осталось: если часть выданного времени уже отсижена, вернуть можно
		// только неотсиженный хвост (отмена существует для свежих ошибок).
		return tx.Model(&models.User{}).Where("id = ?", rec.UserID).
			UpdateColumn("coin_minutes", gorm.Expr("GREATEST(coin_minutes - ?, 0)", rec.Minutes)).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := rec.UserID
	logAdminAction(c, "coin_redeem_void", &target,
		fmt.Sprintf("отмена: %d мин%s, %d монет вернулись", rec.Minutes, zoneSuffix(rec.ZoneName), rec.Coins))
	c.JSON(http.StatusOK, gin.H{"voided": rec.ID, "coins_returned": rec.Coins})
}

// ── Отчёт «Монеты»: эмиссия и обязательства ───────────────────────────

type coinsAgg struct {
	Deposits     int64   `json:"deposits"`  // выдано за пополнения
	Play         int64   `json:"play"`      // накапало за игру
	Cases        int64   `json:"cases"`     // выпало из кейсов
	Issued       int64   `json:"issued"`    // всего выдано
	Redeemed     int64   `json:"redeemed"`  // погашено временем
	Minutes      int64   `json:"minutes"`   // сколько минут отдано
	GivenPLN     float64 `json:"given_pln"` // во сколько это обошлось клубу
	Burned       int64   `json:"burned"`    // сгорело у неактивных (В4-3)
	BurnedGuests int64   `json:"burned_guests"`
}

func aggCoins(p period) coinsAgg {
	var a coinsAgg
	db.Model(&models.Deposit{}).Select("COALESCE(SUM(coins_granted + bonus_coins),0)").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Scan(&a.Deposits)
	db.Model(&models.Session{}).Select("COALESCE(SUM(coins_earned),0)").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Scan(&a.Play)
	db.Model(&models.Case{}).Select("COALESCE(SUM(drop_amount),0)").
		Where("drop_type = ? AND opened_at >= ? AND opened_at < ?", models.DropTypeCoins, p.From, p.To).
		Scan(&a.Cases)
	a.Issued = a.Deposits + a.Play + a.Cases

	var red struct {
		Coins   int64
		Minutes int64
		Pln     float64
	}
	db.Model(&models.CoinRedemption{}).
		Select("COALESCE(SUM(coins),0) AS coins, COALESCE(SUM(minutes),0) AS minutes, COALESCE(SUM(value_pln),0) AS pln").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).Scan(&red)
	a.Redeemed, a.Minutes, a.GivenPLN = red.Coins, red.Minutes, math.Round(red.Pln*100)/100

	// В4-3: сгорание у неактивных — вторая причина, по которой обязательства
	// уменьшаются. Без этой строки монеты «пропадали» бы в отчёте молча.
	var burn struct {
		Coins  int64
		Guests int64
	}
	db.Model(&models.CoinBurn{}).
		Select("COALESCE(SUM(coins),0) AS coins, COUNT(DISTINCT user_id) AS guests").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Scan(&burn)
	a.Burned, a.BurnedGuests = burn.Coins, burn.Guests
	return a
}

// GET /admin/reports/coins — эмиссия монет и обязательства клуба (owner).
func handleReportCoins(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	sr := spendRate()
	cur, old := aggCoins(p), aggCoins(prev)

	var onHand int64
	db.Model(&models.User{}).Select("COALESCE(SUM(coins_balance),0)").Scan(&onHand)

	// сколько монет капает за час игры и какая это доля цены часа — та самая
	// цифра, ради которой владелец и просил конвертер
	perMin := settingInt64("coins_per_min", coinsPerMinute)
	var zones []models.Zone
	db.Order("sort, name").Find(&zones)
	club, _ := defaultClub()
	clubRate := 0.0
	if club != nil {
		clubRate = club.BaseRatePLN
	}
	rows := make([]gin.H, 0, len(zones)+1)
	addZone := func(name string, rate float64) {
		hour := coinsForMinutes(60, rate, sr)
		back := 0.0
		if rate > 0 {
			back = math.Round(coinValuePLN(perMin*60, sr)/rate*1000) / 10
		}
		rows = append(rows, gin.H{"zone": name, "rate_pln": rate, "coins_per_hour": hour,
			"earned_per_hour": perMin * 60, "cashback_pct": back})
	}
	for i := range zones {
		addZone(zones[i].Name, zones[i].RatePLN)
	}
	if len(zones) == 0 {
		addZone("клубный тариф", clubRate)
	}

	var recent []models.CoinRedemption
	db.Where("created_at >= ? AND created_at < ?", p.From, p.To).
		Order("created_at DESC").Limit(50).Find(&recent)
	// отменённые показываем тоже — с пометкой: в истории они должны остаться
	ids := make([]string, 0, len(recent))
	for _, r := range recent {
		ids = append(ids, r.UserID.String())
	}
	nick := nicknamesByID(ids)
	items := make([]gin.H, 0, len(recent))
	for _, r := range recent {
		items = append(items, gin.H{"id": r.ID, "created_at": r.CreatedAt, "nickname": nick[r.UserID.String()],
			"zone": r.ZoneName, "minutes": r.Minutes, "coins": r.Coins, "value_pln": r.ValuePLN,
			"voided": r.VoidedAt != nil})
	}

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{
			"issued":    pctDelta(float64(cur.Issued), float64(old.Issued)),
			"redeemed":  pctDelta(float64(cur.Redeemed), float64(old.Redeemed)),
			"given_pln": pctDelta(cur.GivenPLN, old.GivenPLN),
			"burned":    pctDelta(float64(cur.Burned), float64(old.Burned)),
		},
		"on_hand": gin.H{
			"coins":     onHand,
			"value_pln": coinValuePLN(onHand, sr),
		},
		"rates":       gin.H{"spend": sr, "buy": settingInt64("coins_per_pln", coinsPerPLN), "per_min": perMin},
		"converter":   rows,
		"redemptions": items,
	})
}

// coinsPerHourFor — сколько монет стоит час в зоне (для списка зон в UI).
func coinsPerHourFor(rate float64) int64 {
	return coinsForMinutes(60, rate, spendRate())
}
