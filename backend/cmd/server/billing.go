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

// minutesLeft — прогноз: минутный запас монет + деньги кошелька.
func minutesLeft(coinMinutes, wallet, rateGrosz int64, alreadyMoney int) int {
	return int(coinMinutes) + minutesAffordable(wallet, rateGrosz, alreadyMoney)
}

// walletTickPayload — единое тело wallet_update для шелла.
func walletTickPayload(u *models.User, s *models.Session, left int) map[string]any {
	return map[string]any{
		"wallet_grosz":   u.WalletGrosz,
		"wallet_pln":     models.PLNFromGrosz(u.WalletGrosz),
		"coin_minutes":   u.CoinMinutes,
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
func settleSessionMinutes(s *models.Session, user *models.User, targetMinutes int) (hitZero bool, err error) {
	delta := targetMinutes - s.BilledMinutes
	if delta <= 0 {
		if user.ID == s.UserID { // уже загружен
			return false, nil
		}
		return false, db.First(user, "id = ?", s.UserID).Error
	}
	rateGrosz := models.GroszFromPLN(s.EffectiveRatePLN)

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(user, "id = ?", s.UserID).Error; err != nil {
			return err
		}
		useCoin := int64(delta)
		if useCoin > user.CoinMinutes {
			useCoin = user.CoinMinutes
		}
		payMin := delta - int(useCoin)
		charge := chargeDelta(rateGrosz, s.MoneyMinutes, payMin)
		if charge > user.WalletGrosz {
			payMin = minutesAffordable(user.WalletGrosz, rateGrosz, s.MoneyMinutes)
			charge = chargeDelta(rateGrosz, s.MoneyMinutes, payMin)
			hitZero = true
		}

		// Сессию правим первой и только пока она active: если её параллельно
		// завершили — не списываем ничего.
		upd := map[string]any{
			"billed_minutes":    s.BilledMinutes + int(useCoin) + payMin,
			"coin_minutes_used": s.CoinMinutesUsed + int(useCoin),
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
		s.BilledMinutes += int(useCoin) + payMin
		s.CoinMinutesUsed += int(useCoin)
		s.MoneyMinutes += payMin
		s.ChargedGrosz += charge
		user.CoinMinutes -= useCoin
		user.WalletGrosz -= charge
		return nil
	})
	return hitZero, err
}

// billSession — один тик одной сессии: доначислить прошедшие целые минуты,
// разослать прогноз/предупреждения, обработать ноль и грейс.
func billSession(s *models.Session, now time.Time) bool {
	rateGrosz := models.GroszFromPLN(s.EffectiveRatePLN)
	grace := settingInt64("zero_grace_min", zeroGraceMinDef)

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

	elapsed := int(now.Sub(s.StartedAt).Minutes())
	if elapsed < 0 {
		elapsed = 0
	}

	var user models.User
	hitZero, err := settleSessionMinutes(s, &user, elapsed)
	if err != nil {
		return false
	}

	left := minutesLeft(user.CoinMinutes, user.WalletGrosz, rateGrosz, s.MoneyMinutes)
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
			billingSweep(time.Now())
			time.Sleep(billingTickSeconds * time.Second)
		}
	}()
}
