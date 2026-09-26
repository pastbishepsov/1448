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

// errStockLimit — параллельный приход поднял бы остаток выше maxStockQty (З6-и3).
var errStockLimit = errors.New("stock_limit")

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
	if !validatePLN(price, maxGoodPrice) { // З6-и3: не меньше 0,01 и два знака, как в базе
		return false, "bad_price"
	}
	if lowStock < 0 || lowStock > 100000 {
		return false, "bad_low"
	}
	return true, ""
}

var goodErrors = map[string]string{
	"bad_name":        "Название: 1–64 символа",
	"bad_category":    "Категория: до 32 символов",
	"bad_price":       fmt.Sprintf("Цена — от 0.01 до %.0f zł, не больше двух знаков после запятой", maxGoodPrice),
	"bad_sort":        sortMessage,
	"bad_stock":       fmt.Sprintf("Остаток — от 0 до %d штук", maxStockQty),
	"bad_delta":       fmt.Sprintf("Количество — от 1 до %d штук, приход — только плюсом", maxStockQty),
	"stock_limit":     fmt.Sprintf("На складе стало бы больше %d штук — проверь количество", maxStockQty),
	"bad_note":        fmt.Sprintf("Примечание — до %d символов", maxStockNote),
	"bad_low":         "Порог «заканчивается» — неотрицательное число",
	"bad_description": fmt.Sprintf("Описание — до %d символов", maxGoodDescription),
	"bad_qty":         fmt.Sprintf("Количество — от 1 до %d", maxSaleQty),
	"too_many_items":  fmt.Sprintf("В чеке — до %d позиций", maxReceiptItems),
	"bad_method":      "Оплата: наличные, карта, BLIK — или «при выходе» гостю за ПК",
	"bad_reason":      "Причина движения: supply или adjust",
	"need_note":       "У корректировки должна быть причина — её увидит владелец",
	"negative_stock":  "Остаток ушёл бы в минус",
	"good_inactive":   "Позиция выключена — включи её в ценнике",
	"out_of_stock":    "На складе не хватает",
	"has_sales":       "По позиции уже были продажи — её нельзя удалить, только выключить",
	"has_history":     "У позиции есть история склада или остаток — удалить нельзя, выключи её: история и «Потери» останутся",
	"already_void":    "Продажа уже отменена",
	"not_yours":       "Чужую продажу отменяет владелец",
	"too_old":         "Продажа не с текущей смены — отменяет владелец",
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
	Category    *string  `json:"category"`    // З6-и3: указатель — пустая строка очищает категорию
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
	category := ""
	if req.Category != nil {
		category = strings.TrimSpace(*req.Category)
	}
	if ok, code := validateGood(req.Name, category, price, low); !ok {
		goodFail(c, http.StatusBadRequest, code)
		return
	}
	if !validSort(req.Sort) {
		goodFail(c, http.StatusBadRequest, "bad_sort")
		return
	}
	price = round2(price) // хвост float (0,1 + 0,2) — в ответе то же, что в базе
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
	if stock > maxStockQty {
		goodFail(c, http.StatusBadRequest, "bad_stock")
		return
	}
	g := models.Good{
		ClubID: club.ID, Name: strings.TrimSpace(req.Name),
		Category: category, PricePLN: price,
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
				Reason: "supply", Note: "начальный остаток", CreatedBy: &adminID, PricePLN: g.PricePLN}).Error
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
	if req.Category != nil { // З6-и3: "" очищает; поле не прислано — категория как была
		category = strings.TrimSpace(*req.Category)
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
	if !validSort(req.Sort) {
		goodFail(c, http.StatusBadRequest, "bad_sort")
		return
	}
	price = round2(price)
	changes := []string{}
	if name != g.Name {
		changes = append(changes, g.Name+" → "+name)
	}
	if price != g.PricePLN {
		changes = append(changes, fmt.Sprintf("цена %.2f → %.2f zł", g.PricePLN, price))
	}
	if category != g.Category && category == "" {
		changes = append(changes, "категория убрана")
	} else if category != g.Category {
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
	// З6-и2 (№31): удаление уносило остаток и все движения склада, и «Потери»
	// за прошлые дни менялись задним числом. Удаляется только позиция без
	// движений и с нулевым остатком; остальное — выключить. База держит то же
	// правило (ON DELETE RESTRICT, миграция 069) — и на гонке с приходом.
	var moves int64
	db.Model(&models.StockMove{}).Where("good_id = ?", g.ID).Count(&moves)
	if moves > 0 || g.Stock != 0 {
		goodFail(c, http.StatusConflict, "has_history")
		return
	}
	if err := db.Delete(&g).Error; err != nil {
		if strings.Contains(err.Error(), "23503") { // нарушение внешнего ключа: движение успело появиться
			goodFail(c, http.StatusConflict, "has_history")
			return
		}
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
	// З6-и3: количество — в пределах ±maxStockQty (3·10⁹ падало на int4 базы
	// ошибкой сервера), примечание — до maxStockNote символов (З-Р36).
	if req.Delta == 0 || req.Delta > maxStockQty || req.Delta < -maxStockQty {
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
	if len([]rune(note)) > maxStockNote {
		goodFail(c, http.StatusBadRequest, "bad_note")
		return
	}
	if g.Stock+req.Delta < 0 {
		goodFail(c, http.StatusConflict, "negative_stock")
		return
	}
	if req.Delta > 0 && g.Stock+req.Delta > maxStockQty {
		goodFail(c, http.StatusConflict, "stock_limit")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	err := db.Transaction(func(tx *gorm.DB) error {
		// CAS: проверка «не уйдём в минус» выше сделана вне транзакции, и два
		// одновременных списания уводили склад в минус, раздувая «потери» в
		// отчёте (ревью 26.08).
		// Потолок остатка (З6-и3) — в том же условии: списание он не держит, приход — да.
		res := tx.Model(&models.Good{}).
			Where("id = ? AND stock + ? >= 0 AND (? < 0 OR stock + ? <= ?)", g.ID, req.Delta, req.Delta, req.Delta, maxStockQty).
			UpdateColumn("stock", gorm.Expr("stock + ?", req.Delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 && req.Delta > 0 {
			return errStockLimit
		}
		if res.RowsAffected == 0 {
			return errNegativeStock
		}
		return tx.Create(&models.StockMove{ClubID: g.ClubID, GoodID: g.ID, Delta: req.Delta,
			Reason: req.Reason, Note: note, CreatedBy: &adminID, PricePLN: g.PricePLN}).Error
	})
	if errors.Is(err, errNegativeStock) {
		goodFail(c, http.StatusConflict, "negative_stock")
		return
	}
	if errors.Is(err, errStockLimit) {
		goodFail(c, http.StatusConflict, "stock_limit")
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
			"reason": m.Reason, "note": m.Note, "admin": who, "price_pln": m.PricePLN})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "moves": out})
}

// ── Продажи ───────────────────────────────────────────────────────────

type saleItem struct {
	GoodID string `json:"good_id"`
	Qty    int    `json:"qty"`
}

type saleRequest struct {
	GoodID   string     `json:"good_id"` // одна позиция (как до Ж6) …
	Qty      int        `json:"qty"`
	Items    []saleItem `json:"items"`    // … или корзина (Ж6-и2): все позиции одним чеком
	Method   string     `json:"method"`   // cash | card | blik | later (Ж6-и1: в долг до выхода, только гостю за ПК)
	Nickname string     `json:"nickname"` // строгий ник (как понимала админка до Ж5)
	Guest    string     `json:"guest"`    // Ж5-и4: ник, телефон ИЛИ имя — через общий резолвер
	GuestID  string     `json:"guest_id"` // Ж5-и4: выбран из списка кандидатов (409 guest_ambiguous)
}

// maxReceiptItems — позиций в одном чеке; больше — это уже поставка, а не продажа
const maxReceiptItems = 20

func saleOut(s *models.Sale, nick map[string]string) gin.H {
	row := gin.H{
		"id": s.ID, "name": s.Name, "qty": s.Qty, "price_pln": s.PricePLN,
		"total_pln": s.TotalPLN, "method": s.Method, "created_at": s.CreatedAt,
		"admin": nick[s.CreatedBy.String()], "voided": s.VoidedAt != nil,
		// Ж6 (Ж-Р8): в долг — paid_at пуст, деньги придут при расчёте у стойки
		"paid_at": s.PaidAt, "unpaid": s.PaidAt == nil && s.VoidedAt == nil, "postpaid": s.Postpaid,
	}
	if s.ReceiptID != nil {
		row["receipt_id"] = s.ReceiptID // Ж6-и2: чек — позиции одной продажи
	}
	if s.UserID != nil {
		row["nickname"] = nick[s.UserID.String()]
		row["user_id"] = s.UserID // Ж5-и4: следующие продажи тому же гостю — по id, без второго поиска
	}
	if s.VoidedAt != nil {
		row["voided_at"] = s.VoidedAt
	}
	return row
}

// saleMethodOK — способы живых денег у стойки (Ж-Р8: кошельком товары не платятся).
func saleMethodOK(m string) bool {
	return m == "cash" || m == "card" || m == "blik"
}

// saleItemsFromRequest — корзина из запроса (чистая, тест): одна позиция
// good_id/qty (как до Ж6) или список items; количества по каждой строке
// нормализуются (пусто = 1), одинаковые позиции складываются в одну строку.
func saleItemsFromRequest(req *saleRequest) ([]saleItem, string) {
	items := req.Items
	if len(items) == 0 {
		if strings.TrimSpace(req.GoodID) == "" {
			return nil, "invalid_request"
		}
		items = []saleItem{{GoodID: req.GoodID, Qty: req.Qty}}
	}
	if len(items) > maxReceiptItems*10 { // сырой список до слияния: защита от мусора, не бизнес-лимит
		return nil, "too_many_items"
	}
	merged := make([]saleItem, 0, len(items))
	idx := map[string]int{}
	for _, it := range items {
		id := strings.TrimSpace(it.GoodID)
		if id == "" {
			return nil, "invalid_request"
		}
		q := it.Qty
		if q <= 0 {
			q = 1
		}
		if i, ok := idx[id]; ok {
			merged[i].Qty += q
		} else {
			idx[id] = len(merged)
			merged = append(merged, saleItem{GoodID: id, Qty: q})
		}
	}
	if len(merged) > maxReceiptItems {
		return nil, "too_many_items"
	}
	for _, it := range merged {
		if it.Qty > maxSaleQty {
			return nil, "bad_qty"
		}
	}
	return merged, ""
}

// POST /admin/sales — продать позицию или корзину за злотые (staff).
//
// Ж6-и2: `items` — несколько позиций одним чеком (общий receipt_id, одна
// транзакция: не хватило одной — не продано ничего). Ж6-и1 (Ж-Р8):
// method=later — в долг «рассчитать у стойки при выходе», только гостю с
// идущей сессией: товар выдан и склад списан сразу, деньги (и выручка) —
// при расчёте (/admin/users/:id/settle), как у кухни (Р10).
func handleAdminSaleCreate(c *gin.Context) {
	var req saleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	items, code := saleItemsFromRequest(&req)
	if code != "" {
		goodFail(c, http.StatusBadRequest, code)
		return
	}
	method := req.Method
	if method == "" {
		method = "cash"
	}
	if !saleMethodOK(method) && method != models.SaleMethodLater {
		goodFail(c, http.StatusBadRequest, "bad_method")
		return
	}

	// привязка к гостю — необязательная метка «кому продали» (у долга —
	// обязательная); цель, как и у прочих операций персонала, только player
	// (решение №4 трека Б). Ж5-и4 (№15): опознание — общим резолвером посадки
	// и очереди: ник, телефон или имя, при неоднозначности 409 с кандидатами;
	// строгий nickname понимается как раньше. Резолвер ищет только среди
	// гостей, поэтому ник персонала здесь — «не найден», а не отдельный код.
	var guest *models.User
	if strings.TrimSpace(req.Nickname) != "" || strings.TrimSpace(req.Guest) != "" || strings.TrimSpace(req.GuestID) != "" {
		guest = lookupGuestForAction(c, req.GuestID, req.Guest, req.Nickname) // 404 и 409 пишет сам
		if guest == nil {
			return
		}
	}
	if method == models.SaleMethodLater {
		if guest == nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "need_guest", "message": "В долг — только на гостя: укажи, кому продаёшь"})
			return
		}
		if guest.Status == models.UserStatusBanned {
			c.JSON(http.StatusConflict, gin.H{"code": "banned", "message": "Аккаунт заблокирован — в долг не продаём"})
			return
		}
		var n int64
		db.Model(&models.Session{}).Where("user_id = ? AND status = ?", guest.ID, models.SessionStatusActive).Count(&n)
		if n == 0 {
			c.JSON(http.StatusConflict, gin.H{"code": "need_session",
				"message": "В долг — только гостю за ПК (рассчитается при выходе); без сессии продай за нал, карту или BLIK"})
			return
		}
	}

	// позиции чека: активны и есть на складе (точная проверка — CAS в транзакции)
	goods := make([]models.Good, 0, len(items))
	for _, it := range items {
		var g models.Good
		if err := db.First(&g, "id = ?", it.GoodID).Error; err != nil {
			goodFail(c, http.StatusNotFound, "good_not_found")
			return
		}
		if !g.Active {
			c.JSON(http.StatusConflict, gin.H{"code": "good_inactive", "message": fmt.Sprintf("«%s» выключена — включи её в ценнике", g.Name)})
			return
		}
		if g.Stock < it.Qty {
			c.JSON(http.StatusConflict, gin.H{"code": "out_of_stock",
				"message": fmt.Sprintf("«%s»: на складе %d шт", g.Name, g.Stock)})
			return
		}
		goods = append(goods, g)
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	receiptID := uuid.New()
	sales := make([]models.Sale, 0, len(items))
	for i, it := range items {
		g := goods[i]
		goodID := g.ID
		s := models.Sale{
			ClubID: g.ClubID, GoodID: &goodID, Name: g.Name, Qty: it.Qty,
			PricePLN: g.PricePLN, TotalPLN: roundPLN(g.PricePLN * float64(it.Qty)),
			Method: method, CreatedBy: adminID, ReceiptID: &receiptID,
		}
		if method == models.SaleMethodLater {
			s.Postpaid = true // paid_at пуст — деньги при расчёте
		} else {
			paid := now
			s.PaidAt = &paid
		}
		if guest != nil {
			s.UserID = &guest.ID
		}
		sales = append(sales, s)
	}
	var short *models.Good
	err := db.Transaction(func(tx *gorm.DB) error {
		for i := range sales {
			g := &goods[i]
			// остаток списываем условием в WHERE: две кассы не уведут его в минус
			res := tx.Model(&models.Good{}).Where("id = ? AND stock >= ?", g.ID, sales[i].Qty).
				UpdateColumn("stock", gorm.Expr("stock - ?", sales[i].Qty))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				short = g
				return errOutOfStock
			}
			if err := tx.Create(&sales[i]).Error; err != nil {
				return err
			}
			saleID := sales[i].ID
			if err := tx.Create(&models.StockMove{ClubID: g.ClubID, GoodID: g.ID, Delta: -sales[i].Qty,
				Reason: "sale", SaleID: &saleID, CreatedBy: &adminID, PricePLN: sales[i].PricePLN}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == errOutOfStock {
		msg := goodErrors["out_of_stock"]
		if short != nil {
			msg = fmt.Sprintf("«%s»: на складе не хватает — чек не продан", short.Name)
		}
		c.JSON(http.StatusConflict, gin.H{"code": "out_of_stock", "message": msg})
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	total := 0.0
	names := make([]string, 0, len(sales))
	for i := range sales {
		total += sales[i].TotalPLN
		names = append(names, receiptItemName(&sales[i]))
	}
	total = roundPLN(total)
	payload := map[string]any{"name": strings.Join(names, ", "), "qty": len(sales), "total_pln": total, "method": method,
		"receipt_id": receiptID, "items": len(sales)}
	if len(sales) == 1 {
		payload["name"], payload["qty"] = sales[0].Name, sales[0].Qty
	}
	if guest != nil {
		payload["nickname"] = guest.Nickname
	}
	if method == models.SaleMethodLater {
		payload["kind"] = "later" // в ленте: «в долг», не выручка
	}
	hub.AdminBroadcast("sale", payload)

	nick := nicknamesByID([]string{adminID.String()})
	if guest != nil {
		nick[guest.ID.String()] = guest.Nickname
	}
	outSales := make([]gin.H, 0, len(sales))
	for i := range sales {
		outSales = append(outSales, saleOut(&sales[i], nick))
	}
	goodsOut := make([]gin.H, 0, len(goods))
	for i := range goods {
		var g models.Good
		if db.First(&g, "id = ?", goods[i].ID).Error == nil {
			goodsOut = append(goodsOut, goodOut(&g))
		}
	}
	resp := gin.H{
		"sale": outSales[0], "good": goodsOut[0], // как до Ж6: первая (и обычно единственная) позиция
		"receipt": gin.H{"id": receiptID, "total_pln": total, "items": outSales, "method": method,
			"unpaid": method == models.SaleMethodLater, "count": len(outSales)},
		"goods": goodsOut,
	}
	if guest != nil {
		resp["nickname"] = guest.Nickname
		if method == models.SaleMethodLater {
			resp["debt"] = guestDebt(guest.ID) // сколько гость теперь должен всего (кухня + бар)
		}
	}
	c.JSON(http.StatusCreated, resp)
}

// receiptItemName — «Coca-Cola ×2» для строки чека в ленте и журнале.
func receiptItemName(s *models.Sale) string {
	if s.Qty > 1 {
		return fmt.Sprintf("%s ×%d", s.Name, s.Qty)
	}
	return s.Name
}

var errOutOfStock = fmt.Errorf("out_of_stock")

func roundPLN(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// salesFeedLimit — сколько последних строк отдаёт лента продаж смены; итоги
// ленты — по всему окну суток (З8-и4).
const salesFeedLimit = 300

// salesFeedWhere — окно ленты (Ж6, Ж-Р8): проданное в окне И рассчитанное в
// окне (долг вчерашней смены, закрытый сегодня, — сегодняшние деньги), а
// неоплаченный долг висит в ленте любого дня, пока гость не рассчитается (как
// «ждут оплаты» у кухни).
const salesFeedWhere = "(created_at >= ? AND created_at < ?) OR (paid_at >= ? AND paid_at < ?) OR (paid_at IS NULL AND voided_at IS NULL)"

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
	db.Where(salesFeedWhere, from, to, from, to).
		Order("created_at DESC").Limit(salesFeedLimit).Find(&sales)

	// З8-и4: итоги — по всему окну одним запросом, а не по строкам списка: в
	// загруженный день новая строка вытесняла из трёхсот старую оплаченную, и
	// «продажи смены» занижались. В выручку идёт только оплаченное в окне
	// (продано сегодня, рассчитано в другие сутки — деньги не этой смены),
	// кухня кошельком — отдельно (Г7/Р7: не выручка).
	var sum struct {
		N, Voided, Unpaid        int64
		Total, Wallet, UnpaidPln float64
	}
	paid := "voided_at IS NULL AND paid_at >= ? AND paid_at < ?"
	db.Model(&models.Sale{}).
		Select("COUNT(*) AS n, COUNT(*) FILTER (WHERE voided_at IS NOT NULL) AS voided, "+
			"COUNT(*) FILTER (WHERE voided_at IS NULL AND paid_at IS NULL) AS unpaid, "+
			"COALESCE(SUM(total_pln) FILTER (WHERE voided_at IS NULL AND paid_at IS NULL), 0) AS unpaid_pln, "+
			"COALESCE(SUM(total_pln) FILTER (WHERE "+paid+" AND method <> 'wallet'), 0) AS total, "+
			"COALESCE(SUM(total_pln) FILTER (WHERE "+paid+" AND method = 'wallet'), 0) AS wallet", from, to, from, to).
		Where(salesFeedWhere, from, to, from, to).Scan(&sum)

	ids := make([]string, 0, len(sales)*2)
	for _, s := range sales {
		ids = append(ids, s.CreatedBy.String())
		if s.UserID != nil {
			ids = append(ids, s.UserID.String())
		}
	}
	nick := nicknamesByID(ids)
	out := make([]gin.H, 0, len(sales))
	for i := range sales {
		out = append(out, saleOut(&sales[i], nick))
	}
	c.JSON(http.StatusOK, gin.H{"date": key, "count": len(out), "total_count": sum.N, "sales": out,
		"total_pln": roundPLN(sum.Total), "wallet_pln": roundPLN(sum.Wallet), "voided": sum.Voided,
		"unpaid": sum.Unpaid, "unpaid_pln": roundPLN(sum.UnpaidPln)})
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

// voidSaleTx — отмена одной продажи внутри транзакции: CAS по voided_at,
// возврат на склад, возврат в кошелёк для «кошельком». Продажа в долг (Ж6)
// без paid_at — денег не брали: только склад, долг гостя уменьшается сам.
func voidSaleTx(tx *gorm.DB, sale *models.Sale, adminID uuid.UUID, now time.Time) error {
	// CAS по voided_at: проверка снаружи вне транзакции, и двойной клик по
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
	if sale.Method == "wallet" && sale.UserID != nil && sale.PaidAt != nil {
		saleID := sale.ID
		if _, err := walletApply(tx, walletOp{
			UserID: *sale.UserID, Kind: models.WalletTxRefund,
			Amount: models.GroszFromPLN(sale.TotalPLN), RefType: "sale", RefID: &saleID,
			Note: "возврат: отмена продажи " + sale.Name, CreatedBy: &adminID,
		}); err != nil {
			return err
		}
	}
	// З6-и1 (№14, З-Р11): продажа — расчёт заказа кухни. Товар заказ списал со
	// склада при заказе, а съеденное назад не вернуть: склад НЕ трогаем (иначе
	// инвентаризация даст фантомную «потерю»). Отменена оплата, а не заказ —
	// сбрасываем её, и заказ снова «ждёт оплаты»: он в долге гостя, и его можно
	// рассчитать заново правильным способом. Кошелёк гостю вернули выше.
	res = tx.Model(&models.KitchenOrder{}).Where("sale_id = ?", sale.ID).
		Updates(map[string]any{"paid_at": nil, "pay_method": nil, "paid_by": nil, "sale_id": nil})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
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
		Reason: "void", Note: "отмена продажи", SaleID: &saleID, CreatedBy: &adminID, PricePLN: sale.PricePLN}).Error
}

// isKitchenSale — продажа рассчитала заказ кухни (З6-и1): её отмена не
// возвращает товар, а снова делает заказ неоплаченным.
func isKitchenSale(saleID uuid.UUID) bool {
	var n int64
	db.Model(&models.KitchenOrder{}).Where("sale_id = ?", saleID).Count(&n)
	return n > 0
}

// saleVoidAllowed — права на отмену (canVoidSale) с ответом 403; вернул false — выходить.
func saleVoidAllowed(c *gin.Context, sale *models.Sale) bool {
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	if ok, code := canVoidSale(c.GetString("user_role"), sale.CreatedBy.String(),
		c.GetString("user_id"), sale.CreatedAt, from, to); !ok {
		goodFail(c, http.StatusForbidden, code)
		return false
	}
	return true
}

// POST /admin/sales/:id/void — отменить продажу (одну позицию), вернуть товар на склад.
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
	if !saleVoidAllowed(c, &sale) {
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	kitchen := isKitchenSale(sale.ID) // З6-и1: до отмены — после неё связь с заказом сброшена
	err := db.Transaction(func(tx *gorm.DB) error { return voidSaleTx(tx, &sale, adminID, now) })
	if errors.Is(err, errAlreadyVoid) {
		goodFail(c, http.StatusConflict, "already_void")
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	payload := map[string]any{"kind": "void", "name": sale.Name, "total_pln": sale.TotalPLN}
	if sale.PaidAt == nil {
		payload["unpaid"] = true // долг списан вместе с товаром — денег не возвращаем
	}
	if kitchen {
		payload["kitchen_unpaid"] = true // заказ кухни снова ждёт оплаты, склад не тронут
	}
	hub.AdminBroadcast("sale", payload)
	c.JSON(http.StatusOK, gin.H{"voided": sale.ID, "unpaid": sale.PaidAt == nil, "kitchen_unpaid": kitchen})
}

// POST /admin/receipts/:id/void — Ж6-и2: отменить чек целиком — все живые
// позиции одной транзакцией (права — по каждой, как у одиночной отмены;
// уже отменённые позиции пропускаются, а не ломают остальные).
func handleAdminReceiptVoid(c *gin.Context) {
	var sales []models.Sale
	db.Where("receipt_id = ?", c.Param("id")).Order("created_at").Find(&sales)
	if len(sales) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "receipt_not_found", "message": "Чек не найден"})
		return
	}
	live := make([]*models.Sale, 0, len(sales))
	for i := range sales {
		if sales[i].VoidedAt == nil {
			live = append(live, &sales[i])
		}
	}
	if len(live) == 0 {
		goodFail(c, http.StatusConflict, "already_void")
		return
	}
	for _, s := range live {
		if !saleVoidAllowed(c, s) {
			return
		}
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, s := range live {
			if err := voidSaleTx(tx, s, adminID, now); err != nil && !errors.Is(err, errAlreadyVoid) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	total := 0.0
	names := make([]string, 0, len(live))
	ids := make([]uuid.UUID, 0, len(live))
	for _, s := range live {
		total += s.TotalPLN
		names = append(names, receiptItemName(s))
		ids = append(ids, s.ID)
	}
	hub.AdminBroadcast("sale", map[string]any{"kind": "void", "name": "чек: " + strings.Join(names, ", "), "total_pln": roundPLN(total), "items": len(live)})
	c.JSON(http.StatusOK, gin.H{"receipt_id": c.Param("id"), "voided": ids, "total_pln": roundPLN(total)})
}
