package main

// Сгорание монет у неактивных гостей (спринт В4, этап 3; миграция 029).
//
// Решение основателя 2026-08-18: три месяца аккаунт не трогаем вообще, после
// этого баланс тает на 10% в неделю; активность — СЕССИЯ ЗА ПК. Оба числа —
// настройки, крутятся из «Экономики».
//
// Почему таяние, а не обнуление: вернувшийся на четвёртом месяце застаёт почти
// весь баланс — это повод прийти, а не повод обидеться. Пропавший навсегда
// обнуляется сам примерно за полгода.
//
// Почему с предупреждением: за две недели до старта таяния гость получает
// уведомление. Пропавшему гостю нужен честный повод вернуться — и это он.

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	coinIdleDays     int64 = 90 // дней без сессии до первого сгорания
	coinBurnPctWeek  int64 = 10 // процент баланса в неделю
	coinBurnWarnDays int64 = 14 // за сколько дней предупредить
	burnEveryDays          = 7  // «10% в неделю» = не чаще раза в 7 дней
	burnTailCoins    int64 = 5  // остаток мельче — добиваем, чтобы не тянуть хвост
)

// burnAmount — сколько монет сгорит при балансе balance и проценте pct.
// Округляем ВВЕРХ и добиваем мелкий остаток: иначе хвост в пару монет висел
// бы вечно, а обязательство так и не закрылось. Чистая функция (тест).
func burnAmount(balance int64, pct int64) int64 {
	if balance <= 0 || pct <= 0 {
		return 0
	}
	if pct >= 100 {
		return balance
	}
	b := int64(math.Ceil(float64(balance) * float64(pct) / 100))
	if balance-b < burnTailCoins {
		return balance
	}
	return b
}

// shouldBurn — пора ли жечь. Чистая функция (тест).
// Три условия: таяние включено; с последней сессии прошло не меньше idleDays;
// с прошлого сгорания прошла неделя. Сгорание ДО последней сессии игнорируем —
// человек возвращался, и отсчёт пошёл заново.
func shouldBurn(lastSession time.Time, lastBurn *time.Time, now time.Time, idleDays int64, pct int64) bool {
	if idleDays <= 0 || pct <= 0 {
		return false
	}
	if now.Sub(lastSession) < time.Duration(idleDays)*24*time.Hour {
		return false
	}
	if lastBurn != nil && lastBurn.After(lastSession) &&
		now.Sub(*lastBurn) < burnEveryDays*24*time.Hour {
		return false
	}
	return true
}

// shouldWarn — пора ли предупредить о будущем таянии. Чистая функция (тест).
// Окно предупреждения — последние warnDays перед стартом; уже начавшим таять
// не пишем, им сообщать поздно.
func shouldWarn(lastSession time.Time, now time.Time, idleDays, warnDays int64) bool {
	if idleDays <= 0 || warnDays <= 0 {
		return false
	}
	idle := now.Sub(lastSession)
	start := time.Duration(idleDays) * 24 * time.Hour
	return idle >= start-time.Duration(warnDays)*24*time.Hour && idle < start
}

func burnSettings() (idleDays, pct, warnDays int64) {
	return settingInt64("coin_idle_days", coinIdleDays),
		settingInt64("coin_burn_pct_week", coinBurnPctWeek),
		settingInt64("coin_burn_warn_days", coinBurnWarnDays)
}

// burnRow — гость-кандидат: баланс, когда последний раз играл, что сгорит.
type burnRow struct {
	UserID   uuid.UUID
	Nickname string
	Balance  int64
	IdleDays int
	Coins    int64
}

// coinBurnScan — кто и сколько потеряет прямо сейчас. Общая механика для
// предпросмотра и для самого прогона: показанное владельцу и списанное
// считаются одним кодом, разойтись им негде.
func coinBurnScan(now time.Time) []burnRow {
	idleDays, pct, _ := burnSettings()
	if idleDays <= 0 || pct <= 0 {
		return nil
	}
	type row struct {
		ID          uuid.UUID
		Nickname    string
		Coins       int64
		LastSession *time.Time
		Registered  time.Time
		LastBurn    *time.Time
	}
	var rows []row
	db.Model(&models.User{}).
		Select(`users.id, users.nickname, users.coins_balance AS coins, users.registered_at AS registered,
		        (SELECT MAX(s.started_at) FROM sessions s WHERE s.user_id = users.id) AS last_session,
		        (SELECT MAX(b.created_at) FROM coin_burns b WHERE b.user_id = users.id) AS last_burn`).
		Where("users.role = ? AND users.coins_balance > 0", models.UserRolePlayer).
		Scan(&rows)

	out := make([]burnRow, 0, 8)
	for _, r := range rows {
		last := r.Registered // ни одной сессии — считаем от регистрации
		if r.LastSession != nil {
			last = *r.LastSession
		}
		if !shouldBurn(last, r.LastBurn, now, idleDays, pct) {
			continue
		}
		coins := burnAmount(r.Coins, pct)
		if coins <= 0 {
			continue
		}
		out = append(out, burnRow{UserID: r.ID, Nickname: r.Nickname, Balance: r.Coins,
			IdleDays: int(now.Sub(last).Hours() / 24), Coins: coins})
	}
	return out
}

// runCoinBurn — сжечь у всех, кому пора. Возвращает, у скольких и сколько.
func runCoinBurn(now time.Time, manual bool) (int, int64) {
	_, pct, _ := burnSettings()
	rows := coinBurnScan(now)
	total := int64(0)
	done := 0
	for _, r := range rows {
		err := db.Transaction(func(tx *gorm.DB) error {
			// условие в WHERE: если гость успел потратить монеты между
			// сканом и списанием, не уводим баланс в минус
			res := tx.Model(&models.User{}).Where("id = ? AND coins_balance >= ?", r.UserID, r.Coins).
				UpdateColumn("coins_balance", gorm.Expr("coins_balance - ?", r.Coins))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errBurnSkipped
			}
			return tx.Create(&models.CoinBurn{
				UserID: r.UserID, Coins: r.Coins, BalanceAfter: r.Balance - r.Coins,
				IdleDays: r.IdleDays, Pct: int(pct), Manual: manual,
			}).Error
		})
		if err != nil {
			continue
		}
		done++
		total += r.Coins
		notifyUser(r.UserID, "coins_burned", map[string]any{
			"coins": r.Coins, "left": r.Balance - r.Coins, "idle_days": r.IdleDays})
	}
	return done, total
}

var errBurnSkipped = fmt.Errorf("burn_skipped")

// warnBeforeBurn — предупредить тех, у кого таяние вот-вот начнётся.
// Пишем один раз за «заход»: повторное уведомление после той же сессии не шлём.
func warnBeforeBurn(now time.Time) int {
	idleDays, pct, warnDays := burnSettings()
	if idleDays <= 0 || pct <= 0 || warnDays <= 0 {
		return 0
	}
	type row struct {
		ID          uuid.UUID
		Coins       int64
		LastSession *time.Time
		Registered  time.Time
		LastWarn    *time.Time
	}
	var rows []row
	db.Model(&models.User{}).
		Select(`users.id, users.coins_balance AS coins, users.registered_at AS registered,
		        (SELECT MAX(s.started_at) FROM sessions s WHERE s.user_id = users.id) AS last_session,
		        (SELECT MAX(n.created_at) FROM notifications n
		          WHERE n.user_id = users.id AND n.type = 'coins_burn_soon') AS last_warn`).
		Where("users.role = ? AND users.coins_balance > 0", models.UserRolePlayer).
		Scan(&rows)

	sent := 0
	for _, r := range rows {
		last := r.Registered
		if r.LastSession != nil {
			last = *r.LastSession
		}
		if !shouldWarn(last, now, idleDays, warnDays) {
			continue
		}
		if r.LastWarn != nil && r.LastWarn.After(last) {
			continue // за этот «заход» уже предупреждали
		}
		startsIn := int(math.Ceil(time.Duration(idleDays*24*int64(time.Hour)).Hours()/24 - now.Sub(last).Hours()/24))
		notifyUser(r.ID, "coins_burn_soon", map[string]any{
			"coins": r.Coins, "days": startsIn, "pct": pct})
		sent++
	}
	return sent
}

// startCoinBurnJob — фоновый прогон. Первый через минуту после старта, дальше
// каждые 6 часов: правило «не чаще раза в 7 дней на гостя» делает повторные
// прогоны безопасными, поэтому перезапуск сервера ничего не ломает и не жжёт
// дважды.
func startCoinBurnJob() {
	go func() {
		time.Sleep(time.Minute)
		for {
			safely("coinBurn", func() {
				if n, coins := runCoinBurn(time.Now(), false); n > 0 {
					log.Printf("монеты: сгорело %d у %d гостей", coins, n)
				}
				if w := warnBeforeBurn(time.Now()); w > 0 {
					log.Printf("монеты: предупреждено о таянии — %d гостей", w)
				}
			})
			time.Sleep(6 * time.Hour)
		}
	}()
}

// GET /admin/coins/burn — предпросмотр: кто и сколько потеряет сейчас (owner).
func handleCoinBurnPreview(c *gin.Context) {
	idleDays, pct, warnDays := burnSettings()
	rows := coinBurnScan(time.Now())
	items := make([]gin.H, 0, len(rows))
	var total int64
	for _, r := range rows {
		total += r.Coins
		if len(items) < 100 {
			items = append(items, gin.H{"nickname": r.Nickname, "balance": r.Balance,
				"coins": r.Coins, "idle_days": r.IdleDays})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"count": len(rows), "coins": total, "value_pln": coinValuePLN(total, spendRate()),
		"items": items,
		"rules": gin.H{"idle_days": idleDays, "pct_week": pct, "warn_days": warnDays},
	})
}

// POST /admin/coins/burn — прогнать таяние сейчас, не дожидаясь фонового (owner).
func handleCoinBurnRun(c *gin.Context) {
	n, coins := runCoinBurn(time.Now(), true)
	// ручной прогон делает ровно то же, что фоновый, включая предупреждения:
	// иначе владелец нажал бы кнопку и не понял, почему письма не ушли
	warned := warnBeforeBurn(time.Now())
	if n > 0 {
		logAdminAction(c, "coin_burn", nil, fmt.Sprintf("сгорело %d монет у %d гостей", coins, n))
	}
	c.JSON(http.StatusOK, gin.H{"guests": n, "coins": coins, "warned": warned,
		"value_pln": coinValuePLN(coins, spendRate())})
}
