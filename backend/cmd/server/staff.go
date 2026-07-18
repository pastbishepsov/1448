package main

// Персонал (спринт Б5, ADMIN.md; решение №9 — на своём Go+Postgres).
// Owner назначает/снимает админов из админки вместо SQL. Роль вступает
// в силу после перелогина сотрудника (роль едет в JWT-клейме).
// Все роуты — за ownerMiddleware (main.go); события — в аудит staff_*
// (от роли admin они скрыты фильтром Б1 в audit.go).

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// canChangeStaffRole — чистое правило смены роли (тест в staff_test.go):
// назначить можно только гостя (player), снять — только админа;
// owner'а и самого себя не трогаем.
func canChangeStaffRole(targetRole models.UserRole, targetID, actorID string, promote bool) (ok bool, code string) {
	if targetID != "" && targetID == actorID {
		return false, "cannot_touch_self"
	}
	if targetRole == models.UserRoleOwner {
		return false, "cannot_touch_owner"
	}
	if promote && targetRole != models.UserRolePlayer {
		return false, "already_staff"
	}
	if !promote && targetRole != models.UserRoleAdmin {
		return false, "not_admin"
	}
	return true, ""
}

var staffRoleErrors = map[string]string{
	"cannot_touch_self":  "Свою роль менять нельзя",
	"cannot_touch_owner": "Владельца трогать нельзя",
	"already_staff":      "Это уже персонал",
	"not_admin":          "Снять можно только админа",
}

// GET /admin/staff — персонал клуба: ник, роль, был активен, действий сегодня.
func handleAdminStaffList(c *gin.Context) {
	var staff []models.User
	db.Where("role IN ?", []models.UserRole{models.UserRoleOwner, models.UserRoleAdmin}).
		Order("role DESC").Order("nickname").Find(&staff)

	// действий сегодня: admin_actions + ручные начисления + депозиты
	today := startOfToday()
	acts := map[string]int64{}
	type cnt struct {
		AdminID string
		N       int64
	}
	var rows []cnt
	db.Model(&models.AdminAction{}).Select("admin_id, COUNT(*) AS n").
		Where("created_at >= ?", today).Group("admin_id").Scan(&rows)
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}
	rows = nil
	db.Model(&models.AdminGrant{}).Select("admin_id, COUNT(*) AS n").
		Where("created_at >= ?", today).Group("admin_id").Scan(&rows)
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}
	rows = nil
	db.Model(&models.Deposit{}).Select("created_by AS admin_id, COUNT(*) AS n").
		Where("created_at >= ? AND created_by IS NOT NULL", today).Group("created_by").Scan(&rows)
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}

	out := make([]gin.H, 0, len(staff))
	for _, u := range staff {
		out = append(out, gin.H{
			"id": u.ID, "nickname": u.Nickname, "role": u.Role,
			"last_active_at": u.LastActiveAt,
			"actions_today":  acts[u.ID.String()],
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "staff": out})
}

// POST /admin/staff {nickname} — назначить гостя админом (owner).
func handleAdminStaffPromote(c *gin.Context) {
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен ник"})
		return
	}
	var user models.User
	if err := db.First(&user, "nickname = ?", strings.TrimSpace(req.Nickname)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость с таким ником не найден"})
		return
	}
	if ok, code := canChangeStaffRole(user.Role, user.ID.String(), c.GetString("user_id"), true); !ok {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": staffRoleErrors[code]})
		return
	}
	if err := db.Model(&user).Update("role", models.UserRoleAdmin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "staff_promote", &target, user.Nickname+" → admin")
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": models.UserRoleAdmin})
}

// DELETE /admin/staff/:id — снять админа до гостя (owner).
func handleAdminStaffDemote(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if ok, code := canChangeStaffRole(user.Role, user.ID.String(), c.GetString("user_id"), false); !ok {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": staffRoleErrors[code]})
		return
	}
	if err := db.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "staff_demote", &target, user.Nickname+" → player")
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": models.UserRolePlayer})
}
