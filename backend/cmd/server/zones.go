package main

// Зоны зала со своей ценой часа (спринт В4, этап 5; миграция 027).
// До этого зона была подписью на компьютере, а тариф — один на весь клуб и
// нигде не редактировался: VIP стоил столько же, сколько обычное место.
//
// Источник правды — zone_id у компьютера; текстовая колонка zone остаётся
// кэшем имени (по ней уже строятся карта зала и разрезы отчётов) и пишется
// ТОЛЬКО отсюда, одним путём. Поэтому переименование зоны обновляет кэш у
// всех её машин одной операцией.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxZoneRate = 500.0 // zł/час: потолок от опечатки в три нуля

// validateZone — чистая проверка зоны (тест в zones_test.go).
func validateZone(name string, rate float64) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" || len([]rune(n)) > 32 {
		return false, "bad_name"
	}
	if rate <= 0 || rate > maxZoneRate {
		return false, "bad_rate"
	}
	return true, ""
}

var zoneErrors = map[string]string{
	"bad_name":       "Название зоны — 1–32 символа",
	"bad_rate":       fmt.Sprintf("Цена часа — от 0.01 до %.0f zł", maxZoneRate),
	"already_exists": "Зона с таким названием уже есть",
	"has_computers":  "В зоне есть компьютеры — сначала перенеси их в другую",
	// Е2-и1: пакеты привязаны к зоне (Р10), и у гостей на руках купленные
	// минуты именно этой зоны. Молча снести её нельзя.
	"has_packages": "На эту зону есть пакеты времени — сначала выключи или удали их",
}

func zoneFail(c *gin.Context, status int, code string) {
	msg := zoneErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

// rateForComputer — цена часа для ПК: тариф его зоны, а если зоны нет —
// клубный тариф. Чистая функция (тест): fallback важен, иначе удаление зоны
// обнулило бы цену сессии.
func rateForComputer(zoneRate *float64, clubRate float64) float64 {
	if zoneRate != nil && *zoneRate > 0 {
		return *zoneRate
	}
	return clubRate
}

// zoneRateOf — тариф зоны компьютера из базы (nil, если зоны нет).
func zoneRateOf(pc *models.Computer) *float64 {
	if pc.ZoneID == nil {
		return nil
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", *pc.ZoneID).Error; err != nil {
		return nil
	}
	return &z.RatePLN
}

// GET /admin/zones — зоны с ценами и числом машин (staff: цену видно у стойки).
func handleAdminZones(c *gin.Context) {
	var zones []models.Zone
	db.Order("sort, name").Find(&zones)
	var counts []struct {
		ZoneID string
		N      int64
	}
	db.Model(&models.Computer{}).Select("zone_id, COUNT(*) AS n").
		Where("zone_id IS NOT NULL").Group("zone_id").Scan(&counts)
	byZone := map[string]int64{}
	for _, r := range counts {
		byZone[r.ZoneID] = r.N
	}
	var loose int64
	db.Model(&models.Computer{}).Where("zone_id IS NULL").Count(&loose)

	out := make([]gin.H, 0, len(zones))
	for i := range zones {
		z := zones[i]
		out = append(out, gin.H{"id": z.ID, "name": z.Name, "rate_pln": z.RatePLN,
			"sort": z.Sort, "computers": byZone[z.ID.String()],
			"coins_per_hour": coinsPerHourFor(z.RatePLN)}) // В4-2: конвертер считает сам
	}
	club, _ := defaultClub()
	base := 0.0
	if club != nil {
		base = club.BaseRatePLN
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "zones": out,
		"club_rate_pln": base, "without_zone": loose})
}

type zoneRequest struct {
	Name    string   `json:"name"`
	RatePLN *float64 `json:"rate_pln"`
	Sort    *int     `json:"sort"`
}

// POST /admin/zones — новая зона (owner).
func handleAdminZoneCreate(c *gin.Context) {
	var req zoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	rate := 0.0
	if req.RatePLN != nil {
		rate = *req.RatePLN
	}
	if ok, code := validateZone(req.Name, rate); !ok {
		zoneFail(c, http.StatusBadRequest, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		zoneFail(c, http.StatusConflict, "no_club")
		return
	}
	z := models.Zone{ClubID: club.ID, Name: strings.TrimSpace(req.Name), RatePLN: rate}
	if req.Sort != nil {
		z.Sort = *req.Sort
	}
	if err := db.Create(&z).Error; err != nil {
		if isDuplicate(err) {
			zoneFail(c, http.StatusConflict, "already_exists")
			return
		}
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "zone_create", nil, fmt.Sprintf("%s · %.2f zł/ч", z.Name, z.RatePLN))
	c.JSON(http.StatusCreated, gin.H{"zone": gin.H{"id": z.ID, "name": z.Name, "rate_pln": z.RatePLN, "sort": z.Sort}})
}

// PATCH /admin/zones/:id — имя, цена, порядок (owner).
// Переименование тянет за собой кэш имени у всех машин зоны.
func handleAdminZoneUpdate(c *gin.Context) {
	var z models.Zone
	if err := db.First(&z, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		return
	}
	var req zoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name, rate := z.Name, z.RatePLN
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
	}
	if req.RatePLN != nil {
		rate = *req.RatePLN
	}
	if ok, code := validateZone(name, rate); !ok {
		zoneFail(c, http.StatusBadRequest, code)
		return
	}
	changes := []string{}
	if name != z.Name {
		changes = append(changes, z.Name+" → "+name)
	}
	if rate != z.RatePLN {
		changes = append(changes, fmt.Sprintf("цена %.2f → %.2f zł/ч", z.RatePLN, rate))
	}
	updates := map[string]any{"name": name, "rate_pln": rate}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if err := db.Model(&models.Zone{}).Where("id = ?", z.ID).Updates(updates).Error; err != nil {
		if isDuplicate(err) {
			zoneFail(c, http.StatusConflict, "already_exists")
			return
		}
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if name != z.Name { // кэш имени у машин зоны
		db.Model(&models.Computer{}).Where("zone_id = ?", z.ID).Update("zone", name)
	}
	if len(changes) > 0 {
		logAdminAction(c, "zone_update", nil, strings.Join(changes, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"zone": gin.H{"id": z.ID, "name": name, "rate_pln": rate}})
}

// DELETE /admin/zones/:id — удалить пустую зону (owner).
func handleAdminZoneDelete(c *gin.Context) {
	var z models.Zone
	if err := db.First(&z, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		return
	}
	var n int64
	db.Model(&models.Computer{}).Where("zone_id = ?", z.ID).Count(&n)
	if n > 0 {
		zoneFail(c, http.StatusConflict, "has_computers")
		return
	}
	// Е2-и1: у зоны есть пакеты — база их и так защищает (FK RESTRICT), но
	// владелец увидел бы голое «db_error» и решил, что система сломалась.
	// Проверяем сами и говорим, что именно держит зону.
	db.Model(&models.TimePackage{}).Where("zone_id = ?", z.ID).Count(&n)
	if n == 0 {
		db.Model(&models.UserPackage{}).Where("zone_id = ?", z.ID).Count(&n)
	}
	if n > 0 {
		zoneFail(c, http.StatusConflict, "has_packages")
		return
	}
	if err := db.Delete(&z).Error; err != nil {
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "zone_delete", nil, z.Name)
	c.JSON(http.StatusOK, gin.H{"deleted": z.ID})
}

// zoneFieldsFor — поля обновления ПК при смене зоны: единственный путь, где
// пишутся и ссылка, и кэш имени. Пустой id снимает зону (ПК уйдёт на клубный
// тариф). Второе значение — найденная зона, третье — false, если зоны нет.
func zoneFieldsFor(zoneID string) (map[string]any, *models.Zone, bool) {
	if strings.TrimSpace(zoneID) == "" {
		return map[string]any{"zone_id": nil, "zone": ""}, nil, true
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", zoneID).Error; err != nil {
		return nil, nil, false
	}
	return map[string]any{"zone_id": z.ID, "zone": z.Name}, &z, true
}
