package main

// Каталог приложений гостевого экрана (ТЗ 6.2: конфигурируется через Admin Panel).
// GET /catalog — публичный (гостевой экран и shell-agent читают без токена; пути
// запуска в LAN клуба не секрет). Изменение — только админом.
// До пилота агент получит собственную аутентификацию (MAC+токен, см. STATUS).

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

var catalogIDRe = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)

// validateCatalogApp — чистая проверка полей (тестируется отдельно).
func validateCatalogApp(id, name, category string) (ok bool, code string) {
	if !catalogIDRe.MatchString(id) {
		return false, "bad_id"
	}
	if name == "" || len(name) > 64 {
		return false, "bad_name"
	}
	if category != "game" && category != "app" && category != "system" && category != "platform" {
		return false, "bad_category"
	}
	return true, ""
}

// GET /catalog — включённые приложения по категориям (для экрана и агента).
func handleGetCatalog(c *gin.Context) {
	var apps []models.CatalogApp
	if err := db.Where("enabled = ?", true).Order("sort").Order("name").Find(&apps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	games, appsList, system, platforms := []models.CatalogApp{}, []models.CatalogApp{}, []models.CatalogApp{}, []models.CatalogApp{}
	for _, a := range apps {
		switch a.Category {
		case "game":
			games = append(games, a)
		case "app":
			appsList = append(appsList, a)
		case "system":
			system = append(system, a)
		case "platform":
			platforms = append(platforms, a)
		}
	}
	c.JSON(http.StatusOK, gin.H{"games": games, "apps": appsList, "system": system, "platforms": platforms})
}

// GET /admin/catalog — все приложения, включая выключенные.
func handleAdminCatalog(c *gin.Context) {
	var apps []models.CatalogApp
	db.Order("category").Order("sort").Order("name").Find(&apps)
	c.JSON(http.StatusOK, gin.H{"count": len(apps), "apps": apps})
}

type catalogUpsertRequest struct {
	ID       string  `json:"id" binding:"required"`
	Name     string  `json:"name" binding:"required"`
	Category string  `json:"category" binding:"required"`
	Subtitle *string `json:"subtitle"`
	Tag      *string `json:"tag"`
	Emoji    *string `json:"emoji"`
	ColorA   *string `json:"color_a"`
	ColorB   *string `json:"color_b"`
	Target   *string `json:"target"`
	Args     *string `json:"args"`
	Sort     *int    `json:"sort"`
}

// POST /admin/catalog — создать или обновить приложение (upsert по id).
func handleAdminCatalogUpsert(c *gin.Context) {
	var req catalogUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if ok, code := validateCatalogApp(req.ID, req.Name, req.Category); !ok {
		msg := map[string]string{
			"bad_id":       "id: строчные латинские буквы/цифры/_, 2–32 символа",
			"bad_name":     "Название: 1–64 символа",
			"bad_category": "category: game | app | system | platform",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	app := models.CatalogApp{
		ID: req.ID, Name: req.Name, Category: req.Category,
		Subtitle: req.Subtitle, Tag: req.Tag, Emoji: req.Emoji,
		ColorA: req.ColorA, ColorB: req.ColorB,
		Target: req.Target, Args: req.Args,
		Sort: 100, Enabled: true,
	}
	if req.Sort != nil {
		app.Sort = *req.Sort
	}

	// upsert: сохранить как новый или обновить существующий (enabled не трогаем)
	var existing models.CatalogApp
	if err := db.First(&existing, "id = ?", req.ID).Error; err == nil {
		app.Enabled = existing.Enabled
		if err := db.Model(&existing).Select("*").Omit("id").Updates(app).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
			return
		}
	} else if err := db.Create(&app).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "catalog_upsert", nil, app.ID)
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "ok": true})
}

// POST /admin/catalog/:id/toggle — включить/выключить приложение.
func handleAdminCatalogToggle(c *gin.Context) {
	var app models.CatalogApp
	if err := db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	newEnabled := !app.Enabled
	if err := db.Model(&app).Update("enabled", newEnabled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	state := "выключено"
	if newEnabled {
		state = "включено"
	}
	logAdminAction(c, "catalog_toggle", nil, app.ID+" — "+state)
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "enabled": newEnabled})
}

// DELETE /admin/catalog/:id — удалить приложение из каталога.
func handleAdminCatalogDelete(c *gin.Context) {
	res := db.Delete(&models.CatalogApp{}, "id = ?", c.Param("id"))
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	logAdminAction(c, "catalog_delete", nil, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "deleted": true})
}
