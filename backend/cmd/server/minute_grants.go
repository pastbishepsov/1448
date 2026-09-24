package main

// Ж2 (ADMIN.md, трек Ж, решение Ж-Р3): зонные минуты — minute_grants (миграция 063).
//
// Минуты, выданные за монеты, и минуты компенсации годны только в СВОЕЙ зоне —
// ровно как пакеты (Р10 трека Е). До Ж2 зона при выдаче означала только цену в
// монетах, а тратился общий счётчик users.coin_minutes на любой машине: 60
// минут «в Standard» отыгрывались в VIP, и клуб терял треть цены каждой выдачи
// (ревью 14.09, №3). Старый пул coin_minutes остаётся «без зоны» и
// дотрачивается как раньше — ничего не переносим и не сжигаем.
//
// Порядок расхода в биллинге (settleSessionMinutes): гранты этой зоны →
// coin_minutes без зоны → пакеты зоны → кошелёк. Гранты первыми — они годны
// только здесь; пул без зоны пригодится и на другой машине.

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// grantScope — условие «годен в этой зоне»: своя зона или без зоны; на
// машине вне зон годны только гранты без зоны.
func grantScope(q *gorm.DB, userID uuid.UUID, zoneID *uuid.UUID) *gorm.DB {
	q = q.Where("user_id = ? AND minutes_left > 0", userID)
	if zoneID == nil {
		return q.Where("zone_id IS NULL")
	}
	return q.Where("(zone_id = ? OR zone_id IS NULL)", *zoneID)
}

// liveGrantsFor — живые гранты гостя, годные в зоне: сперва зонные (годны
// только здесь), потом без зоны; внутри — раньше выданные первыми.
func liveGrantsFor(tx *gorm.DB, userID uuid.UUID, zoneID *uuid.UUID) []models.MinuteGrant {
	var grants []models.MinuteGrant
	grantScope(tx.Model(&models.MinuteGrant{}), userID, zoneID).
		Order("(zone_id IS NULL), created_at ASC").Find(&grants)
	return grants
}

// takeGrantMinutes — снять need минут с грантов по порядку; возвращает, сколько
// снять удалось. UPDATE под условием остатка (как takePackageMinutes):
// параллельная отмена выдачи или вторая сессия гостя не уведут остаток в минус.
func takeGrantMinutes(tx *gorm.DB, grants []models.MinuteGrant, need int) (int, error) {
	used := 0
	for i := range grants {
		if need <= 0 {
			break
		}
		take := min(grants[i].MinutesLeft, need)
		res := tx.Model(&models.MinuteGrant{}).
			Where("id = ? AND minutes_left >= ?", grants[i].ID, take).
			UpdateColumn("minutes_left", gorm.Expr("minutes_left - ?", take))
		if res.Error != nil {
			return used, res.Error
		}
		if res.RowsAffected == 0 {
			continue // остаток увели параллельно — идём дальше
		}
		used += take
		need -= take
	}
	return used, nil
}

// liveGrantMinutes — сколько зонных минут гость может потратить в этой зоне
// прямо сейчас: прогноз «сколько осталось» и порог старта.
func liveGrantMinutes(userID uuid.UUID, zoneID *uuid.UUID) int64 {
	var sum int64
	grantScope(db.Model(&models.MinuteGrant{}), userID, zoneID).
		Select("COALESCE(SUM(minutes_left),0)").Scan(&sum)
	return sum
}

// zoneMinutes — запас гостя по зонам для карточки и клиентов.
type zoneMinutes struct {
	ZoneID   *uuid.UUID `json:"zone_id"`
	Zone     string     `json:"zone"` // '' — без зоны
	Minutes  int64      `json:"minutes"`
	FromPool bool       `json:"from_pool,omitempty"` // старый пул users.coin_minutes
}

// grantMinutesByZone — живые гранты гостя, сгруппированные по зоне, плюс
// старый пул без зоны отдельной строкой (если он не пуст). Клиенты печатают
// это словами: «60 мин Standard · 18 мин без зоны».
func grantMinutesByZone(userID uuid.UUID, coinMinutes int64) []zoneMinutes {
	var rows []struct {
		ZoneID   *uuid.UUID
		ZoneName string
		Minutes  int64
	}
	db.Model(&models.MinuteGrant{}).
		Select("zone_id, MAX(zone_name) AS zone_name, COALESCE(SUM(minutes_left),0) AS minutes").
		Where("user_id = ? AND minutes_left > 0", userID).
		Group("zone_id").Order("MAX(zone_name)").Scan(&rows)
	out := make([]zoneMinutes, 0, len(rows)+1)
	for _, r := range rows {
		out = append(out, zoneMinutes{ZoneID: r.ZoneID, Zone: r.ZoneName, Minutes: r.Minutes})
	}
	if coinMinutes > 0 {
		out = append(out, zoneMinutes{Minutes: coinMinutes, FromPool: true})
	}
	return out
}

// splitMinuteSpend — чистая арифметика порядка расхода (тест): сколько минут
// покрыть грантами зоны, сколько пулом без зоны, сколько остаётся пакетам и
// кошельку.
func splitMinuteSpend(delta, grantAvail int, coinAvail int64) (useGrant, useCoin, rest int) {
	if delta <= 0 {
		return 0, 0, 0
	}
	useGrant = min(delta, max(grantAvail, 0))
	useCoin = min(delta-useGrant, int(max(coinAvail, 0)))
	return useGrant, useCoin, delta - useGrant - useCoin
}

// grantFor — новая строка гранта (без записи): зона и имя — снимком.
func grantFor(userID uuid.UUID, zone *models.Zone, minutes int, ratePLN float64, source string, refID *uuid.UUID, by *uuid.UUID) models.MinuteGrant {
	g := models.MinuteGrant{
		UserID: userID, MinutesTotal: minutes, MinutesLeft: minutes,
		Source: source, RefID: refID, CreatedBy: by,
		RatePLN: ratePLN, // З5-и2: «должны временем» — в злотых по этому тарифу
	}
	if zone != nil {
		zid := zone.ID
		g.ZoneID, g.ZoneName = &zid, zone.Name
	}
	return g
}

// grantsRemainingFor — сколько минут выдачи ещё не отыграно (по ref_id),
// строка под замком до конца транзакции: биллинг, снимающий минуты
// параллельно, подождёт и увидит уже обнулённый остаток. Для старых выдач
// без гранта — found=false: у них остаток лежал в общем пуле.
func grantsRemainingFor(tx *gorm.DB, refID uuid.UUID) (left int, found bool) {
	var g models.MinuteGrant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("ref_id = ?", refID).First(&g).Error; err != nil {
		return -1, false
	}
	return g.MinutesLeft, true
}

// zoneByID — зона по id (для выдачи и компенсации); nil — не найдена.
func zoneByID(id *uuid.UUID) *models.Zone {
	if id == nil {
		return nil
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", *id).Error; err != nil {
		return nil
	}
	return &z
}
