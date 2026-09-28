package main

// З10-и3 (OWNER.md, трек З; №68): партии в работе. Каждое движение склада знает
// свои партии: stockMove после строки stock_moves зовёт lotsApply в той же
// транзакции. Списание (продажа, заказ кухни, «бой») — FIFO, со старых партий;
// приход — строка поставки (приёмка многими строками — З10-и4) или новая
// поставка из одной строки, ждёт цен; правка приёмки — в партию своей строки
// («пришло» меняется вместе с остатком, З10-и4); «нашлось» — новая партия без
// цены (Д5); возврат при отмене продажи или
// заказа — в те партии, откуда взяло его списание, а продажа или заказ до партий
// — в самую новую партию позиции (грабли плана). Сверка «сумма остатков партий =
// остаток позиции» — GET /admin/goods/lots/check (владелец).

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// lotsApply — партии движения moveID. Строка позиции уже под замком UPDATE из
// stockMove — два движения одной позиции идут по очереди, партии не делятся.
func lotsApply(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	switch {
	case op.Reason == "supply" && op.Lot != nil:
		return lotsFix(tx, op, moveID)
	case op.Delta < 0:
		return lotsTake(tx, op, moveID)
	case op.Reason == "supply":
		return lotsSupply(tx, op, moveID)
	case op.Reason == "void":
		return lotsBack(tx, op, moveID)
	}
	l, err := lotCreate(tx, op, models.LotFound, nil, op.Delta, 0) // «нашлось» (Д5)
	if err != nil {
		return err
	}
	return lotsWrite(tx, moveID, []lotPart{{l.ID, op.Delta}}, 1)
}

// lotCreate — новая партия позиции op.GoodID без цены: пришло qtyIn, осталось
// left (движение, что её завело, допишет остаток через lotsWrite).
func lotCreate(tx *gorm.DB, op stockOp, kind string, supplyID *uuid.UUID, qtyIn, left int) (models.StockLot, error) {
	l := models.StockLot{ClubID: op.ClubID, GoodID: op.GoodID, Kind: kind, SupplyID: supplyID,
		QtyIn: qtyIn, QtyLeft: left, ReceivedAt: time.Now(), Part: 1}
	return l, tx.Create(&l).Error
}

// lotsWrite — доли движения по партиям: остаток партий и строки lot_moves (sign:
// −1 — списание, +1 — приход и возврат).
func lotsWrite(tx *gorm.DB, moveID uuid.UUID, parts []lotPart, sign int) error {
	for _, p := range parts {
		if err := tx.Model(&models.StockLot{}).Where("id = ?", p.ID).
			UpdateColumn("qty_left", gorm.Expr("qty_left + ?", sign*p.Qty)).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.LotMove{StockMoveID: moveID, LotID: p.ID, Qty: sign * p.Qty}).Error; err != nil {
			return err
		}
	}
	return nil
}

// lotsTake — списание FIFO. Партии отстали от остатка (остаток меняли мимо
// stockMove — старый сервер во время обновления, SQL) — недостающее дописывается
// начальной партией без цены, продажа не встаёт; в журнал сервера — строка.
func lotsTake(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	var lots []models.StockLot
	if err := tx.Where("good_id = ? AND qty_left > 0", op.GoodID).Order("received_at, part, id").Find(&lots).Error; err != nil {
		return err
	}
	left, have := make([]lotLeft, 0, len(lots)+1), 0
	for _, l := range lots {
		left, have = append(left, lotLeft{l.ID, l.QtyLeft}), have+l.QtyLeft
	}
	if need := -op.Delta; have < need {
		l, err := lotCreate(tx, op, models.LotInitial, nil, need-have, need-have)
		if err != nil {
			return err
		}
		log.Printf("партии: у позиции %s в партиях на %d шт меньше, чем списываем, — дописал начальную партию", op.GoodID, need-have)
		left = append(left, lotLeft{l.ID, l.QtyLeft})
	}
	parts, err := lotFIFO(left, -op.Delta)
	if err != nil {
		return err
	}
	return lotsWrite(tx, moveID, parts, -1)
}

// lotsSupply — приход: строка поставки op.SupplyID или поставка из одной строки
// (ждёт цен), её партия без цены.
func lotsSupply(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	sid := op.SupplyID
	if sid == nil {
		s := models.Supply{ClubID: op.ClubID, Status: models.SupplyWaiting, Note: op.Note, ReceivedBy: op.By}
		if err := tx.Create(&s).Error; err != nil {
			return err
		}
		sid = &s.ID
	}
	l, err := lotCreate(tx, op, models.LotSupply, sid, op.Delta, 0)
	if err != nil {
		return err
	}
	return lotsWrite(tx, moveID, []lotPart{{l.ID, op.Delta}}, 1)
}

// errLineSold — правка приёмки уводит строку ниже того, что из неё уже ушло.
var errLineSold = errors.New("line_sold")

// lotsFix — правка приёмки (З10-и4): «пришло» и остаток партии строки op.Lot
// меняются на Delta; из строки ушло больше, чем останется, — errLineSold.
func lotsFix(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	res := tx.Model(&models.StockLot{}).
		Where("id = ? AND good_id = ? AND qty_left + ? >= 0 AND qty_in + ? >= 0", *op.Lot, op.GoodID, op.Delta, op.Delta).
		Updates(map[string]any{"qty_in": gorm.Expr("qty_in + ?", op.Delta), "qty_left": gorm.Expr("qty_left + ?", op.Delta)})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errLineSold
	}
	return tx.Create(&models.LotMove{StockMoveID: moveID, LotID: *op.Lot, Qty: op.Delta}).Error
}

// lotsBack — возврат при отмене: в партии, откуда взяло списание той же продажи
// или того же заказа, за вычетом уже вернувшегося (lotBack); чего там нет —
// продажа или заказ до партий — в самую новую партию позиции, а партий нет вовсе —
// в новую начальную.
func lotsBack(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	var col string
	var ref *uuid.UUID
	switch {
	case op.SaleID != nil:
		col, ref = "sale_id", op.SaleID
	case op.OrderID != nil:
		col, ref = "kitchen_order_id", op.OrderID
	}
	parts, rest := []lotPart{}, op.Delta
	if ref != nil {
		var taken, back []lotPart
		lm := func(sign string) *gorm.DB {
			return tx.Table("lot_moves lm").Select("lm.lot_id AS id, ABS(lm.qty) AS qty").
				Joins("JOIN stock_moves m ON m.id = lm.stock_move_id").Joins("JOIN stock_lots l ON l.id = lm.lot_id").
				Where("m."+col+" = ? AND m.delta "+sign+" 0 AND m.id <> ?", *ref, moveID).Order("l.received_at, l.part, l.id")
		}
		if err := lm("<").Scan(&taken).Error; err != nil {
			return err
		}
		if err := lm(">").Scan(&back).Error; err != nil {
			return err
		}
		net := 0
		for _, t := range taken {
			net += t.Qty
		}
		for _, b := range back {
			net -= b.Qty
		}
		if q := min(rest, max(net, 0)); q > 0 {
			if p, err := lotBack(taken, back, q); err == nil {
				parts, rest = p, rest-q
			}
		}
	}
	if rest > 0 { // до партий — в самую новую партию позиции
		var newest models.StockLot
		err := tx.Where("good_id = ?", op.GoodID).Order("received_at DESC, part DESC, id DESC").First(&newest).Error
		if err == gorm.ErrRecordNotFound {
			newest, err = lotCreate(tx, op, models.LotInitial, nil, rest, 0)
		}
		if err != nil {
			return err
		}
		parts = addPart(parts, newest.ID, rest)
	}
	return lotsWrite(tx, moveID, parts, 1)
}

// addPart — прибавить к доле партии (одна строка на партию у движения).
func addPart(ps []lotPart, id uuid.UUID, qty int) []lotPart {
	for i := range ps {
		if ps[i].ID == id {
			ps[i].Qty += qty
			return ps
		}
	}
	return append(ps, lotPart{id, qty})
}

// GET /admin/goods/lots/check — сверка партий (owner): у каждой позиции сумма
// остатков партий = остаток. Расхождение — список позиций с разницей.
func handleGoodsLotsCheck(c *gin.Context) {
	var rows []struct {
		ID    uuid.UUID
		Name  string
		Stock int
		Lots  int
		N     int
	}
	if err := db.Raw(`SELECT g.id, g.name, g.stock, COALESCE(SUM(l.qty_left), 0) AS lots, COUNT(l.id) AS n
		FROM goods g LEFT JOIN stock_lots l ON l.good_id = g.id GROUP BY g.id, g.name, g.stock ORDER BY g.name`).Scan(&rows).Error; err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	bad, lots := []gin.H{}, 0
	for _, r := range rows {
		lots += r.N
		if r.Lots != r.Stock {
			bad = append(bad, gin.H{"good_id": r.ID, "name": r.Name, "stock": r.Stock, "lots_left": r.Lots, "diff": r.Stock - r.Lots})
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": len(bad) == 0, "goods": len(rows), "lots": lots, "mismatch": bad})
}
