package main

// Товары, продажи и остатки (спринт В2 трека владельца, ADMIN.md; миграция 024).
// Решение основателя 2026-08-18: выручка клуба = пополнения + продажи еды и
// товаров; учёт — ценник и остатки без себестоимости; платят только злотыми.
//
// Границы ролей повторяют решение №2 трека Б (каталог): ценник — правила игры,
// его правит owner; продажа, приём поставки и корректировка — операции смены,
// их делает admin. Остаток нельзя поменять «молча»: любое изменение проходит
// через stock_moves, поэтому владелец в отчёте видит потери — всё, что ушло со
// склада мимо кассы.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// errNegativeStock — остаток увели в минус параллельной корректировкой (ревью 26.08).
var errNegativeStock = errors.New("negative_stock")

const (
	maxGoodPrice = 10000.0 // цена позиции, zł
	maxSaleQty   = 99
)

// validateGood — чистая проверка позиции ценника (тест в goods_test.go).
func validateGood(name, category string, price float64, lowStock int) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" || len([]rune(n)) > 64 {
		return false, "bad_name"
	}
	if len([]rune(strings.TrimSpace(category))) > 32 {
		return false, "bad_category"
	}
	if price <= 0 || price > maxGoodPrice {
		return false, "bad_price"
	}
	if lowStock < 0 || lowStock > 100000 {
		return false, "bad_low"
	}
	return true, ""
}

var goodErrors = map[string]string{
	"bad_name":       "Название: 1–64 символа",
	"bad_category":   "Категория: до 32 символов",
	"bad_price":      fmt.Sprintf("Цена — от 0.01 до %.0f zł", maxGoodPrice),
	"bad_low":        "Порог «заканчивается» — неотрицательное число",
	"bad_description": fmt.Sprintf("Описание — до %d символов", maxGoodDescription),
	"bad_qty":        fmt.Sprintf("Количество — от 1 до %d", maxSaleQty),
	"bad_method":     "Оплата: наличные, карта или BLIK",
	"bad_reason":     "Причина движения: supply или adjust",
	"need_note":      "У корректировки должна быть причина — её увидит владелец",
	"negative_stock": "Остаток ушёл бы в минус",
	"good_inactive":  "Позиция выключена — включи её в ценнике",
	"out_of_stock":   "На складе не хватает",
	"has_sales":      "По позиции уже были продажи — её нельзя удалить, только выключить",
	"already_void":   "Продажа уже отменена",
	"not_yours":      "Чужую продажу отменяет владелец",
	"too_old":        "Продажа не с текущей смены — отменяет владелец",
}

func goodFail(c *gin.Context, status int, code string) {
	msg := goodErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

func goodOut(g *models.Good) gin.H {
	return gin.H{
		"id": g.ID, "name": g.Name, "category": g.Category, "price_pln": g.PricePLN,
		"stock": g.Stock, "low_stock": g.LowStock, "sort": g.Sort, "active": g.Active,
		"low": g.LowStock > 0 && g.Stock <= g.LowStock,
		// Г7: карточка позиции для гостевой кухни
		"description": g.Description, "photo": goodPhotoURL(g),
	}
}

// GET /admin/goods — ценник целиком (staff): продавать и видеть остатки нужно
// обоим ролям, выключенные позиции отдаём с флагом — UI решает, что показать.
func handleAdminGoods(c *gin.Context) {
	var goods []models.Good
	db.Order("sort, category, name").Find(&goods)
	out := make([]gin.H, 0, len(goods))
	cats := map[string]bool{}
	low := 0
	for i := range goods {
		out = append(out, goodOut(&goods[i]))
		if goods[i].Category != "" {
			cats[goods[i].Category] = true
		}
		if goods[i].Active && goods[i].LowStock > 0 && goods[i].Stock <= goods[i].LowStock {
			low++
		}
	}
	list := make([]string, 0, len(cats))
	for k := range cats {
		list = append(list, k)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "goods": out, "categories": list, "low_count": low})
}

type goodRequest struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description *string  `json:"description"` // Г7: карточка для гостевой кухни
	PricePLN    *float64 `json:"price_pln"`
	Stock       *int     `json:"stock"`
	LowStock    *int     `json:"low_stock"`
	Sort        *int     `json:"sort"`
	Active      *bool    `json:"active"`
}

const maxGoodDescription = 500 // символов; описание — пара строк на плитке, не статья

// POST /admin/goods — новая позиция ценника (owner). Начальный остаток, если
// задан, сразу ложится движением supply: остаток без следа не появляется.
func handleAdminGoodCreate(c *gin.Context) {
	var req goodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	price := 0.0
	if req.PricePLN != nil {
		price = *req.PricePLN
	}
	low := 0
	if req.LowStock != nil {
		low = *req.LowStock
	}
	if ok, code := validateGood(req.Name, req.Category, price, low); !ok {
		goodFail(c, http.StatusBadRequest, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		goodFail(c, http.StatusConflict, "no_club")
		return
	}
	stock := 0
	if req.Stock != nil {
		stock = *req.Stock
	}
	if stock < 0 {
		goodFail(c, http.StatusBadRequest, "negative_stock")
		return
	}
	g := models.Good{
		ClubID: club.ID, Name: strings.TrimSpace(req.Name),
		Category: strings.TrimSpace(req.Category), PricePLN: price,
		Stock: stock, LowStock: low, Active: true,
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if len([]rune(d)) > maxGoodDescription {
			goodFail(c, http.StatusBadRequest, "bad_description")
			return
		}
		g.Description = d
	}
	if req.Sort != nil {
		g.Sort = *req.Sort
	}
	if req.Active != nil {
		g.Active = *req.Active
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		if stock > 0 {
			return tx.Create(&models.StockMove{ClubID: club.ID, GoodID: g.ID, Delta: stock,
				Reason: "supply", Note: "начальный остаток", CreatedBy: &adminID}).Error
		}
		return nil
	})
	if err != nil {
		if isDuplicate(err) {
			goodFail(c, http.StatusConflict, "already_exists")
			return
		}
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "good_create", nil, fmt.Sprintf("%s · %.2f zł · остаток %d", g.Name, g.PricePLN, g.Stock))
	c.JSON(http.StatusCreated, gin.H{"good": goodOut(&g)})
}

// PATCH /admin/goods/:id — правка позиции (owner). Остаток тут не меняется:
// он живёт только через движения склада, иначе потери не посчитать.
func handleAdminGoodUpdate(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	var req goodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name, category, price, low := g.Name, g.Category, g.PricePLN, g.LowStock
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
	}
	if req.Category != "" {
		category = strings.TrimSpace(req.Category)
	}
	if req.PricePLN != nil {
		price = *req.PricePLN
	}
	if req.LowStock != nil {
		low = *req.LowStock
	}
	if ok, code := validateGood(name, category, price, low); !ok {
		goodFail(c, http.StatusBadRequest, code)
		return
	}
	changes := []string{}
	if name != g.Name {
		changes = append(changes, g.Name+" → "+name)
	}
	if price != g.PricePLN {
		changes = append(changes, fmt.Sprintf("цена %.2f → %.2f zł", g.PricePLN, price))
	}
	if category != g.Category {
		changes = append(changes, "категория: "+category)
	}
	if low != g.LowStock {
		changes = append(changes, fmt.Sprintf("порог %d → %d", g.LowStock, low))
	}
	g.Name, g.Category, g.PricePLN, g.LowStock = name, category, price, low
	if req.Description != nil { // Г7: описание для плитки кухни
		d := strings.TrimSpace(*req.Description)
		if len([]rune(d)) > maxGoodDescription {
			goodFail(c, http.StatusBadRequest, "bad_description")
			return
		}
		if d != g.Description {
			g.Description = d
			changes = append(changes, "описание обновлено")
		}
	}
	if req.Sort != nil {
		g.Sort = *req.Sort
	}
	if req.Active != nil && *req.Active != g.Active {
		g.Active = *req.Active
		changes = append(changes, map[bool]string{true: "включена", false: "выключена"}[g.Active])
	}
	if err := db.Save(&g).Error; err != nil {
		if isDuplicate(err) {
			goodFail(c, http.StatusConflict, "already_exists")
			return
		}
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if len(changes) > 0 {
		logAdminAction(c, "good_update", nil, g.Name+": "+strings.Join(changes, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"good": goodOut(&g)})
}

// DELETE /admin/goods/:id — удалить позицию (owner). Если продажи были —
// не удаляем: иначе история выручки потеряет имя проданного.
func handleAdminGoodDelete(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	var sold int64
	db.Model(&models.Sale{}).Where("good_id = ?", g.ID).Count(&sold)
	if sold > 0 {
		goodFail(c, http.StatusConflict, "has_sales")
		return
	}
	if err := db.Delete(&g).Error; err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "good_delete", nil, g.Name)
	c.JSON(http.StatusOK, gin.H{"deleted": g.ID})
}

type stockRequest struct {
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// POST /admin/goods/:id/stock — приход поставки или корректировка (staff).
// У корректировки причина обязательна: именно по ней владелец потом отличит
// бой и пересчёт от того, что просто «не сошлось».
func handleAdminGoodStock(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	var req stockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Reason != "supply" && req.Reason != "adjust" {
		goodFail(c, http.StatusBadRequest, "bad_reason")
		return
	}
	if req.Delta == 0 {
		goodFail(c, http.StatusBadRequest, "bad_delta")
		return
	}
	if req.Reason == "supply" && req.Delta < 0 {
		goodFail(c, http.StatusBadRequest, "bad_delta")
		return
	}
	note := strings.TrimSpace(req.Note)
	if req.Reason == "adjust" && note == "" {
		goodFail(c, http.StatusBadRequest, "need_note")
		return
	}
	if g.Stock+req.Delta < 0 {
		goodFail(c, http.StatusConflict, "negative_stock")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	err := db.Transaction(func(tx *gorm.DB) error {
		// CAS: проверка «не уйдём в минус» выше сделана вне транзакции, и два
		// одновременных списания уводили склад в минус, раздувая «потери» в
		// отчёте (ревью 26.08).
		res := tx.Model(&models.Good{}).
			Where("id = ? AND stock + ? >= 0", g.ID, req.Delta).
			UpdateColumn("stock", gorm.Expr("stock + ?", req.Delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNegativeStock
		}
		return tx.Create(&models.StockMove{ClubID: g.ClubID, GoodID: g.ID, Delta: req.Delta,
			Reason: req.Reason, Note: note, CreatedBy: &adminID}).Error
	})
	if errors.Is(err, errNegativeStock) {
		goodFail(c, http.StatusConflict, "negative_stock")
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	db.First(&g, "id = ?", g.ID)
	action := "stock_in"
	if req.Reason == "adjust" {
		action = "stock_adjust"
	}
	logAdminAction(c, action, nil, fmt.Sprintf("%s · %+d → %d%s", g.Name, req.Delta, g.Stock, noteSuffix(note)))
	c.JSON(http.StatusOK, gin.H{"good": goodOut(&g)})
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return " · " + note
}

// GET /admin/goods/:id/moves — история движений позиции (owner).
func handleAdminGoodMoves(c *gin.Context) {
	var moves []models.StockMove
	db.Where("good_id = ?", c.Param("id")).Order("created_at DESC").Limit(100).Find(&moves)
	ids := make([]string, 0, len(moves))
	for _, m := range moves {
		if m.CreatedBy != nil {
			ids = append(ids, m.CreatedBy.String())
		}
	}
	nick := nicknamesByID(ids)
	out := make([]gin.H, 0, len(moves))
	for _, m := range moves {
		who := ""
		if m.CreatedBy != nil {
			who = nick[m.CreatedBy.String()]
		}
		out = append(out, gin.H{"created_at": m.CreatedAt, "delta": m.Delta,
			"reason": m.Reason, "note": m.Note, "admin": who})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "moves": out})
}

// ── Продажи ───────────────────────────────────────────────────────────

type saleRequest struct {
	GoodID   string `json:"good_id" binding:"required"`
	Qty      int    `json:"qty"`
	Method   string `json:"method"`
	Nickname string `json:"nickname"`
}

func saleOut(s *models.Sale, nick map[string]string) gin.H {
	row := gin.H{
		"id": s.ID, "name": s.Name, "qty": s.Qty, "price_pln": s.PricePLN,
		"total_pln": s.TotalPLN, "method": s.Method, "created_at": s.CreatedAt,
		"admin": nick[s.CreatedBy.String()], "voided": s.VoidedAt != nil,
	}
	if s.UserID != nil {
		row["nickname"] = nick[s.UserID.String()]
	}
	if s.VoidedAt != nil {
		row["voided_at"] = s.VoidedAt
	}
	return row
}

// POST /admin/sales — продать позицию за злотые (staff).
func handleAdminSaleCreate(c *gin.Context) {
	var req saleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Qty > maxSaleQty {
		goodFail(c, http.StatusBadRequest, "bad_qty")
		return
	}
	method := req.Method
	if method == "" {
		method = "cash"
	}
	if method != "cash" && method != "card" && method != "blik" {
		goodFail(c, http.StatusBadRequest, "bad_method")
		return
	}
	var g models.Good
	if err := db.First(&g, "id = ?", req.GoodID).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	if !g.Active {
		goodFail(c, http.StatusConflict, "good_inactive")
		return
	}
	if g.Stock < req.Qty {
		c.JSON(http.StatusConflict, gin.H{"code": "out_of_stock",
			"message": fmt.Sprintf("«%s»: на складе %d шт", g.Name, g.Stock)})
		return
	}

	// привязка к гостю — необязательная метка «кому продали»; цель, как и у
	// прочих операций персонала, только player (решение №4 трека Б)
	var guest *models.User
	if nickname := strings.TrimSpace(req.Nickname); nickname != "" {
		var u models.User
		if err := db.First(&u, "nickname = ?", nickname).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость с таким ником не найден"})
			return
		}
		if u.Role != models.UserRolePlayer {
			c.JSON(http.StatusConflict, gin.H{"code": "not_player", "message": "Продажу можно записать только на гостя"})
			return
		}
		guest = &u
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	goodID := g.ID
	sale := models.Sale{
		ClubID: g.ClubID, GoodID: &goodID, Name: g.Name, Qty: req.Qty,
		PricePLN: g.PricePLN, TotalPLN: roundPLN(g.PricePLN * float64(req.Qty)),
		Method: method, CreatedBy: adminID,
	}
	if guest != nil {
		sale.UserID = &guest.ID
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		// остаток списываем условием в WHERE: две кассы не уведут его в минус
		res := tx.Model(&models.Good{}).Where("id = ? AND stock >= ?", g.ID, req.Qty).
			UpdateColumn("stock", gorm.Expr("stock - ?", req.Qty))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errOutOfStock
		}
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}
		saleID := sale.ID
		return tx.Create(&models.StockMove{ClubID: g.ClubID, GoodID: g.ID, Delta: -req.Qty,
			Reason: "sale", SaleID: &saleID, CreatedBy: &adminID}).Error
	})
	if err == errOutOfStock {
		goodFail(c, http.StatusConflict, "out_of_stock")
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	payload := map[string]any{"name": sale.Name, "qty": sale.Qty, "total_pln": sale.TotalPLN, "method": sale.Method}
	if guest != nil {
		payload["nickname"] = guest.Nickname
	}
	hub.AdminBroadcast("sale", payload)

	db.First(&g, "id = ?", g.ID)
	nick := nicknamesByID([]string{adminID.String()})
	if guest != nil {
		nick[guest.ID.String()] = guest.Nickname
	}
	c.JSON(http.StatusCreated, gin.H{"sale": saleOut(&sale, nick), "good": goodOut(&g)})
}

var errOutOfStock = fmt.Errorf("out_of_stock")

func roundPLN(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// GET /admin/sales?date=YYYY-MM-DD — лента продаж (staff).
// Роли, решение №3 трека Б: admin видит только текущие клубные сутки,
// owner листает любой день.
func handleAdminSales(c *gin.Context) {
	reportHour := int(settingInt64("report_hour", 8))
	from, to, key, ok := shiftWindow(c.Query("date"), reportHour, time.Now())
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": "date — в формате ГГГГ-ММ-ДД"})
		return
	}
	if c.GetString("user_role") != string(models.UserRoleOwner) {
		from, to, key, _ = shiftWindow("", reportHour, time.Now())
	}
	var sales []models.Sale
	db.Where("created_at >= ? AND created_at < ?", from, to).
		Order("created_at DESC").Limit(200).Find(&sales)

	ids := make([]string, 0, len(sales)*2)
	for _, s := range sales {
		ids = append(ids, s.CreatedBy.String())
		if s.UserID != nil {
			ids = append(ids, s.UserID.String())
		}
	}
	nick := nicknamesByID(ids)
	out := make([]gin.H, 0, len(sales))
	total, walletTotal, voided := 0.0, 0.0, 0
	for i := range sales {
		out = append(out, saleOut(&sales[i], nick))
		switch {
		case sales[i].VoidedAt != nil:
			voided++
		case sales[i].Method == "wallet": // Г7/Р7: кухня кошельком — не выручка
			walletTotal += sales[i].TotalPLN
		default:
			total += sales[i].TotalPLN
		}
	}
	c.JSON(http.StatusOK, gin.H{"date": key, "count": len(out), "sales": out,
		"total_pln": roundPLN(total), "wallet_pln": roundPLN(walletTotal), "voided": voided})
}

// canVoidSale — чистая (тест в goods_test.go): владелец отменяет любую продажу,
// админ — только свою и только в пределах текущих клубных суток. Смена сдана —
// исправляет владелец, иначе «отмена» становится тихим способом убрать выручку.
func canVoidSale(role, saleCreatedBy, userID string, saleAt, dayFrom, dayTo time.Time) (bool, string) {
	if role == string(models.UserRoleOwner) {
		return true, ""
	}
	if saleCreatedBy != userID {
		return false, "not_yours"
	}
	if saleAt.Before(dayFrom) || !saleAt.Before(dayTo) {
		return false, "too_old"
	}
	return true, ""
}

// POST /admin/sales/:id/void — отменить продажу, вернуть товар на склад.
func handleAdminSaleVoid(c *gin.Context) {
	var sale models.Sale
	if err := db.First(&sale, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "sale_not_found", "message": "Продажа не найдена"})
		return
	}
	if sale.VoidedAt != nil {
		goodFail(c, http.StatusConflict, "already_void")
		return
	}
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	if ok, code := canVoidSale(c.GetString("user_role"), sale.CreatedBy.String(),
		c.GetString("user_id"), sale.CreatedAt, from, to); !ok {
		goodFail(c, http.StatusForbidden, code)
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		// CAS по voided_at: проверка выше вне транзакции, и двойной клик по
		// «отменить» возвращал деньги на кошелёк и товар на склад дважды
		// (ревью 26.08).
		res := tx.Model(&models.Sale{}).
			Where("id = ? AND voided_at IS NULL", sale.ID).
			Updates(map[string]any{"voided_at": now, "voided_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errAlreadyVoid
		}
		// Г7: продажа «кошельком» — это выданный заказ кухни; отмена возвращает
		// деньги в кошелёк гостя (иначе списание повисло бы без товара)
		if sale.Method == "wallet" && sale.UserID != nil {
			saleID := sale.ID
			if _, err := walletApply(tx, walletOp{
				UserID: *sale.UserID, Kind: models.WalletTxRefund,
				Amount: models.GroszFromPLN(sale.TotalPLN), RefType: "sale", RefID: &saleID,
				Note: "возврат: отмена продажи " + sale.Name, CreatedBy: &adminID,
			}); err != nil {
				return err
			}
		}
		if sale.GoodID == nil { // позицию успели удалить — возвращать некуда
			return nil
		}
		if err := tx.Model(&models.Good{}).Where("id = ?", *sale.GoodID).
			UpdateColumn("stock", gorm.Expr("stock + ?", sale.Qty)).Error; err != nil {
			return err
		}
		saleID := sale.ID
		return tx.Create(&models.StockMove{ClubID: sale.ClubID, GoodID: *sale.GoodID, Delta: sale.Qty,
			Reason: "void", Note: "отмена продажи", SaleID: &saleID, CreatedBy: &adminID}).Error
	})
	if errors.Is(err, errAlreadyVoid) {
		goodFail(c, http.StatusConflict, "already_void")
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	hub.AdminBroadcast("sale", map[string]any{"kind": "void", "name": sale.Name, "total_pln": sale.TotalPLN})
	c.JSON(http.StatusOK, gin.H{"voided": sale.ID})
}
