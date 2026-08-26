package main

// Биллинг кошелька: поминутное списание за активные сессии (трек Г, Г1;
// GUEST.md, решения Р2/Р8).
//
// Как это работает:
//   - фоновый прогон раз в billingTickSeconds по всем активным сессиям;
//   - за каждую ПРОШЕДШУЮ целую минуту сессия «доначисляется»: сперва из
//     минутного запаса монет (users.coin_minutes, В4-redeem — Г1-и5), потом
//     деньгами из кошелька по ставке сессии (цена зоны минус скидки — та же
//     effective_rate_pln, что и раньше);
//   - деньги уходят ТОЛЬКО через walletApply (kind=session_spend, ref=сессия) —
//     журнал остаётся полным, инвариант Г0 не нарушается;
//   - суммы считаются от общего итога (chargeDelta), а не по минуте: за 60
//     минут выходит ровно цена часа, как бы тики её ни резали, и рестарт
//     сервера ничего не теряет и не задваивает — вся память в БД
//     (billed_minutes / money_minutes / charged_grosz на сессии);
//   - прогноз «сколько минут осталось» = минутный запас + сколько минут ещё
//     покрывает кошелёк; на ~15 и ~5 минутах гостю уходит balance_low (шина
//     Б4 + wallet_update на шелл), по одному разу на сессию;
//   - кошелёк упёрся в ноль → balance_zero и грейс zero_grace_min (деф. 2
//     мин), затем штатное finishSession с ended_reason=balance. Минуты грейса
//     не тарифицируются, ПОКА гость не додепнул: депозит/redeem сбрасывают
//     zero_since (deposits.go / coins.go), и следующий тик доначисляет
//     накопившийся хвост уже из живых денег — справедливо и просто.
//
// now везде параметр — арифметика и пороги проверяются тестами без ожиданий.

import (
	"errors"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

const (
	billingTickSeconds       = 30
	minStartMinutesDef int64 = 15 // порог старта сессии (настройка min_start_minutes)
	zeroGraceMinDef    int64 = 2  // грейс на нуле до автозавершения (настройка zero_grace_min)
	warnMinutesFirst         = 15 // первое предупреждение: осталось ~15 минут
	warnMinutesLast          = 5  // второе: ~5 минут
)

var errSessionGone = errors.New("session_gone")

// errComputerBusy — ПК заняли параллельным запросом между проверкой статуса и
// транзакцией старта сессии (ревью 26.08).
var errComputerBusy = errors.New("computer_busy")

// costForMinutes — стоимость m минут по ставке rateGrosz за час, вверх до
// гроша: на округлении клуб не теряет, гость не переплачивает больше гроша.
func costForMinutes(rateGrosz int64, m int) int64 {
	if rateGrosz <= 0 || m <= 0 {
		return 0
	}
	return (rateGrosz*int64(m) + 59) / 60
}

// chargeDelta — сколько доначислить за add минут при already уже оплаченных.
// Разность общих сумм — без дрейфа округления между тиками.
func chargeDelta(rateGrosz int64, already, add int) int64 {
	if add <= 0 {
		return 0
	}
	return costForMinutes(rateGrosz, already+add) - costForMinutes(rateGrosz, already)
}

// minutesAffordable — сколько ЕЩЁ минут покрывает кошелёк wallet при already
// оплаченных минутах. Оценка сверху + точная проверка (ceil даёт ±1 минуту).
func minutesAffordable(wallet, rateGrosz int64, already int) int {
	if wallet <= 0 || rateGrosz <= 0 {
		return 0
	}
	m := int(wallet*60/rateGrosz) + 1
	for m > 0 && chargeDelta(rateGrosz, already, m) > wallet {
		m--
	}
	return m
}

// minutesLeft — прогноз: минутный запас монет + минуты пакета + деньги
// кошелька (Е2-и3). Пакет входит сюда обязательно: гость с пятичасовым
// пакетом и пустым кошельком должен видеть пять часов, а не ноль, — и по
// этому же прогнозу его пускает порог старта.
func minutesLeft(coinMinutes, packMinutes, wallet, rateGrosz int64, alreadyMoney int) int {
	return int(coinMinutes) + int(packMinutes) + minutesAffordable(wallet, rateGrosz, alreadyMoney)
}

// walletTickPayload — единое тело wallet_update для шелла.
func walletTickPayload(u *models.User, s *models.Session, left int) map[string]any {
	return map[string]any{
		"wallet_grosz":   u.WalletGrosz,
		"wallet_pln":     models.PLNFromGrosz(u.WalletGrosz),
		"coin_minutes":   u.CoinMinutes,
		"pack_minutes":   s.PackMinutesUsed, // Е2-и3: сколько этой сессии закрыл пакет
		"minutes_left":   left,
		"charged_grosz":  s.ChargedGrosz,
		"billed_minutes": s.BilledMinutes,
	}
}

// settleSessionMinutes — доначислить сессии минуты до targetMinutes: сперва
// минутным запасом, затем кошельком; если денег на весь хвост нет — списывает
// сколько покрывается и сообщает, что упёрлись в ноль. Вся правка — одной
// транзакцией, сессия под условием status=active (гонка с завершением).
// bestEffort=true (финальный расчёт при завершении) не считает ноль ошибкой.
func settleSessionMinutes(s *models.Session, user *models.User, targetMinutes int, now time.Time) (hitZero bool, err error) {
	delta := targetMinutes - s.BilledMinutes
	if delta <= 0 {
		if user.ID == s.UserID { // уже загружен
			return false, nil
		}
		return false, db.First(user, "id = ?", s.UserID).Error
	}
	rateGrosz := models.GroszFromPLN(s.EffectiveRatePLN)
	// Е2-и3: зона машины — по ней выбираются пакеты. Читаем до транзакции:
	// внутри она уже не изменится, а держать лишний запрос под блокировкой
	// пользователя незачем.
	zoneID := zoneOfComputer(s.ComputerID)

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(user, "id = ?", s.UserID).Error; err != nil {
			return err
		}
		useCoin := int64(delta)
		if useCoin > user.CoinMinutes {
			useCoin = user.CoinMinutes
		}
		// Е2-и3: после монет — минуты пакета этой зоны, ближайший к сгоранию
		// первым. Деньги трогаем только тем, что пакеты не закрыли.
		usePack := 0
		if rest := delta - int(useCoin); rest > 0 {
			var perr error
			usePack, perr = takePackageMinutes(tx, livePackagesFor(tx, s.UserID, zoneID, now), rest)
			if perr != nil {
				return perr
			}
		}
		payMin := delta - int(useCoin) - usePack
		charge := chargeDelta(rateGrosz, s.MoneyMinutes, payMin)
		if charge > user.WalletGrosz {
			payMin = minutesAffordable(user.WalletGrosz, rateGrosz, s.MoneyMinutes)
			charge = chargeDelta(rateGrosz, s.MoneyMinutes, payMin)
			hitZero = true
		}

		// Сессию правим первой и только пока она active: если её параллельно
		// завершили — не списываем ничего.
		upd := map[string]any{
			"billed_minutes":    s.BilledMinutes + int(useCoin) + usePack + payMin,
			"coin_minutes_used": s.CoinMinutesUsed + int(useCoin),
			"pack_minutes_used": s.PackMinutesUsed + usePack,
			"money_minutes":     s.MoneyMinutes + payMin,
			"charged_grosz":     s.ChargedGrosz + charge,
		}
		res := tx.Model(&models.Session{}).
			Where("id = ? AND status = ?", s.ID, models.SessionStatusActive).
			Updates(upd)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errSessionGone
		}
		if useCoin > 0 {
			if err := tx.Model(&models.User{}).Where("id = ?", s.UserID).
				UpdateColumn("coin_minutes", gorm.Expr("coin_minutes - ?", useCoin)).Error; err != nil {
				return err
			}
		}
		if charge > 0 {
			sid := s.ID
			if _, err := walletApply(tx, walletOp{
				UserID: s.UserID, Kind: models.WalletTxSessionSpend, Amount: -charge,
				RefType: "session", RefID: &sid,
			}); err != nil {
				return err
			}
		}
		// локальное состояние — для прогноза и уведомлений после транзакции
		s.BilledMinutes += int(useCoin) + usePack + payMin
		s.CoinMinutesUsed += int(useCoin)
		s.PackMinutesUsed += usePack
		s.MoneyMinutes += payMin
		s.ChargedGrosz += charge
		user.CoinMinutes -= useCoin
		user.WalletGrosz -= charge
		return nil
	})
	return hitZero, err
}

// billSession — один тик одной сессии: пауза/AFK (Г2), затем доначислить
// прошедшие БЕЗ пауз целые минуты, разослать прогноз/предупреждения,
// обработать ноль и грейс.
func billSession(s *models.Session, now time.Time) bool {
	// — Е1: окно [Готов!]. Пока гость не подтвердил, сессия не тарифицируется
	// вообще: ни денег, ни минутного запаса, ни предупреждений. По истечении
	// дедлайна подтверждаем САМИ (решение Р1) и уходим — списывать начнёт
	// следующий тик, ровно с момента дедлайна.
	//
	// Проверка стоит ПЕРВОЙ, до дедлайна чужой брони: во время ожидания
	// завершать сессию нечем — денег не брали, и finishSession записал бы
	// «сыгранную» сессию на пустом месте. Столкнуться они не могут: правило
	// посадки Г3 требует, чтобы в окно до чужой брони влез минимальный сеанс
	// (30 мин), а ожидание ограничено семью минутами.
	if sessionWaitingReady(s) {
		if now.Before(*s.ReadyDeadline) {
			return false
		}
		if err := confirmReady(s, *s.ReadyDeadline); err != nil {
			log.Printf("биллинг: авто-старт сессии %s не прошёл: %v", s.ID, err)
			return false
		}
		notifyUser(s.UserID, "ready_auto", map[string]any{"started_at": s.StartedAt})
		hub.AdminBroadcast("session", map[string]any{"kind": "ready_auto", "session_id": s.ID})
		return true
	}

	rateGrosz := models.GroszFromPLN(s.EffectiveRatePLN)
	grace := settingInt64("zero_grace_min", zeroGraceMinDef)
	pauseLimit := settingInt64("pause_limit_min", pauseLimitMinDef)
	afkStop := settingInt64("afk_stop_min", afkStopMinDef)
	idle, idleKnown := shellIdleSec(s.ComputerID.String())

	// — Г3-и2: чужая бронь впереди — жёсткий дедлайн (start − lock). Работает
	// поверх паузы и нуля: ПК обещан другому гостю, сессия его освобождает. —
	if nb := nextForeignBooking(s.ComputerID, s.UserID, now); nb != nil {
		deadline := nb.StartTime.Add(-time.Duration(settingInt64("booking_lock_min", bookingLockMinDef)) * time.Minute)
		if !now.Before(deadline) {
			notifyUser(s.UserID, "booking_deadline", map[string]any{"start_time": nb.StartTime})
			if _, err := finishSession(s, nil, "booking"); err == nil {
				log.Printf("биллинг: сессия %s завершена — ПК уходит под бронь", s.ID)
			}
			return true
		}
		leftMin := int(deadline.Sub(now).Minutes())
		if leftMin <= 5 && s.BkWarn5At == nil {
			notifyUser(s.UserID, "booking_soon", map[string]any{"minutes_left": leftMin, "start_time": nb.StartTime})
			db.Model(&models.Session{}).Where("id = ?", s.ID).
				Updates(map[string]any{"bkwarn5_at": now, "bkwarn15_at": now})
			t := now
			s.BkWarn5At, s.BkWarn15At = &t, &t
		} else if leftMin <= 15 && s.BkWarn15At == nil {
			notifyUser(s.UserID, "booking_soon", map[string]any{"minutes_left": leftMin, "start_time": nb.StartTime})
			db.Model(&models.Session{}).Where("id = ?", s.ID).Update("bkwarn15_at", now)
			t := now
			s.BkWarn15At = &t
		}
	}

	// — Пауза (Г2-и1): время и деньги стоят — до бюджета или возвращения —
	if s.PausedAt != nil {
		guestBack := idleKnown && idle <= afkBackIdleSec
		byAfk := s.PausedBy != nil && *s.PausedBy == "afk"
		switch {
		case byAfk && guestBack: // гость шевельнул мышью — afk-пауза снимается сама
			_ = resumePause(s, now)
		case pauseBudgetLeftSec(s, pauseLimit, now) <= 0:
			if byAfk { // AFK и паузный бюджет кончился — освобождаем ПК
				if _, err := finishSession(s, nil, "afk"); err == nil {
					log.Printf("биллинг: сессия %s завершена по AFK", s.ID)
				}
				return true
			}
			_ = resumePause(s, now) // ручная пауза упёрлась в лимит — время снова идёт
			notifyUser(s.UserID, "pause_over", map[string]any{"limit_min": pauseLimit})
		default:
			return true // пауза идёт: не списываем и не пугаем
		}
	}

	// — AFK-детект (Г2-и2): только с живым датчиком и включённой настройкой —
	if afkStop > 0 && idleKnown && s.PausedAt == nil {
		if idle >= int(afkStop)*60 {
			if s.AfkWarnedAt == nil {
				notifyUser(s.UserID, "afk_warn", map[string]any{"idle_min": idle / 60})
				db.Model(&models.Session{}).Where("id = ?", s.ID).Update("afk_warned_at", now)
				t := now
				s.AfkWarnedAt = &t
			}
			if pauseLimit > 0 && pauseBudgetLeftSec(s, pauseLimit, now) > 0 {
				if err := startPause(s, now, "afk"); err == nil {
					notifyUser(s.UserID, "afk_pause", map[string]any{"limit_min": pauseLimit})
				}
				return true
			}
			if _, err := finishSession(s, nil, "afk"); err == nil {
				log.Printf("биллинг: сессия %s завершена по AFK (пауза недоступна)", s.ID)
			}
			return true
		}
		if s.AfkWarnedAt != nil && idle <= afkBackIdleSec {
			// вернулся после предупреждения — следующий AFK предупредит заново
			db.Model(&models.Session{}).Where("id = ?", s.ID).Update("afk_warned_at", nil)
			s.AfkWarnedAt = nil
		}
	}

	// Кошелёк уже на нуле: минуты грейса не тарифицируем (додеп сбросит
	// zero_since — и хвост доначислится следующим тиком), ждём истечения.
	if s.ZeroSince != nil {
		if now.Sub(*s.ZeroSince) >= time.Duration(grace)*time.Minute {
			if _, err := finishSession(s, nil, "balance"); err == nil {
				log.Printf("биллинг: сессия %s завершена по нулю кошелька", s.ID)
			}
		}
		return true
	}

	// Минуты считаем БЕЗ пауз (Г2): пауза не тарифицируется.
	elapsed := effectiveElapsedMinutes(s, now)
	prevBilled := s.BilledMinutes

	var user models.User
	hitZero, err := settleSessionMinutes(s, &user, elapsed, now)
	if err != nil {
		return false
	}

	// Активные минуты (анти-фарм Г5): учтённые этим тиком минуты считаются
	// активными, если гость не в простое — или датчика нет вовсе.
	if billedDelta := s.BilledMinutes - prevBilled; billedDelta > 0 && (!idleKnown || idle < activeIdleSec) {
		db.Model(&models.Session{}).Where("id = ? AND status = ?", s.ID, models.SessionStatusActive).
			UpdateColumn("active_minutes", gorm.Expr("active_minutes + ?", billedDelta))
		s.ActiveMinutes += billedDelta
	}

	packLeft := livePackMinutes(s.UserID, zoneOfComputer(s.ComputerID), now) // Е2-и3
	left := minutesLeft(user.CoinMinutes, packLeft, user.WalletGrosz, rateGrosz, s.MoneyMinutes)
	notifyShell(s.ComputerID.String(), websocket.MsgWalletUpdate, walletTickPayload(&user, s, left))

	if hitZero {
		zs := now
		s.ZeroSince = &zs
		db.Model(&models.Session{}).
			Where("id = ? AND status = ?", s.ID, models.SessionStatusActive).
			Update("zero_since", now)
		notifyUser(s.UserID, "balance_zero", map[string]any{
			"grace_min":  grace,
			"wallet_pln": models.PLNFromGrosz(user.WalletGrosz),
		})
		if grace <= 0 {
			if _, err := finishSession(s, nil, "balance"); err == nil {
				log.Printf("биллинг: сессия %s завершена по нулю кошелька (без грейса)", s.ID)
			}
		}
		return true
	}

	// Предупреждения — по одному разу на сессию; «5 минут» гасит и первое.
	if left <= warnMinutesLast && s.Warn5At == nil {
		notifyUser(s.UserID, "balance_low", map[string]any{
			"minutes_left": left, "wallet_pln": models.PLNFromGrosz(user.WalletGrosz),
		})
		db.Model(&models.Session{}).Where("id = ?", s.ID).
			Updates(map[string]any{"warn5_at": now, "warn15_at": now})
		zs := now
		s.Warn5At, s.Warn15At = &zs, &zs
	} else if left <= warnMinutesFirst && s.Warn15At == nil {
		notifyUser(s.UserID, "balance_low", map[string]any{
			"minutes_left": left, "wallet_pln": models.PLNFromGrosz(user.WalletGrosz),
		})
		db.Model(&models.Session{}).Where("id = ?", s.ID).Update("warn15_at", now)
		zs := now
		s.Warn15At = &zs
	}
	return true
}

// billingSweep — один проход по всем активным сессиям (now — параметром).
func billingSweep(now time.Time) int {
	var sessions []models.Session
	if err := db.Where("status = ?", models.SessionStatusActive).Find(&sessions).Error; err != nil {
		return 0
	}
	touched := 0
	for i := range sessions {
		if billSession(&sessions[i], now) {
			touched++
		}
	}
	return touched
}

// startWalletBillingJob — фоновый биллинг. Вся память — в БД, поэтому
// перезапуск сервера безопасен: первый же прогон догоняет прошедшие минуты
// одним доначислением, без потерь и задвоений.
func startWalletBillingJob() {
	go func() {
		time.Sleep(15 * time.Second)
		for {
			// Каждый тик под recover: паника здесь не перехватывается
			// gin.Recovery и убивала процесс целиком (ревью 26.08).
			safely("billingSweep", func() { billingSweep(time.Now()) })
			safely("bookingSweep", func() { bookingSweep(time.Now()) }) // Г4: напоминания и no-show
			time.Sleep(billingTickSeconds * time.Second)
		}
	}()
}
