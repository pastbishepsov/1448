package main

// Ж5-и1 «Шторки знают друг о друге» (ревью 14.09, №18–19): стойка видит
// деньги гостя там, где он сидит. До этого спринта шторка ПК показывала ник и
// таймер, а «сколько у него осталось» админ узнавал, открыв карточку гостя и
// посчитав в голове тариф × кошелёк. Прогноз здесь — ТОТ ЖЕ minutesLeft, что
// биллинг каждые 30 с шлёт шеллу в wallet_update (billing.go): две шторки и
// экран гостя показывают одну цифру, а не три похожих.

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// sessionForecast — «на сколько хватит» по активной сессии.
type sessionForecast struct {
	RateGrosz   int64 // тариф сессии, гроши/час (EffectiveRatePLN — с талантами)
	ZoneMinutes int64 // зонные минуты (за монеты, компенсации), годные на ЭТОЙ машине (Ж2)
	PackMinutes int64 // минуты пакетов этой зоны (Е2)
	MinutesLeft int   // сколько ЕЩЁ минут закрыто сверх уже учтённых: запас + пакеты + кошелёк
	// Until — когда деньги кончатся при непрерывной игре: учтённые минуты плюс
	// MinutesLeft от начала БЕЗ пауз. nil — сессия ещё не тарифицируется (окно
	// [Готов!], ожидание шелла) или стоит на паузе: обещать время, которое не
	// идёт, значит врать.
	Until           *time.Time
	BookingDeadline *time.Time // чужая бронь отрежет раньше (Г3): start − booking_lock_min
}

// forecastSession — прогноз для активной сессии s гостя u за машиной зоны
// zoneID. Считает ровно как тик биллинга: зонные минуты этой зоны + пул без
// зоны + пакеты зоны + минуты, которые кошелёк ещё покрывает по тарифу сессии.
func forecastSession(s *models.Session, u *models.User, zoneID *uuid.UUID, now time.Time) sessionForecast {
	f := sessionForecast{
		RateGrosz:   models.GroszFromPLN(s.EffectiveRatePLN),
		ZoneMinutes: liveGrantMinutes(s.UserID, zoneID),
		PackMinutes: livePackMinutes(s.UserID, zoneID, now),
	}
	f.MinutesLeft = minutesLeft(u.CoinMinutes+f.ZoneMinutes, f.PackMinutes, u.WalletGrosz, f.RateGrosz, s.MoneyMinutes)
	if !sessionWaitingReady(s) && s.PausedAt == nil {
		f.Until = forecastUntil(s.StartedAt, pausedDuration(s, now), s.BilledMinutes, f.MinutesLeft, now)
	}
	if nb := nextForeignBooking(s.ComputerID, s.UserID, now); nb != nil {
		d := nb.StartTime.Add(-time.Duration(settingInt64("booking_lock_min", bookingLockMinDef)) * time.Minute)
		f.BookingDeadline = &d
	}
	return f
}

// forecastUntil — момент, когда кончатся деньги (чистая, тест): сессия
// оплачена до минуты billed+left от старта без пауз. Уже прошедшее не
// обещаем: если учтённого и оставшегося меньше, чем прошло (грейс нуля),
// ответ — «сейчас».
func forecastUntil(startedAt time.Time, pausedSec, billed, left int, now time.Time) *time.Time {
	paidUntil := startedAt.Add(time.Duration(pausedSec)*time.Second + time.Duration(billed+left)*time.Minute)
	if paidUntil.Before(now) {
		paidUntil = now
	}
	return &paidUntil
}

// forecastFields — поля прогноза для JSON шторки ПК и карточки гостя. Имена
// те же, что в wallet_update шелла (walletTickPayload), чтобы стойка и гость
// читали одно и то же.
func forecastFields(u *models.User, f sessionForecast) gin.H {
	out := gin.H{
		"wallet_grosz": u.WalletGrosz,
		"wallet_pln":   models.PLNFromGrosz(u.WalletGrosz),
		"coin_minutes": u.CoinMinutes, // пул без зоны — годен и здесь
		"zone_minutes": f.ZoneMinutes,
		"pack_minutes": f.PackMinutes,
		"minutes_left": f.MinutesLeft,
	}
	if f.Until != nil {
		out["until"] = f.Until
	}
	if f.BookingDeadline != nil {
		out["booking_deadline"] = f.BookingDeadline
	}
	return out
}
