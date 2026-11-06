package main

// Кухня для гостя (трек Г, спринт Г7; миграции 038/039).
//
// Конструктор ценника — В2 (goods.go): владелец ведёт позиции и остатки. Тут —
// гостевая сторона: меню (только включённые позиции, фото, стоп-лист по
// складу), заказ «одна позиция × количество» (как продажи В2 — без корзины),
// очередь для админа (принял → готовит → несут → выдан → оплачен) и отмены.
//
// Р10 (решение основателя 24.08): один вариант заказа — ПОСТОПЛАТА. Гость
// заказал из-за компа (нужна активная сессия), ему принесли, а рассчитывается
// он у стойки, когда закончил играть. Продажа (и выручка) появляется в момент
// РАСЧЁТА: нал/карта/BLIK — выручка как В2, кошелёк — method='wallet'
// (погашение обязательства, НЕ выручка — Р7). Выданный неоплаченный заказ
// висит в «ждут оплаты» и в карточке гостя; при завершении сессии — гостю
// напоминание, админам сигнал. Склад резервируется при заказе (stock_move
// reason='order'), отмена возвращает (reason='void').
//
// Ачивки (Р5): еда даёт XP, никогда не кейс. kitchen_orders в user_progress
// растёт при ВЫДАЧЕ (done), не при заказе — отмена не фармит «Подкрепился».

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	return "/api/v1/goods/" + g.ID.String() + "/photo?v=" + imageVersion(*g.PhotoAt) // З18-и3: версия — в миллисекундах
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
}

func kitchenOrderOut(o *models.KitchenOrder, compName string) gin.H {
	row := gin.H{
		"id": o.ID, "name": o.Name, "qty": o.Qty, "price_pln": o.PricePLN,
		"total_pln": o.TotalPLN, "status": o.Status,
		"created_at": o.CreatedAt, "status_at": o.StatusAt,
		"can_cancel": o.Status == models.KitchenNew,
		// Р10: выдан, но ещё не рассчитан у стойки; З11-и4: списанный владельцем — уже не ждёт оплаты
		"unpaid":      o.Status == models.KitchenDone && o.PaidAt == nil && o.WriteOffID == nil,
		"written_off": o.WriteOffID != nil,
	}
	if o.PayMethod != nil {
		row["pay_method"] = *o.PayMethod
	}
	if compName != "" {
		row["computer"] = compName
	}
	return row
}

// unpaidKitchen — выданные и не рассчитанные заказы гостя (Р10): их видно в
// карточке гостя, в «ждут оплаты» и в напоминании при завершении сессии.
// З11-и4: списанные — нет.
func unpaidKitchen(userID uuid.UUID) (int64, float64) {
	var agg struct {
		Cnt int64
		Sum float64
	}
	db.Model(&models.KitchenOrder{}).
		Select("COUNT(*) AS cnt, COALESCE(SUM(total_pln),0) AS sum").
		Where("user_id = ? AND "+kitchenDebtWhere, userID).
		Scan(&agg)
	return agg.Cnt, roundPLN(agg.Sum)
}

// POST /me/kitchen/orders — заказ гостя (Р10: постоплата). Нужна активная
// сессия — заказ «принесём к ПК», расчёт у стойки после игры; без сессии еду
// продают обычной продажей В2. Склад резервируется в транзакции (CAS по
// остатку — две кассы не уведут в минус).
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
	if rejectBanned(c, userID) { // К0-и4: постоплата забаненному не выдаётся
		return
	}

	// Р10: только из-за компа — постоплата привязана к живой сессии
	var sess models.Session
	if err := db.First(&sess, "user_id = ? AND status = ?", userID, models.SessionStatusActive).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "need_session",
			"message": "Заказ из-за компа: начни сессию — принесём к твоему ПК. У стойки продадут и так"})
		return
	}
	compID := sess.ComputerID
	compName := computerNameByID(&compID)

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

	goodID := g.ID
	order := models.KitchenOrder{
		ClubID: g.ClubID, UserID: userID, GoodID: &goodID, Name: g.Name,
		Qty: req.Qty, PricePLN: g.PricePLN, TotalPLN: roundPLN(g.PricePLN * float64(req.Qty)),
		Paid: models.KitchenPaidPostpay, Status: models.KitchenNew, ComputerID: &compID, StatusAt: time.Now(),
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		orderID := order.ID // З10-и1: движение знает свой заказ
		err := stockMove(tx, stockOp{ClubID: g.ClubID, GoodID: g.ID, Delta: -req.Qty, Reason: "order",
			Note: "заказ кухни", OrderID: &orderID, Price: g.PricePLN})
		if errors.Is(err, errNegativeStock) {
			return errOutOfStock
		}
		return err
	})
	if err == errOutOfStock {
		goodFail(c, http.StatusConflict, "out_of_stock")
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
		"nickname": nick[userID.String()], "computer": compName,
	})
	c.JSON(http.StatusCreated, gin.H{"order": kitchenOrderOut(&order, compName)})
}

// GET /me/kitchen/orders — мои заказы: открытые, «ждут оплаты» (Р10 — висят,
// пока не рассчитаешься) и закрытые за последние сутки.
func handleGetMyKitchenOrders(c *gin.Context) {
	userID := c.GetString("user_id")
	uid, _ := uuid.Parse(userID)
	var orders []models.KitchenOrder
	db.Where("user_id = ? AND (status IN ? OR ("+kitchenDebtWhere+") OR created_at > ?)", // З11-и4: списанные не висят
		userID, kitchenOpenStatuses, time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").Limit(20).Find(&orders)
	out := make([]gin.H, 0, len(orders))
	for i := range orders {
		out = append(out, kitchenOrderOut(&orders[i], computerNameByID(orders[i].ComputerID)))
	}
	cnt, sum := unpaidKitchen(uid)
	// Ж6 (Ж-Р8): бар в долг — тот же «рассчитайся у стойки»; гость видит весь долг
	barCnt, barSum := unpaidBar(uid)
	c.JSON(http.StatusOK, gin.H{"count": len(out), "orders": out,
		"unpaid_count": cnt, "unpaid_pln": sum,
		"bar_unpaid_count": barCnt, "bar_unpaid_pln": barSum, "unpaid_total_pln": roundPLN(sum + barSum)})
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

// cancelKitchenOrder — отменить заказ: статус (CAS) + возврат склада. При
// постоплате (Р10) денег ещё не брали — расчёт только у стойки. З8-и6: заказ
// от стойки, оплаченный сразу (нал, карта, BLIK, кошелёк — Ж6-и3), отменяется
// вместе со своей продажей в той же транзакции: кошелёк возвращается гостю,
// выручка и ящик уменьшаются; refunded — отменённая продажа (nil — денег не было).
// guestOnly — гостю можно только пока new.
// refund — как вернули деньги оплаченного заказа (З11-и1, pickRefund): "" — как платили.
func cancelKitchenOrder(o *models.KitchenOrder, byGuest bool, adminID *uuid.UUID, refund string) (code string, ok bool, refunded *models.Sale) {
	allowed := kitchenOpenStatuses
	if byGuest {
		allowed = []string{models.KitchenNew}
	}
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.KitchenOrder{}).
			Where("id = ? AND status IN ?", o.ID, allowed).
			Updates(map[string]any{"status": models.KitchenCancelled, "status_at": now, "status_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errKitchenState
		}
		if o.SaleID != nil && adminID != nil {
			// продажа заказа: отмена возвращает кошелёк и сбрасывает оплату заказа,
			// склад не трогает — товар вернёт отмена заказа ниже, один раз
			var sale models.Sale
			if err := tx.First(&sale, "id = ?", *o.SaleID).Error; err != nil {
				return err
			}
			switch err := voidSaleTx(tx, &sale, *adminID, now, refund); {
			case err == nil:
				refunded = &sale
			case !errors.Is(err, errAlreadyVoid): // продажу уже отменили — деньги вернулись тогда
				return err
			}
		}
		if o.GoodID != nil { // вернуть на склад (позицию могли удалить)
			orderID := o.ID
			return stockMove(tx, stockOp{ClubID: o.ClubID, GoodID: *o.GoodID, Delta: o.Qty, Reason: "void",
				Note: "отмена заказа кухни", OrderID: &orderID, By: adminID, Price: o.PricePLN})
		}
		return nil
	})
	if err == errKitchenState {
		return "kitchen_state", false, nil
	}
	if err != nil {
		return "db_error", false, nil
	}
	return "", true, refunded
}

var errKitchenState = fmt.Errorf("kitchen_state")

// refundWhere — куда вернули деньги отменённого заказа (чистая, тест): строка
// журнала и тост стойки.
func refundWhere(method string) string {
	switch method {
	case "wallet":
		return "на кошелёк гостя"
	case "cash":
		return "наличными"
	case "card":
		return "на карту"
	case "blik":
		return "по BLIK"
	case refundNone: // З11-и1
		return "не возвращали"
	}
	return method
}

// kitchenRefundErrors — заказ оплачен продажей, которую этот админ отменить не
// может (canVoidSale): отмена с возвратом денег — владельцу (З8-и6).
var kitchenRefundErrors = map[string]string{
	"not_yours": "Заказ оплачен продажей другого админа — отменить с возвратом денег может владелец",
	"too_old":   "Заказ оплачен не в эту смену — отменить с возвратом денег может владелец",
}

// POST /me/kitchen/orders/:id/cancel — гость передумал (только пока заказ new).
func handleMyKitchenOrderCancel(c *gin.Context) {
	userID := c.GetString("user_id")
	var o models.KitchenOrder
	if err := db.First(&o, "id = ? AND user_id = ?", c.Param("id"), userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	if code, ok, _ := cancelKitchenOrder(&o, true, nil, ""); !ok {
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

// kitchenClosedLimit — сколько закрытых за сутки заказов (выданы и рассчитаны
// или отменены) отдаёт очередь кухни — последних; открытые и «ждут оплаты» —
// всегда все (З8-и5).
const kitchenClosedLimit = 200

// kitchenQueue — очередь кухни одним списком от старых к новым (чистая, тест):
// живые заказы (открытые и «ждут оплаты») и последние закрытые, пришедшие от
// новых к старым, встают по времени заказа.
func kitchenQueue(live, closed []models.KitchenOrder) []models.KitchenOrder {
	out := make([]models.KitchenOrder, 0, len(live)+len(closed))
	out = append(out, live...)
	out = append(out, closed...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// GET /admin/kitchen/orders — очередь кухни: все открытые, все «ждут оплаты»
// (Р10 — не тонут в смене, пока гость не рассчитался) + закрытые за текущие
// клубные сутки (окно report_hour, как продажи) — последние kitchenClosedLimit.
// З8-и5: раньше всё шло одним списком от старых к новым с обрезкой на 200 — в
// загруженный день только что пришедший заказ в очередь не попадал, а значок
// «в работе · ждут оплаты» считался по обрезанному списку.
func handleAdminKitchenOrders(c *gin.Context) {
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	var live, closed []models.KitchenOrder
	db.Where("status IN ? OR ("+kitchenDebtWhere+")", kitchenOpenStatuses).
		Order("created_at").Find(&live)
	closedQ := func() *gorm.DB { // З11-и4: списанный долг — закрыт, как оплаченный
		return db.Model(&models.KitchenOrder{}).
			Where("created_at >= ? AND created_at < ? AND (status = ? OR (status = ? AND (paid_at IS NOT NULL OR write_off_id IS NOT NULL)))",
				from, to, models.KitchenCancelled, models.KitchenDone)
	}
	var closedTotal int64
	closedQ().Count(&closedTotal)
	closedQ().Order("created_at DESC").Limit(kitchenClosedLimit).Find(&closed)
	orders := kitchenQueue(live, closed)

	ids := make([]string, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.UserID.String())
	}
	nick := nicknamesByID(ids)
	open, unpaid := 0, 0
	unpaidSum := 0.0
	out := make([]gin.H, 0, len(orders))
	for i := range orders {
		o := &orders[i]
		row := kitchenOrderOut(o, computerNameByID(o.ComputerID))
		row["nickname"] = nick[o.UserID.String()]
		row["user_id"] = o.UserID // Ж6-и1: «рассчитать всё» ведёт в карточку гостя
		out = append(out, row)
		if o.Status == models.KitchenDone && o.PaidAt == nil && o.WriteOffID == nil {
			unpaid++
			unpaidSum += o.TotalPLN
		}
		for _, s := range kitchenOpenStatuses {
			if o.Status == s {
				open++
				break
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "open": open, "orders": out,
		"unpaid": unpaid, "unpaid_pln": roundPLN(unpaidSum), "closed_total": closedTotal})
}

type kitchenStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// POST /admin/kitchen/orders/:id/status — двинуть заказ по потоку (staff).
// Выдача (done) кормит ачивки и оставляет заказ «ждёт оплаты» (Р10): продажа
// и деньги появятся при расчёте (ручка /pay). Склад не трогаем — списан при
// заказе.
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
	res := db.Model(&models.KitchenOrder{}).
		Where("id = ? AND status = ?", o.ID, o.Status).
		Updates(map[string]any{"status": req.Status, "status_at": time.Now(), "status_by": adminID})
	if res.Error != nil || res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state", "message": "Заказ уже двинули с другой кассы"})
		return
	}
	notifyUser(o.UserID, "kitchen_status", map[string]any{
		"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": req.Status,
		"computer": computerNameByID(o.ComputerID)})
	if req.Status == models.KitchenDone {
		kitchenProgress(o.UserID) // «Подкрепился»/«Первый заказ» — при выдаче (Р5, анти-фарм отменами)
		logAdminAction(c, "kitchen_done", &o.UserID,
			fmt.Sprintf("%s ×%d · %.2f zł · ждёт оплаты", o.Name, o.Qty, o.TotalPLN))
	}
	c.JSON(http.StatusOK, gin.H{"status": req.Status})
}

type kitchenPayRequest struct {
	Method string `json:"method" binding:"required"` // cash | card | blik | wallet
}

// POST /admin/kitchen/orders/:id/pay — расчёт у стойки (Р10): гость доиграл
// и платит. Продажа появляется здесь: нал/карта/BLIK — выручка (как В2),
// кошелёк — списание kitchen_spend и method='wallet' (НЕ выручка, Р7).
func handleAdminKitchenPay(c *gin.Context) {
	var o models.KitchenOrder
	if err := db.First(&o, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	if o.Status != models.KitchenDone {
		c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state", "message": "Сначала выдай заказ — расчёт после"})
		return
	}
	if o.PaidAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "already_paid", "message": "Заказ уже оплачен"})
		return
	}
	if o.WriteOffID != nil { // З11-и4
		c.JSON(http.StatusConflict, gin.H{"code": "written_off", "message": writtenOffMsg})
		return
	}
	var req kitchenPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Method != "cash" && req.Method != "card" && req.Method != "blik" && req.Method != "wallet" {
		goodFail(c, http.StatusBadRequest, "bad_method")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	sale := models.Sale{
		ClubID: o.ClubID, GoodID: o.GoodID, Name: o.Name, Qty: o.Qty,
		PricePLN: o.PricePLN, TotalPLN: o.TotalPLN, Method: req.Method,
		UserID: &o.UserID, CreatedBy: adminID,
		PaidAt: &now, Postpaid: true, // Ж6: момент денег — расчёт; заказ был в долг
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.KitchenOrder{}).
			Where("id = ? AND "+kitchenDebtWhere, o.ID).
			Updates(map[string]any{"pay_method": req.Method, "paid_at": now, "paid_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errKitchenState
		}
		if req.Method == "wallet" {
			orderID := o.ID
			if _, err := walletApply(tx, walletOp{
				UserID: o.UserID, Kind: models.WalletTxKitchenSpend,
				Amount: -models.GroszFromPLN(o.TotalPLN),
				RefType: kitchenRefType, RefID: &orderID,
				Note:      fmt.Sprintf("кухня: %s ×%d", o.Name, o.Qty),
				CreatedBy: &adminID,
			}); err != nil {
				return err
			}
		}
		if err := tx.Create(&sale).Error; err != nil {
			return err
		}
		return tx.Model(&models.KitchenOrder{}).Where("id = ?", o.ID).
			UpdateColumn("sale_id", sale.ID).Error
	})
	if err == errKitchenState {
		c.JSON(http.StatusConflict, gin.H{"code": "kitchen_state", "message": "Заказ уже оплатили с другой кассы"})
		return
	}
	if err == errWalletInsufficient {
		c.JSON(http.StatusConflict, gin.H{"code": "wallet_low",
			"message": fmt.Sprintf("В кошельке не хватает на %.2f zł — возьми налом/картой/BLIK", o.TotalPLN)})
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	notifyUser(o.UserID, "kitchen_status", map[string]any{
		"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": "paid", "method": req.Method})
	if req.Method != "wallet" {
		hub.AdminBroadcast("sale", map[string]any{"name": sale.Name, "qty": sale.Qty,
			"total_pln": sale.TotalPLN, "method": req.Method})
	}
	logAdminAction(c, "kitchen_pay", &o.UserID, fmt.Sprintf("%s ×%d · %.2f zł · %s", o.Name, o.Qty, o.TotalPLN,
		map[string]string{"wallet": "кошельком", "cash": "нал", "card": "карта", "blik": "BLIK"}[req.Method]))
	c.JSON(http.StatusOK, gin.H{"paid": true, "sale_id": sale.ID})
}

// POST /admin/kitchen/orders/:id/cancel — отмена админом: товар на склад,
// гостю уведомление. При постоплате денег ещё не брали (Р10); заказ, оплаченный
// сразу, отменяется вместе с продажей — права те же, что у отмены продажи (З8-и6).
func handleAdminKitchenCancel(c *gin.Context) {
	var o models.KitchenOrder
	if err := db.First(&o, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "order_not_found", "message": "Заказ не найден"})
		return
	}
	asked, ok := refundAsked(c) // З11-и1: как вернули деньги оплаченного заказа
	if !ok {
		return
	}
	paid := false
	refund := ""
	if o.SaleID != nil {
		var sale models.Sale
		if db.First(&sale, "id = ?", *o.SaleID).Error == nil && sale.VoidedAt == nil {
			from, to, _, _ := shiftWindow("", int(settingInt64("report_hour", 8)), time.Now())
			if ok, code := canVoidSale(c.GetString("user_role"), sale.CreatedBy.String(),
				c.GetString("user_id"), sale.CreatedAt, from, to); !ok {
				c.JSON(http.StatusForbidden, gin.H{"code": code, "message": kitchenRefundErrors[code]})
				return
			}
			paid = true
			r, code := pickRefund(salePaid(&sale), asked, refundByOwner(c))
			if code != "" {
				refundFail(c, code)
				return
			}
			refund = r
		}
	}
	if !paid && asked != "" && asked != refundNone { // постоплата: денег ещё не брали
		refundFail(c, "refund_unpaid")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	code, ok, refunded := cancelKitchenOrder(&o, false, &adminID, refund)
	if !ok {
		msg := "Заказ уже закрыт"
		if code == "db_error" {
			msg = "Не получилось, попробуй ещё раз"
		}
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
		return
	}
	notifyUser(o.UserID, "kitchen_status", map[string]any{
		"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": models.KitchenCancelled})
	what, out := fmt.Sprintf("%s ×%d", o.Name, o.Qty), gin.H{"cancelled": o.ID}
	if refunded != nil {
		how := refunded.Method
		if refunded.RefundMethod != nil { // З11-и1: как вернули на самом деле
			how = *refunded.RefundMethod
		}
		if how == refundNone {
			what += " · денег не возвращали"
			out["refund"] = gin.H{"method": how, "pln": 0}
		} else {
			what += fmt.Sprintf(" · возврат %.2f zł %s", refunded.TotalPLN, refundWhere(how))
			out["refund"] = gin.H{"method": how, "pln": refunded.TotalPLN}
		}
		hub.AdminBroadcast("sale", map[string]any{"kind": "void", "name": refunded.Name, "total_pln": refunded.TotalPLN, "refund_method": how})
	}
	logAdminAction(c, "kitchen_cancel", &o.UserID, what)
	c.JSON(http.StatusOK, out)
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
	checkAchievements(userID, fullPlayerStats(userID, now))
}

// ── Фото позиции (Г7): BYTEA в БД, байты живут только в этих трёх ручках ──

// GET /goods/:id/photo — публичная отдача фото (как статика обложек): вне лимитера, кэш на сутки по адресу с версией,
// 304 без чтения байтов (З18-и3, images.go).
func handleGoodPhoto(c *gin.Context) { photoImage.serve(c, c.Param("id")) }

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
