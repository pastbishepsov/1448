package main

// Кухня для гостя (трек Г, спринт Г7; миграция 038).
//
// Конструктор ценника — В2 (goods.go): владелец ведёт позиции и остатки. Тут —
// гостевая сторона: меню (только включённые позиции, фото, стоп-лист по
// складу), заказ «одна позиция × количество» (как продажи В2 — без корзины),
// очередь для админа (принял → готовит → несут → выдан) и отмены.
//
// Деньги (Р7): wallet-заказ списывается с кошелька СРАЗУ (kind=kitchen_spend,
// это погашение обязательства, НЕ выручка); «оплачу у стойки» — выручка в
// момент выдачи, обычная продажа cash/card/blik. Выдача любого заказа создаёт
// строку sales (wallet-заказ — method='wallet', отчёты исключают его из
// выручки). Склад резервируется при заказе (stock_move reason='order'),
// отмена возвращает (reason='void') — владелец видит каждый шаг.
//
// Ачивки (Р5): еда даёт XP, никогда не кейс. kitchen_orders в user_progress
// растёт при ВЫДАЧЕ (done), не при заказе — отмена не фармит «Подкрепился».

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxOrderQty    = 5         // за раз; больше — это уже стойка и разговор с админом
	maxOpenOrders  = 3         // открытых заказов на гостя: анти-спам кухни
	maxPhotoBytes  = 500 << 10 // фото позиции ≤500КБ (клиент жмёт до ~512px)
	kitchenRefType = "kitchen" // ref_type wallet-транзакций заказа
)

var kitchenOpenStatuses = []string{models.KitchenNew, models.KitchenAccepted, models.KitchenPreparing, models.KitchenDelivering}

// allowedKitchenNext — допустимые переходы статуса заказа (чистая, тест).
// Отмена идёт отдельным путём (cancelKitchenOrder), здесь только «вперёд»:
// у стойки заказ могут принять и сразу выдать (accepted → done).
func allowedKitchenNext(cur, next string) bool {
	switch next {
	case models.KitchenAccepted:
		return cur == models.KitchenNew
	case models.KitchenPreparing:
		return cur == models.KitchenNew || cur == models.KitchenAccepted
	case models.KitchenDelivering:
		return cur == models.KitchenNew || cur == models.KitchenAccepted || cur == models.KitchenPreparing
	case models.KitchenDone:
		return cur == models.KitchenNew || cur == models.KitchenAccepted ||
			cur == models.KitchenPreparing || cur == models.KitchenDelivering
	}
	return false
}

func goodPhotoURL(g *models.Good) string {
	if g.PhotoAt == nil {
		return ""
	}
	return "/api/v1/goods/" + g.ID.String() + "/photo?v=" + fmt.Sprint(g.PhotoAt.Unix())
}

// GET /me/menu — меню кухни для гостя: только включённые позиции; закончилось —
// показываем с пометкой out (честный стоп-лист вместо тихого исчезновения).
func handleGetMyMenu(c *gin.Context) {
	var goods []models.Good
	db.Where("active = ?", true).Order("sort, category, name").Find(&goods)
	out := make([]gin.H, 0, len(goods))
	cats := make([]string, 0, 4)
	seen := map[string]bool{}
	for i := range goods {
		g := &goods[i]
		out = append(out, gin.H{
			"id": g.ID, "name": g.Name, "category": g.Category,
			"description": g.Description, "price_pln": g.PricePLN,
			"photo": goodPhotoURL(g), "out": g.Stock <= 0,
		})
		if !seen[g.Category] {
			seen[g.Category] = true
			cats = append(cats, g.Category)
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "menu": out, "categories": cats,
		"max_qty": maxOrderQty, "max_open": maxOpenOrders})
}

type kitchenOrderRequest struct {
	GoodID string `json:"good_id" binding:"required"`
	Qty    int    `json:"qty"`
	Pay    string `json:"pay"` // wallet | counter
}

func kitchenOrderOut(o *models.KitchenOrder, compName string) gin.H {
	row := gin.H{
		"id": o.ID, "name": o.Name, "qty": o.Qty, "price_pln": o.PricePLN,
		"total_pln": o.TotalPLN, "paid": o.Paid, "status": o.Status,
		"created_at": o.CreatedAt, "status_at": o.StatusAt,
		"can_cancel": o.Status == models.KitchenNew,
	}
	if compName != "" {
		row["computer"] = compName
	}
	return row
}

// POST /me/kitchen/orders — заказ гостя. Кошельком — списание сразу; склад
// резервируется в той же транзакции (CAS по остатку — две кассы не уведут в
// минус). Если у гостя идёт сессия, заказ несём к его ПК.
func handleMyKitchenOrderCreate(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var req kitchenOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Qty > maxOrderQty {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_qty",
			"message": fmt.Sprintf("За раз — до %d шт; больше закажи у стойки", maxOrderQty)})
		return
	}
	if req.Pay != models.KitchenPaidWallet && req.Pay != models.KitchenPaidCounter {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_pay", "message": "Оплата: кошельком или у стойки"})
		return
	}

	var open int64
	db.Model(&models.KitchenOrder{}).
		Where("user_id = ? AND status IN ?", userID, kitchenOpenStatuses).Count(&open)
	if open >= maxOpenOrders {
		c.JSON(http.StatusConflict, gin.H{"code": "too_many_orders",
			"message": fmt.Sprintf("У тебя уже %d заказа в работе — дождись их", open)})
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
			"message": fmt.Sprintf("«%s»: осталось %d шт", g.Name, g.Stock)})
		return
	}

	// сессия идёт → несём к ПК гостя
	var compID *uuid.UUID
	var compName string
	var sess models.Session
	if err := db.First(&sess, "user_id = ? AND status = ?", userID, models.SessionStatusActive).Error; err == nil {
		id := sess.ComputerID
		compID = &id
		var comp models.Computer
		if db.First(&comp, "id = ?", id).Error == nil {
			compName = comp.Name
		}
	}

	goodID := g.ID
	order := models.KitchenOrder{
		ClubID: g.ClubID, UserID: userID, GoodID: &goodID, Name: g.Name,
		Qty: req.Qty, PricePLN: g.PricePLN, TotalPLN: roundPLN(g.PricePLN * float64(req.Qty)),
		Paid: req.Pay, Status: models.KitchenNew, ComputerID: compID, StatusAt: time.Now(),
	}
	totalGrosz := models.GroszFromPLN(order.TotalPLN)
	err = db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Good{}).Where("id = ? AND stock >= ?", g.ID, req.Qty).
			UpdateColumn("stock", gorm.Expr("stock - ?", req.Qty))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errOutOfStock
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		orderID := order.ID
		if err := tx.Create(&models.StockMove{ClubID: g.ClubID, GoodID: g.ID, Delta: -req.Qty,
			Reason: "order", Note: "заказ кухни"}).Error; err != nil {
			return err
		}
		if req.Pay == models.KitchenPaidWallet {
			_, err := walletApply(tx, walletOp{
				UserID: userID, Kind: models.WalletTxKitchenSpend, Amount: -totalGrosz,
				RefType: kitchenRefType, RefID: &orderID,
				Note: fmt.Sprintf("кухня: %s ×%d", order.Name, order.Qty),
			})
			return err
		}
		return nil
	})
	if err == errOutOfStock {
		goodFail(c, http.StatusConflict, "out_of_stock")
		return
	}
	if err == errWalletInsufficient {
		c.JSON(http.StatusConflict, gin.H{"code": "wallet_low",
			"message": fmt.Sprintf("В кошельке не хватает: заказ на %.2f zł. Пополни у стойки или выбери «оплачу у стойки»", order.TotalPLN)})
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	// live-сигнал админке: новый заказ в очереди
	nick := nicknamesByID([]string{userID.String()})
	hub.AdminBroadcast("kitchen", map[string]any{
		"kind": "new", "name": order.Name, "qty": order.Qty, "total_pln": order.TotalPLN,
		"paid": order.Paid, "nickname": nick[userID.String()], "computer": compName,
	})
	c.JSON(http.StatusCreated, gin.H{"order": kitchenOrderOut(&order, compName)})
}

// GET /me/kitchen/orders — мои заказы: открытые + закрытые за последние сутки.
func handleGetMyKitchenOrders(c *gin.Context) {
	userID := c.GetString("user_id")
	var orders []models.KitchenOrder
	db.Where("user_id = ? AND (status IN ? OR created_at > ?)",
		userID, kitchenOpenStatuses, time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").Limit(20).Find(&orders)
	out := make([]gin.H, 0, len(orders))
	for i := range orders {
		out = append(out, kitchenOrderOut(&orders[i], computerNameByID(orders[i].ComputerID)))
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "orders": out})
}

func computerNameByID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	var comp models.Computer
	if db.First(&comp, "id = ?", *id).Error != nil {
		return ""
	}
	return comp.Name
}

// cancelKitchenOrder — отменить заказ: статус (CAS), возврат склада, возврат
// денег кошельку (если платили им). guestOnly — гостю можно только пока new.
func cancelKitchenOrder(o *models.KitchenOrder, byGuest bool, adminID *uuid.UUID) (string, bool) {
	allowed := kitchenOpenStatuses
	if byGuest {
		allowed = []string{models.KitchenNew}
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.KitchenOrder{}).
			Where("id = ? AND status IN ?", o.ID, allowed).
			Updates(map[string]any{"status": models.KitchenCancelled, "status_at": time.Now(), "status_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errKitchenState
		}
		if o.GoodID != nil { // вернуть на склад (позицию могли удалить)
			if err := tx.Model(&models.Good{}).Where("id = ?", *o.GoodID).
				UpdateColumn("stock", gorm.Expr("stock + ?", o.Qty)).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.StockMove{ClubID: o.ClubID, GoodID: *o.GoodID, Delta: o.Qty,
				Reason: "void", Note: "отмена заказа кухни", CreatedBy: adminID}).Error; err != nil {
				return err
			}
		}
		if o.Paid == models.KitchenPaidWallet {
			orderID := o.ID
			_, err := walletApply(tx, walletOp{
				UserID: o.UserID, Kind: models.WalletTxRefund, Amount: models.GroszFromPLN(o.TotalPLN),
				RefType: kitchenRefType, RefID: &orderID,
				Note: fmt.Sprintf("возврат: %s ×%d", o.Name, o.Qty), CreatedBy: adminID,
			})
			return err
		}
		return nil
	})
	if err == errKitchenState {
		return "kitchen_state", false
	}
	if err != nil {
		return "db_error", false
	}
	return "", true
}

var errKitchenState = fmt.Errorf("kitchen_state")

// POST /me/kitchen/orders/:id/cancel — гость передумал (только пока заказ new).
func handleMyKitchenOrderCancel(c *gin.Context) {
	userID := c.GetString("user_id")
	var o models.KitchenOrder
	if err := db.First(&o, "id = ? AND user_id = ?", c.Param("id"), userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	if code, ok := cancelKitchenOrder(&o, true, nil); !ok {
		msg := "Заказ уже готовят — отменить может админ у стойки"
		if code == "db_error" {
			msg = "Не получилось, попробуй ещё раз"
		}
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
		return
	}
	hub.AdminBroadcast("kitchen", map[string]any{"kind": "cancel", "name": o.Name})
	c.JSON(http.StatusOK, gin.H{"cancelled": o.ID})
}

// ── Админская сторона ─────────────────────────────────────────────────

// GET /admin/kitchen/orders — очередь кухни: все открытые + закрытые за
// текущие клубные сутки (окно report_hour, как продажи).
func handleAdminKitchenOrders(c *gin.Context) {
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	var orders []models.KitchenOrder
	db.Where("status IN ? OR (created_at >= ? AND created_at < ?)", kitchenOpenStatuses, from, to).
		Order("created_at").Limit(200).Find(&orders)

	ids := make([]string, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.UserID.String())
	}
	nick := nicknamesByID(ids)
	open := 0
	out := make([]gin.H, 0, len(orders))
	for i := range orders {
		o := &orders[i]
		row := kitchenOrderOut(o, computerNameByID(o.ComputerID))
		row["nickname"] = nick[o.UserID.String()]
		out = append(out, row)
		for _, s := range kitchenOpenStatuses {
			if o.Status == s {
				open++
				break
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "open": open, "orders": out})
}

type kitchenStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Method string `json:"method"` // для выдачи counter-заказа: cash | card | blik
}

// POST /admin/kitchen/orders/:id/status — двинуть заказ по потоку (staff).
// Выдача (done) создаёт продажу: wallet-заказ — method='wallet' (не выручка,
// Р7), «у стойки» — cash/card/blik (выручка, как продажа В2). Склад не
// трогаем — он списан при заказе.
func handleAdminKitchenStatus(c *gin.Context) {
	var o models.KitchenOrder
	if err := db.First(&o, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	var req kitchenStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if !allowedKitchenNext(o.Status, req.Status) {
		c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state",
			"message": fmt.Sprintf("Из «%s» в «%s» нельзя", o.Status, req.Status)})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))

	if req.Status != models.KitchenDone {
		res := db.Model(&models.KitchenOrder{}).
			Where("id = ? AND status = ?", o.ID, o.Status).
			Updates(map[string]any{"status": req.Status, "status_at": time.Now(), "status_by": adminID})
		if res.Error != nil || res.RowsAffected == 0 {
			c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state", "message": "Заказ уже двинули с другой кассы"})
			return
		}
		notifyUser(o.UserID, "kitchen_status", map[string]any{
			"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": req.Status, "computer": computerNameByID(o.ComputerID)})
		c.JSON(http.StatusOK, gin.H{"status": req.Status})
		return
	}

	// Выдача: продажа + прогресс ачивок
	method := "wallet"
	if o.Paid == models.KitchenPaidCounter {
		method = req.Method
		if method != "cash" && method != "card" && method != "blik" {
			goodFail(c, http.StatusBadRequest, "bad_method")
			return
		}
	}
	sale := models.Sale{
		ClubID: o.ClubID, GoodID: o.GoodID, Name: o.Name, Qty: o.Qty,
		PricePLN: o.PricePLN, TotalPLN: o.TotalPLN, Method: method,
		UserID: &o.UserID, CreatedBy: adminID,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.KitchenOrder{}).
			Where("id = ? AND status = ?", o.ID, o.Status).
			Updates(map[string]any{"status": models.KitchenDone, "status_at": time.Now(), "status_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errKitchenState
		}
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}
		return tx.Model(&models.KitchenOrder{}).Where("id = ?", o.ID).
			UpdateColumn("sale_id", sale.ID).Error
	})
	if err == errKitchenState {
		c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state", "message": "Заказ уже двинули с другой кассы"})
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	kitchenProgress(o.UserID) // «Подкрепился»/«Первый заказ» — при выдаче (Р5, анти-фарм отменами)
	notifyUser(o.UserID, "kitchen_status", map[string]any{
		"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": models.KitchenDone,
		"computer": computerNameByID(o.ComputerID)})
	if method != "wallet" {
		hub.AdminBroadcast("sale", map[string]any{"name": sale.Name, "qty": sale.Qty,
			"total_pln": sale.TotalPLN, "method": method})
	}
	logAdminAction(c, "kitchen_done", &o.UserID, fmt.Sprintf("%s ×%d · %.2f zł · %s", o.Name, o.Qty, o.TotalPLN,
		map[string]string{"wallet": "кошельком", "cash": "нал", "card": "карта", "blik": "BLIK"}[method]))
	c.JSON(http.StatusOK, gin.H{"status": models.KitchenDone, "sale_id": sale.ID})
}

// POST /admin/kitchen/orders/:id/cancel — отмена админом: деньги в кошелёк
// (если платили им), товар на склад, гостю уведомление.
func handleAdminKitchenCancel(c *gin.Context) {
	var o models.KitchenOrder
	if err := db.First(&o, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	if code, ok := cancelKitchenOrder(&o, false, &adminID); !ok {
		msg := "Заказ уже закрыт"
		if code == "db_error" {
			msg = "Не получилось, попробуй ещё раз"
		}
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
		return
	}
	refund := ""
	if o.Paid == models.KitchenPaidWallet {
		refund = fmt.Sprintf(" · %.2f zł вернулись в кошелёк", o.TotalPLN)
	}
	notifyUser(o.UserID, "kitchen_status", map[string]any{
		"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": models.KitchenCancelled,
		"refund_pln": map[bool]float64{true: o.TotalPLN, false: 0}[o.Paid == models.KitchenPaidWallet]})
	logAdminAction(c, "kitchen_cancel", &o.UserID, fmt.Sprintf("%s ×%d%s", o.Name, o.Qty, refund))
	c.JSON(http.StatusOK, gin.H{"cancelled": o.ID})
}

// kitchenProgress — выданный заказ в суточный прогресс + проверка ачивок
// («Подкрепился» daily, «Первый заказ» lifetime — Р5: XP, не кейс).
func kitchenProgress(userID uuid.UUID) {
	now := time.Now()
	db.Exec(`INSERT INTO user_progress (user_id, day_key, kitchen_orders)
		VALUES (?, ?, 1)
		ON CONFLICT (user_id, day_key) DO UPDATE SET
			kitchen_orders = user_progress.kitchen_orders + 1`,
		userID, achDayKey(now))
	stats := gatherPeriodicStats(userID, now)
	stats.HoursPlayed = userHoursPlayed(userID.String())
	stats.LoginCount = 1
	stats.DepositCount = userDepositCount(userID.String())
	stats.BookingsCount = userBookingsCount(userID.String())
	checkAchievements(userID, stats)
}

// ── Фото позиции (Г7): BYTEA в БД, байты живут только в этих трёх ручках ──

// GET /goods/:id/photo — публичная отдача фото (как статика обложек).
func handleGoodPhoto(c *gin.Context) {
	var row struct {
		Photo     []byte
		PhotoType *string
		PhotoAt   *time.Time
	}
	if err := db.Raw("SELECT photo, photo_type, photo_at FROM goods WHERE id = ?", c.Param("id")).
		Scan(&row).Error; err != nil || len(row.Photo) == 0 || row.PhotoAt == nil {
		c.Status(http.StatusNotFound)
		return
	}
	etag := fmt.Sprintf(`"g%d"`, row.PhotoAt.Unix())
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	ct := "image/jpeg"
	if row.PhotoType != nil && *row.PhotoType != "" {
		ct = *row.PhotoType
	}
	c.Data(http.StatusOK, ct, row.Photo)
}

// PUT /admin/goods/:id/photo — владелец кладёт фото (сырое тело запроса).
func handleAdminGoodPhotoPut(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	ct := c.ContentType()
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_photo", "message": "Фото — JPEG, PNG или WebP"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPhotoBytes+1))
	if err != nil || len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_photo", "message": "Пустое тело запроса"})
		return
	}
	if len(data) > maxPhotoBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "photo_too_big",
			"message": fmt.Sprintf("Фото — до %d КБ (сожми на клиенте)", maxPhotoBytes>>10)})
		return
	}
	if err := db.Exec("UPDATE goods SET photo = ?, photo_type = ?, photo_at = now() WHERE id = ?",
		data, ct, g.ID).Error; err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	db.First(&g, "id = ?", g.ID)
	logAdminAction(c, "good_update", nil, g.Name+": фото обновлено")
	c.JSON(http.StatusOK, gin.H{"photo": goodPhotoURL(&g)})
}

// DELETE /admin/goods/:id/photo — убрать фото.
func handleAdminGoodPhotoDelete(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	if err := db.Exec("UPDATE goods SET photo = NULL, photo_type = NULL, photo_at = NULL WHERE id = ?", g.ID).Error; err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "good_update", nil, g.Name+": фото убрано")
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
