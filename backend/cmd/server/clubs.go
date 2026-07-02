package main

// Клубы: список, карточка, компьютеры со статусами. Публичные GET-роуты
// (мобилка/веб показывают карту клубов до логина), бронь — за JWT (bookings.go).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// GET /clubs — активные клубы + счётчики ПК (всего/свободно).
func handleGetClubs(c *gin.Context) {
	var clubs []models.Club
	if err := db.Where("is_active = ?", true).Order("name").Find(&clubs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
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

	out := make([]gin.H, 0, len(clubs))
	for _, cl := range clubs {
		r := counts[cl.ID.String()]
		out = append(out, gin.H{
			"id": cl.ID, "name": cl.Name, "address": cl.Address,
			"latitude": cl.Latitude, "longitude": cl.Longitude,
			"phone": cl.Phone, "telegram": cl.Telegram, "instagram": cl.Instagram,
			"base_rate_pln":   cl.BaseRatePLN,
			"computers_total": r.Total, "computers_free": r.Free,
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "clubs": out})
}

// GET /clubs/:id — карточка клуба.
func handleGetClub(c *gin.Context) {
	var club models.Club
	if err := db.First(&club, "id = ? AND is_active = ?", c.Param("id"), true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Клуб не найден"})
		return
	}
	c.JSON(http.StatusOK, club)
}

// GET /clubs/:id/computers — ПК клуба со статусами.
func handleGetClubComputers(c *gin.Context) {
	var computers []models.Computer
	if err := db.Where("club_id = ?", c.Param("id")).Order("name").Find(&computers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	free := 0
	for _, pc := range computers {
		if pc.Status == models.ComputerStatusAvailable {
			free++
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(computers), "free": free, "computers": computers})
}
