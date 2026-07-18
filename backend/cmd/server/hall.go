package main

// Конструктор зала (спринт А8, ADMIN.md): владелец создаёт/правит/убирает ПК
// и задаёт размер схемы клуба. Все роуты — за ownerMiddleware (main.go).
// Правила: имя ПК уникально в клубе; клетка схемы вмещает один ПК; удалить
// можно только ПК без сессий и броней в истории (иначе — в ремонт навсегда).

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// posInBounds — чистая проверка позиции в границах схемы (тест в hall_test.go).
func posInBounds(x, y, w, h int) bool {
	return x >= 0 && x < w && y >= 0 && y < h
}

// cellBusy — занята ли клетка другим ПК клуба (excludeID — сам ПК при переносе).
func cellBusy(clubID, excludeID string, x, y int) bool {
	var n int64
	q := db.Model(&models.Computer{}).
		Where("club_id = ? AND pos_x = ? AND pos_y = ?", clubID, x, y)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	q.Count(&n)
	return n > 0
}

// nameTaken — занято ли имя ПК в клубе.
func nameTaken(clubID, excludeID, name string) bool {
	var n int64
	q := db.Model(&models.Computer{}).Where("club_id = ? AND name = ?", clubID, name)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	q.Count(&n)
	return n > 0
}

type computerCreateRequest struct {
	Name string  `json:"name" binding:"required,min=1,max=32"`
	Zone *string `json:"zone"`
	PosX *int    `json:"pos_x"`
	PosY *int    `json:"pos_y"`
}

// POST /admin/computers — новый ПК (owner). Клуб — первый активный (пилот).
func handleAdminCreateComputer(c *gin.Context) {
	var req computerCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужно имя ПК (1–32 символа)"})
		return
	}
	var club models.Club
	if err := db.First(&club, "is_active = ?", true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Активный клуб не найден"})
		return
	}
	if nameTaken(club.ID.String(), "", req.Name) {
		c.JSON(http.StatusConflict, gin.H{"code": "name_taken", "message": "ПК с таким именем уже есть"})
		return
	}
	if (req.PosX == nil) != (req.PosY == nil) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_pos", "message": "Позиция — оба поля: pos_x и pos_y"})
		return
	}
	if req.PosX != nil {
		if !posInBounds(*req.PosX, *req.PosY, club.LayoutW, club.LayoutH) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "out_of_bounds", "message": "Позиция за границами схемы"})
			return
		}
		if cellBusy(club.ID.String(), "", *req.PosX, *req.PosY) {
			c.JSON(http.StatusConflict, gin.H{"code": "cell_busy", "message": "Эта клетка уже занята"})
			return
		}
	}
	pc := models.Computer{
		ClubID: club.ID, Name: req.Name,
		Status: models.ComputerStatusAvailable,
		PosX:   req.PosX, PosY: req.PosY,
	}
	if req.Zone != nil {
		pc.Zone = *req.Zone
	}
	if err := db.Create(&pc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "pc_create", nil, pc.Name)
	c.JSON(http.StatusCreated, gin.H{"computer_id": pc.ID, "name": pc.Name})
}

type computerPatchRequest struct {
	Name     *string `json:"name" binding:"omitempty,min=1,max=32"`
	Zone     *string `json:"zone"`
	PosX     *int    `json:"pos_x"`
	PosY     *int    `json:"pos_y"`
	ClearPos bool    `json:"clear_pos"` // true — снять ПК со схемы
	MAC      *string `json:"mac"`       // Б8: для Wake-on-LAN; "" — стереть
}

// PATCH /admin/computers/:id — имя/зона/позиция (owner).
func handleAdminUpdateComputer(c *gin.Context) {
	var pc models.Computer
	if err := db.Preload("Club").First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	var req computerPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	updates := map[string]any{}
	changes := ""
	if req.Name != nil && *req.Name != pc.Name {
		if nameTaken(pc.ClubID.String(), pc.ID.String(), *req.Name) {
			c.JSON(http.StatusConflict, gin.H{"code": "name_taken", "message": "ПК с таким именем уже есть"})
			return
		}
		updates["name"] = *req.Name
		changes = pc.Name + " → " + *req.Name
	}
	if req.Zone != nil {
		updates["zone"] = *req.Zone
	}
	if req.MAC != nil { // Б8: MAC для WoL, пустая строка стирает
		m := strings.ToUpper(strings.TrimSpace(*req.MAC))
		if m == "" {
			updates["mac"] = nil
		} else {
			if _, err := net.ParseMAC(m); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_mac", "message": "MAC вида AA:BB:CC:DD:EE:FF"})
				return
			}
			updates["mac"] = m
		}
	}
	if req.ClearPos {
		updates["pos_x"] = nil
		updates["pos_y"] = nil
	} else if req.PosX != nil || req.PosY != nil {
		if (req.PosX == nil) != (req.PosY == nil) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_pos", "message": "Позиция — оба поля: pos_x и pos_y"})
			return
		}
		if !posInBounds(*req.PosX, *req.PosY, pc.Club.LayoutW, pc.Club.LayoutH) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "out_of_bounds", "message": "Позиция за границами схемы"})
			return
		}
		if cellBusy(pc.ClubID.String(), pc.ID.String(), *req.PosX, *req.PosY) {
			c.JSON(http.StatusConflict, gin.H{"code": "cell_busy", "message": "Эта клетка уже занята"})
			return
		}
		updates["pos_x"] = *req.PosX
		updates["pos_y"] = *req.PosY
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "unchanged": true})
		return
	}
	if err := db.Model(&pc).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	if changes == "" {
		changes = pc.Name
	}
	logAdminAction(c, "pc_update", nil, changes)
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID})
}

// DELETE /admin/computers/:id — удалить ПК без истории (owner).
// С историей сессий/броней удалять нельзя — начисления и аудит должны жить;
// такой ПК оставляют в ремонте.
func handleAdminDeleteComputer(c *gin.Context) {
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	var sessions, bookings int64
	db.Model(&models.Session{}).Where("computer_id = ?", pc.ID).Count(&sessions)
	db.Model(&models.Booking{}).Where("computer_id = ?", pc.ID).Count(&bookings)
	if sessions > 0 || bookings > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "has_history",
			"message": fmt.Sprintf("У «%s» есть история (%d сессий, %d броней) — удалить нельзя, оставь его в ремонте", pc.Name, sessions, bookings)})
		return
	}
	if err := db.Delete(&pc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "pc_delete", nil, pc.Name)
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "deleted": true})
}

type layoutRequest struct {
	LayoutW int `json:"layout_w" binding:"required,min=4,max=30"`
	LayoutH int `json:"layout_h" binding:"required,min=3,max=20"`
}

// PATCH /admin/clubs/:id/layout — размер схемы зала (owner).
// Сжимать схему можно только если за новыми границами нет ПК.
func handleAdminClubLayout(c *gin.Context) {
	var club models.Club
	if err := db.First(&club, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Клуб не найден"})
		return
	}
	var req layoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Ширина 4–30, высота 3–20"})
		return
	}
	var outside int64
	db.Model(&models.Computer{}).
		Where("club_id = ? AND pos_x IS NOT NULL AND (pos_x >= ? OR pos_y >= ?)", club.ID, req.LayoutW, req.LayoutH).
		Count(&outside)
	if outside > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "pcs_outside",
			"message": fmt.Sprintf("За новыми границами останется ПК: %d — сначала передвинь их", outside)})
		return
	}
	old := fmt.Sprintf("%d×%d", club.LayoutW, club.LayoutH)
	if err := db.Model(&club).Updates(map[string]any{"layout_w": req.LayoutW, "layout_h": req.LayoutH}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "layout_update", nil, fmt.Sprintf("%s → %d×%d", old, req.LayoutW, req.LayoutH))
	c.JSON(http.StatusOK, gin.H{"club_id": club.ID, "layout_w": req.LayoutW, "layout_h": req.LayoutH})
}
