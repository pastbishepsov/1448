package main

// З13-и1 «Ставки VAT: схема и снимки» (OWNER.md, трек З; №72 «а»; решения основателя 04.10 — бухгалтерия под любого
// бухгалтера: каждый вопрос бухгалтеру — настройка владельца; только Польша; VAT у пополнения — настройкой, по
// умолчанию «при трате»).
//
// Ставка VAT — у зоны, пакета и позиции; ручки зон, пакетов и позиций принимают и отдают `vat_pct`. Снимок ставки в
// операции пишет модель (models/vat.go). Здесь — список ставок для экрана и «ставка всей категории» одной кнопкой.
// Всё — только владельцу.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// vatBadMessage — ответ на ставку не из списка: «Ставка VAT — одна из: 23 %, 8 %, 5 %, 0 %, zw».
var vatBadMessage = "Ставка VAT — одна из: " + vatRatesText()

func vatRatesText() string {
	parts := make([]string, 0, len(models.VatRates))
	for _, v := range models.VatRates {
		parts = append(parts, models.VatLabel(v))
	}
	return strings.Join(parts, ", ")
}

// vatFromRequest — ставка из запроса: не прислана — def; не из списка — false (чистая, тест).
func vatFromRequest(v *int, def int) (int, bool) {
	if v == nil {
		return def, true
	}
	return *v, models.ValidVat(*v)
}

// GET /admin/vat/rates — ставки, которые можно выбрать, с подписями (owner): экран строит выбор по ним, и смена
// ставок законом — одна строка на сервере.
func handleVatRates(c *gin.Context) {
	out := make([]gin.H, 0, len(models.VatRates))
	for _, v := range models.VatRates {
		out = append(out, gin.H{"vat_pct": v, "label": models.VatLabel(v)})
	}
	c.JSON(http.StatusOK, gin.H{"rates": out, "default": models.VatDefault})
}

type goodsVatRequest struct {
	Category *string `json:"category"` // "" — позиции без категории
	VatPct   *int    `json:"vat_pct"`
}

// PUT /admin/vat/goods — ставка VAT всей категории позиций одной кнопкой (owner): напитки — 23 %, кухня — 8 %.
// Прошлые продажи не меняются — у них свой снимок.
func handleGoodsVat(c *gin.Context) {
	var req goodsVatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Category == nil || req.VatPct == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны категория и ставка"})
		return
	}
	if !models.ValidVat(*req.VatPct) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_vat", "message": vatBadMessage})
		return
	}
	cat := strings.TrimSpace(*req.Category)
	res := db.Model(&models.Good{}).Where("category = ? AND vat_pct <> ?", cat, *req.VatPct).Update("vat_pct", *req.VatPct)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "Не сохранилось — попробуй ещё раз"})
		return
	}
	var total int64
	db.Model(&models.Good{}).Where("category = ?", cat).Count(&total)
	if res.RowsAffected > 0 {
		name := cat
		if name == "" {
			name = "без категории"
		}
		n := int(res.RowsAffected)
		logAdminAction(c, "goods_vat", nil, fmt.Sprintf("категория «%s»: VAT → %s (%d %s)",
			name, models.VatLabel(*req.VatPct), n, plural(n, "позиция", "позиции", "позиций")))
	}
	c.JSON(http.StatusOK, gin.H{"category": cat, "vat_pct": *req.VatPct, "changed": res.RowsAffected, "total": total})
}
