package main

// Клубы: список, карточка, компьютеры со статусами. Публичные GET-роуты
// (мобилка/веб показывают карту клубов до логина), бронь — за JWT (bookings.go).
// З18-и5 (из №40): наружу — белым списком (publicClub, publicComputer в club_card.go):
// без rtp_modifier, схемы зала, MAC и придержания ПК.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// GET /clubs — активные клубы + счётчики ПК (всего/свободно).
func handleGetClubs(c *gin.Context) {
	var clubs []models.Club
	if err := db.Where("is_active = ?", true).Order("name").Find(&clubs).Error; err != nil {
		respondDBError(c, err)
		return
	}

	type row struct {
		Total int64
		Free  int64
	}
	counts := map[string]*row{}
	for i := range clubs {
		counts[clubs[i].ID.String()] = &row{}
	}

	var stats []struct {
		ClubID string
		Status models.ComputerStatus
		N      int64
	}
	db.Model(&models.Computer{}).
		Select("club_id, status, COUNT(*) as n").
		Group("club_id, status").
		Scan(&stats)
	for _, s := range stats {
		if r, ok := counts[s.ClubID]; ok {
			r.Total += s.N
			if s.Status == models.ComputerStatusAvailable {
				r.Free += s.N
			}
		}
	}

	// К2-и5 (KIOSK.md, Р8): тарифы зон — публично, как ценник на стене клуба.
	// Заставка киоска показывает «Standard 23 zł/ч · VIP 30 zł/ч»; только
	// имя и цена часа, без id и служебных полей.
	// З24-и1: цена часа — сейчас, по расписанию цен зоны; следующая (next_at, next_rate_pln) — если сменится скоро.
	var zones []models.Zone
	db.Order("club_id, sort, name").Find(&zones)
	pub := publicZonePrices(zones)
	zonesBy := map[string][]gin.H{}
	for _, z := range zones {
		k := z.ClubID.String()
		zonesBy[k] = append(zonesBy[k], pub[z.ID])
	}

	out := make([]gin.H, 0, len(clubs))
	for _, cl := range clubs {
		r := counts[cl.ID.String()]
		zs := zonesBy[cl.ID.String()]
		if zs == nil {
			zs = []gin.H{}
		}
		row := publicClub(cl)
		row["computers_total"], row["computers_free"], row["zones"] = r.Total, r.Free, zs
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "clubs": out})
}

// GET /clubs/:id — карточка клуба: то же, что строка списка /clubs (З18-и5: раньше — вся строка таблицы, с
// rtp_modifier и схемой зала).
func handleGetClub(c *gin.Context) {
	var club models.Club
	if err := db.First(&club, "id = ? AND is_active = ?", c.Param("id"), true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Клуб не найден"})
		return
	}
	var zones []models.Zone
	db.Where("club_id = ?", club.ID).Order("sort, name").Find(&zones)
	zs := make([]gin.H, 0, len(zones))
	pub := publicZonePrices(zones) // З24-и1: цена сейчас и следующая
	for _, z := range zones {
		zs = append(zs, pub[z.ID])
	}
	var total, free int64
	db.Model(&models.Computer{}).Where("club_id = ?", club.ID).Count(&total)
	db.Model(&models.Computer{}).Where("club_id = ? AND status = ?", club.ID, models.ComputerStatusAvailable).Count(&free)
	row := publicClub(club)
	row["computers_total"], row["computers_free"], row["zones"] = total, free, zs
	c.JSON(http.StatusOK, row)
}

// GET /clubs/:id/computers — ПК клуба со статусами (З18-и5: имя, зона, статус и место на схеме — без MAC и придержания).
func handleGetClubComputers(c *gin.Context) {
	var computers []models.Computer
	if err := db.Where("club_id = ?", c.Param("id")).Order("name").Find(&computers).Error; err != nil {
		respondDBError(c, err)
		return
	}
	free, out := 0, make([]gin.H, 0, len(computers))
	for _, pc := range computers {
		if pc.Status == models.ComputerStatusAvailable {
			free++
		}
		out = append(out, publicComputer(pc))
	}
	c.JSON(http.StatusOK, gin.H{"count": len(computers), "free": free, "computers": out})
}
