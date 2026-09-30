package main

// Ж6 «Касса» (ADMIN.md, трек Ж; решение Ж-Р8, ревью 14.09 №16–17):
// долг гостя = кухня (Р10 трека Г) + бар — одной строкой в карточке, в чеке
// сессии и в сигнале при завершении; закрывается одной операцией
// «Рассчитать» (нал/карта/BLIK) — и кухня, и бар сразу. Кошельком товары не
// платятся (решение основателя 15.09); кошелёк для одиночного заказа кухни
// остаётся в /admin/kitchen/orders/:id/pay, как было в Г7.
//
// Ж6-и3: заказ кухни от стойки — гость через общий резолвер, позиция меню,
// статус «принят»; постоплата по Р10 (только гостю за ПК) или сразу
// нал/карта/BLIK/кошелёк.

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

// Открытый долг — одно правило для карточки, расчёта, кухни, ленты продаж и
// отчётов: бар — продано в долг и не рассчитано; кухня — выдано «при выходе» и
// не рассчитано. З11-и4: списанное владельцем (write_off_id) — уже не долг.
const (
	barDebtWhere     = "paid_at IS NULL AND voided_at IS NULL AND write_off_id IS NULL"
	kitchenDebtWhere = "status = 'done' AND paid_at IS NULL AND write_off_id IS NULL"
)

// guestDebtInfo — что гость должен клубу прямо сейчас.
type guestDebtInfo struct {
	KitchenCount int64   `json:"kitchen_count"`
	KitchenPLN   float64 `json:"kitchen_pln"`
	BarCount     int64   `json:"bar_count"`
	BarPLN       float64 `json:"bar_pln"`
	TotalPLN     float64 `json:"total_pln"`
}

// unpaidBar — неоплаченные живые продажи гостя (Ж6: method=later без paid_at;
// З11-и4: и не списанные).
func unpaidBar(userID uuid.UUID) (int64, float64) {
	var agg struct {
		Cnt int64
		Sum float64
	}
	db.Model(&models.Sale{}).
		Select("COUNT(*) AS cnt, COALESCE(SUM(total_pln),0) AS sum").
		Where("user_id = ? AND "+barDebtWhere, userID).
		Scan(&agg)
	return agg.Cnt, roundPLN(agg.Sum)
}

// guestDebt — кухня (выдано, не рассчитано) + бар (продано в долг).
func guestDebt(userID uuid.UUID) guestDebtInfo {
	var d guestDebtInfo
	d.KitchenCount, d.KitchenPLN = unpaidKitchen(userID)
	d.BarCount, d.BarPLN = unpaidBar(userID)
	d.TotalPLN = roundPLN(d.KitchenPLN + d.BarPLN)
	return d
}

// settleResult — что закрыл расчёт.
type settleResult struct {
	Method    string
	Kitchen   []models.KitchenOrder
	Bar       []models.Sale
	Sales     []models.Sale // созданные продажи кухни (по одной на заказ)
	KitchenPL float64
	BarPLN    float64
	TotalPLN  float64
}

var errNoDebt = fmt.Errorf("no_debt")

// settleGuestTx — расчёт гостя одной транзакцией: бар — proставить paid_at и
// способ у продаж в долг (CAS по paid_at IS NULL); кухня — как в /pay: у
// выданного заказа pay_method/paid_at и продажа с этим же моментом денег.
// Ничего не должен — errNoDebt, ни одной строки не трогаем.
func settleGuestTx(tx *gorm.DB, userID, adminID uuid.UUID, method string, now time.Time) (*settleResult, error) {
	r := &settleResult{Method: method}
	if err := tx.Where("user_id = ? AND "+barDebtWhere, userID).
		Order("created_at").Find(&r.Bar).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("user_id = ? AND "+kitchenDebtWhere, userID).
		Order("created_at").Find(&r.Kitchen).Error; err != nil {
		return nil, err
	}
	if len(r.Bar) == 0 && len(r.Kitchen) == 0 {
		return nil, errNoDebt
	}
	for i := range r.Bar {
		s := &r.Bar[i]
		res := tx.Model(&models.Sale{}).
			Where("id = ? AND "+barDebtWhere, s.ID).
			Updates(map[string]any{"method": method, "paid_at": now, "paid_by": adminID})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 { // параллельно рассчитали, отменили или списали — не считаем
			continue
		}
		s.Method, s.PaidAt, s.PaidBy = method, &now, &adminID
		r.BarPLN += s.TotalPLN
	}
	for i := range r.Kitchen {
		o := &r.Kitchen[i]
		res := tx.Model(&models.KitchenOrder{}).
			Where("id = ? AND "+kitchenDebtWhere, o.ID).
			Updates(map[string]any{"pay_method": method, "paid_at": now, "paid_by": adminID})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			continue
		}
		paid := now
		sale := models.Sale{
			ClubID: o.ClubID, GoodID: o.GoodID, Name: o.Name, Qty: o.Qty,
			PricePLN: o.PricePLN, TotalPLN: o.TotalPLN, Method: method,
			UserID: &o.UserID, CreatedBy: adminID, PaidAt: &paid, Postpaid: true,
		}
		if err := tx.Create(&sale).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&models.KitchenOrder{}).Where("id = ?", o.ID).
			UpdateColumn("sale_id", sale.ID).Error; err != nil {
			return nil, err
		}
		r.Sales = append(r.Sales, sale)
		r.KitchenPL += o.TotalPLN
	}
	r.BarPLN, r.KitchenPL = roundPLN(r.BarPLN), roundPLN(r.KitchenPL)
	r.TotalPLN = roundPLN(r.BarPLN + r.KitchenPL)
	if r.TotalPLN == 0 {
		return nil, errNoDebt
	}
	return r, nil
}

// POST /admin/users/:id/settle {method} — «Рассчитать»: закрыть весь долг
// гостя (кухня + бар) одним способом оплаты. Выручка появляется здесь.
func handleAdminUserSettle(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
		return
	}
	var req struct {
		Method string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !saleMethodOK(req.Method) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_method", "message": "Расчёт: наличные, карта или BLIK"})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	var res *settleResult
	err := db.Transaction(func(tx *gorm.DB) error {
		r, err := settleGuestTx(tx, user.ID, adminID, req.Method, now)
		res = r
		return err
	})
	if err == errNoDebt {
		c.JSON(http.StatusConflict, gin.H{"code": "no_debt", "message": "Гость ничего не должен"})
		return
	}
	if err != nil {
		respondDBError(c, err)
		return
	}
	details := fmt.Sprintf("%.2f zł · %s", res.TotalPLN, payMethodRU(req.Method))
	parts := make([]string, 0, 2)
	if res.KitchenPL > 0 {
		parts = append(parts, fmt.Sprintf("кухня %.2f zł (%d)", res.KitchenPL, len(res.Kitchen)))
	}
	if res.BarPLN > 0 {
		parts = append(parts, fmt.Sprintf("бар %.2f zł (%d)", res.BarPLN, len(res.Bar)))
	}
	if len(parts) > 0 {
		details += " · " + strings.Join(parts, ", ")
	}
	target := user.ID
	logAdminAction(c, "settle", &target, details)
	for i := range res.Kitchen {
		o := &res.Kitchen[i]
		notifyUser(o.UserID, "kitchen_status", map[string]any{
			"order_id": o.ID, "name": o.Name, "qty": o.Qty, "status": "paid", "method": req.Method})
	}
	hub.AdminBroadcast("sale", map[string]any{"kind": "settle", "nickname": user.Nickname,
		"total_pln": res.TotalPLN, "method": req.Method, "kitchen_pln": res.KitchenPL, "bar_pln": res.BarPLN})
	c.JSON(http.StatusOK, gin.H{
		"paid_pln": res.TotalPLN, "method": req.Method, "nickname": user.Nickname,
		"kitchen": gin.H{"count": len(res.Kitchen), "pln": res.KitchenPL},
		"bar":     gin.H{"count": len(res.Bar), "pln": res.BarPLN},
		"debt":    guestDebt(user.ID), // после расчёта — ноль, если ничего не добавилось параллельно
	})
}

func payMethodRU(m string) string {
	switch m {
	case "cash":
		return "нал"
	case "card":
		return "карта"
	case "blik":
		return "BLIK"
	case "wallet":
		return "кошельком"
	case models.SaleMethodLater:
		return "в долг"
	}
	return m
}

// GET /admin/users/:id/debt — долг гостя со списком позиций (карточка: «что
// именно должен» перед расчётом).
func handleAdminUserDebt(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var bar []models.Sale
	db.Where("user_id = ? AND "+barDebtWhere, user.ID).Order("created_at").Find(&bar)
	var kitchen []models.KitchenOrder
	db.Where("user_id = ? AND "+kitchenDebtWhere, user.ID).Order("created_at").Find(&kitchen)
	items := make([]gin.H, 0, len(bar)+len(kitchen))
	for i := range kitchen {
		o := &kitchen[i]
		items = append(items, gin.H{"kind": "kitchen", "id": o.ID, "name": o.Name, "qty": o.Qty, "total_pln": o.TotalPLN, "created_at": o.CreatedAt})
	}
	for i := range bar {
		s := &bar[i]
		row := gin.H{"kind": "bar", "id": s.ID, "name": s.Name, "qty": s.Qty, "total_pln": s.TotalPLN, "created_at": s.CreatedAt}
		if s.ReceiptID != nil {
			row["receipt_id"] = s.ReceiptID
		}
		items = append(items, row)
	}
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "debt": guestDebt(user.ID), "items": items})
}

// POST /admin/kitchen/orders — Ж6-и3: заказ кухни от стойки.
//
// Гость — через общий резолвер (guest | guest_id | nickname), позиция меню и
// количество как у гостевого заказа; статус сразу «принят» (админ у стойки
// его и принял). Оплата: пусто или later — постоплата Р10 (только гостю за
// ПК: долг привязан к сессии, при выходе рассчитается); cash | card | blik |
// wallet — оплачен сразу, продажа (и выручка) появляется тут же.
func handleAdminKitchenOrderCreate(c *gin.Context) {
	var req struct {
		Nickname string `json:"nickname"`
		Guest    string `json:"guest"`
		GuestID  string `json:"guest_id"`
		GoodID   string `json:"good_id" binding:"required"`
		Qty      int    `json:"qty"`
		Method   string `json:"method"` // "" | later | cash | card | blik | wallet
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Qty > maxOrderQty {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_qty",
			"message": fmt.Sprintf("В одном заказе — до %d шт", maxOrderQty)})
		return
	}
	method := req.Method
	if method == "" {
		method = models.SaleMethodLater
	}
	if method != models.SaleMethodLater && method != "wallet" && !saleMethodOK(method) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_method", "message": "Оплата: при выходе (гостю за ПК), наличные, карта, BLIK или кошелёк"})
		return
	}
	guest := lookupGuestForAction(c, req.GuestID, req.Guest, req.Nickname) // 400/404/409 пишет сам
	if guest == nil {
		return
	}
	if ok, code := canTargetUser(guest.Role, guest.ID.String(), c.GetString("user_id")); !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": "Заказ кухни записывается только на гостя"})
		return
	}
	if guest.Status == models.UserStatusBanned {
		c.JSON(http.StatusConflict, gin.H{"code": "banned", "message": "Аккаунт заблокирован — сначала разбань"})
		return
	}
	var compID *uuid.UUID
	var sess models.Session
	if db.First(&sess, "user_id = ? AND status = ?", guest.ID, models.SessionStatusActive).Error == nil {
		cid := sess.ComputerID
		compID = &cid
	}
	if method == models.SaleMethodLater && compID == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "need_session",
			"message": "«При выходе» — только гостю за ПК; без сессии возьми оплату сразу: нал, карта, BLIK или кошелёк"})
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

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	goodID := g.ID
	order := models.KitchenOrder{
		ClubID: g.ClubID, UserID: guest.ID, GoodID: &goodID, Name: g.Name,
		Qty: req.Qty, PricePLN: g.PricePLN, TotalPLN: roundPLN(g.PricePLN * float64(req.Qty)),
		Paid: models.KitchenPaidPostpay, Status: models.KitchenAccepted, ComputerID: compID,
		StatusAt: now, StatusBy: &adminID,
	}
	var sale *models.Sale
	if method != models.SaleMethodLater {
		m := method
		order.PayMethod, order.PaidAt, order.PaidBy = &m, &now, &adminID
		paid := now
		sale = &models.Sale{
			ClubID: g.ClubID, GoodID: &goodID, Name: g.Name, Qty: req.Qty,
			PricePLN: g.PricePLN, TotalPLN: order.TotalPLN, Method: method,
			UserID: &guest.ID, CreatedBy: adminID, PaidAt: &paid,
		}
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		orderID := order.ID // З10-и1: движение знает свой заказ
		err := stockMove(tx, stockOp{ClubID: g.ClubID, GoodID: g.ID, Delta: -req.Qty, Reason: "order",
			Note: "заказ кухни от стойки", OrderID: &orderID, By: &adminID, Price: g.PricePLN})
		if errors.Is(err, errNegativeStock) {
			return errOutOfStock
		}
		if err != nil {
			return err
		}
		if sale == nil {
			return nil
		}
		if method == "wallet" {
			orderID := order.ID
			if _, err := walletApply(tx, walletOp{
				UserID: guest.ID, Kind: models.WalletTxKitchenSpend,
				Amount:  -models.GroszFromPLN(order.TotalPLN),
				RefType: kitchenRefType, RefID: &orderID,
				Note:      fmt.Sprintf("кухня: %s ×%d", order.Name, order.Qty),
				CreatedBy: &adminID,
			}); err != nil {
				return err
			}
		}
		if err := tx.Create(sale).Error; err != nil {
			return err
		}
		return tx.Model(&models.KitchenOrder{}).Where("id = ?", order.ID).UpdateColumn("sale_id", sale.ID).Error
	})
	if err == errOutOfStock {
		goodFail(c, http.StatusConflict, "out_of_stock")
		return
	}
	if err == errWalletInsufficient {
		c.JSON(http.StatusConflict, gin.H{"code": "wallet_low",
			"message": fmt.Sprintf("В кошельке не хватает на %.2f zł — возьми налом/картой/BLIK или при выходе", order.TotalPLN)})
		return
	}
	if err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}

	compName := computerNameByID(compID)
	notifyUser(guest.ID, "kitchen_status", map[string]any{
		"order_id": order.ID, "name": order.Name, "qty": order.Qty, "status": order.Status, "computer": compName})
	hub.AdminBroadcast("kitchen", map[string]any{
		"kind": "new", "name": order.Name, "qty": order.Qty, "total_pln": order.TotalPLN,
		"nickname": guest.Nickname, "computer": compName, "by_staff": true, "method": method,
	})
	if sale != nil && method != "wallet" {
		hub.AdminBroadcast("sale", map[string]any{"name": sale.Name, "qty": sale.Qty,
			"total_pln": sale.TotalPLN, "method": method, "nickname": guest.Nickname})
	}
	target := guest.ID
	logAdminAction(c, "kitchen_order", &target, fmt.Sprintf("%s ×%d · %.2f zł · %s%s", order.Name, order.Qty, order.TotalPLN,
		payMethodRU(method), map[bool]string{true: " · к " + compName, false: ""}[compName != ""]))
	row := kitchenOrderOut(&order, compName)
	row["nickname"] = guest.Nickname
	resp := gin.H{"order": row, "nickname": guest.Nickname}
	if sale != nil {
		resp["sale_id"] = sale.ID
	} else {
		resp["debt"] = guestDebt(guest.ID)
	}
	c.JSON(http.StatusCreated, resp)
}
