package main

// З10-и7 (OWNER.md, трек З; №68, №70; решение основателя 29.09): партии в
// корректировке и «Потерях». Владелец списывает «бой» из выбранной партии —
// список с остатками и ценами; админ — со старых партий первыми (FIFO), партию
// ему не выбрать: в списке цены закупки. «Потери» считаются и по рознице (цена
// движения, З6-и2), и по себестоимости партий, из которых ушло; ушедшее из
// партий без цены — «без себестоимости: N шт». Партиям без поставки — начальному
// остатку (З10-и3) и «нашлось» (Д5) — владелец ставит одну цену за штуку в
// «Закупках» (решение основателя 29.09): без VAT и без расхода, это не покупка.

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

var errLotShort = errors.New("lot_short")

func init() {
	goodErrors["lot_owner_only"] = "Партию выбирает владелец — админ списывает со старых партий первыми"
	goodErrors["bad_lot"] = "Партию выбирают только для корректировки в минус и только у этой позиции"
	goodErrors["lot_short"] = "В выбранной партии меньше, чем списываешь"
	goodErrors["lot_not_found"] = "Партия не найдена"
	goodErrors["lot_supply"] = "У партии поставки цену ставит оценка поставки — «Бухгалтерия» → «Закупки»"
	goodErrors["bad_lot_price"] = fmt.Sprintf("Цена за штуку — от 0 до %d zł, не больше двух знаков после запятой", maxSupplyPricePLN)
}

// lotsTakeLot — списание из выбранной партии (владелец): партия той же позиции и
// в ней не меньше, чем списываем; иначе errLotShort.
func lotsTakeLot(tx *gorm.DB, op stockOp, moveID uuid.UUID) error {
	res := tx.Model(&models.StockLot{}).Where("id = ? AND good_id = ? AND qty_left + ? >= 0", *op.Lot, op.GoodID, op.Delta).
		UpdateColumn("qty_left", gorm.Expr("qty_left + ?", op.Delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errLotShort
	}
	return tx.Create(&models.LotMove{StockMoveID: moveID, LotID: *op.Lot, Qty: op.Delta}).Error
}

// lotTitle — откуда партия словами (чистая, тест): «поставка от 28.09 18:40»
// (время приёмки — из supplyAt), «начальный остаток», «нашлось 29.09 12:00».
func lotTitle(l models.StockLot, supplyAt map[uuid.UUID]time.Time) string {
	switch l.Kind {
	case models.LotInitial:
		return "начальный остаток"
	case models.LotFound:
		return "нашлось " + l.ReceivedAt.In(clubLocation).Format("02.01 15:04")
	}
	if l.SupplyID != nil {
		if at, ok := supplyAt[*l.SupplyID]; ok {
			return supplyTitle(at)
		}
	}
	return "поставка"
}

// lotsOut — партии владельцу: откуда, пришло, осталось, цена за штуку (нет —
// null), позиция.
func lotsOut(lots []models.StockLot) []gin.H {
	sids, gids := []uuid.UUID{}, []uuid.UUID{}
	for _, l := range lots {
		gids = append(gids, l.GoodID)
		if l.SupplyID != nil {
			sids = append(sids, *l.SupplyID)
		}
	}
	at, names := map[uuid.UUID]time.Time{}, map[uuid.UUID]string{}
	if len(sids) > 0 {
		var sups []models.Supply
		db.Select("id, created_at").Where("id IN ?", sids).Find(&sups)
		for _, s := range sups {
			at[s.ID] = s.CreatedAt
		}
	}
	if len(gids) > 0 {
		var goods []models.Good
		db.Select("id, name").Where("id IN ?", gids).Find(&goods)
		for _, g := range goods {
			names[g.ID] = g.Name
		}
	}
	out := make([]gin.H, 0, len(lots))
	for _, l := range lots {
		row := gin.H{"id": l.ID, "good_id": l.GoodID, "name": names[l.GoodID], "kind": l.Kind, "title": lotTitle(l, at),
			"qty_in": l.QtyIn, "qty_left": l.QtyLeft, "part": l.Part, "received_at": l.ReceivedAt, "price_pln": nil}
		if l.PriceGrosz != nil {
			row["price_pln"] = models.PLNFromGrosz(*l.PriceGrosz)
		}
		if l.SupplyID != nil {
			row["supply_id"] = *l.SupplyID
		}
		out = append(out, row)
	}
	return out
}

// GET /admin/goods/:id/lots — партии позиции, где что-то осталось, по порядку
// списания (owner): корректировка с выбором партии.
func handleGoodLots(c *gin.Context) {
	var g models.Good
	if err := db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "good_not_found")
		return
	}
	var lots []models.StockLot
	db.Where("good_id = ? AND qty_left > 0", g.ID).Order("received_at, part, id").Find(&lots)
	c.JSON(http.StatusOK, gin.H{"lots": lotsOut(lots), "stock": g.Stock})
}

// extraLots — партии без поставки (начальный остаток и «нашлось») для
// «Закупок»: без цены — все, где что-то приходило; с ценой — пока в них что-то
// осталось. Сначала без цены, по позиции.
func extraLots() ([]gin.H, int) {
	var lots []models.StockLot
	db.Where("kind IN ? AND qty_in > 0 AND (price_grosz IS NULL OR qty_left > 0)", []string{models.LotInitial, models.LotFound}).
		Order("received_at, part, id").Find(&lots)
	out := lotsOut(lots)
	sort.SliceStable(out, func(a, b int) bool {
		pa, pb := out[a]["price_pln"] == nil, out[b]["price_pln"] == nil
		if pa != pb {
			return pa
		}
		return strings.ToLower(out[a]["name"].(string)) < strings.ToLower(out[b]["name"].(string))
	})
	unpriced := 0
	for _, l := range out {
		if l["price_pln"] == nil {
			unpriced++
		}
	}
	return out, unpriced
}

// PUT /admin/lots/:id/price — цена за штуку партии без поставки (owner):
// начальный остаток или «нашлось». Партию поставки оценивает её поставка.
func handleLotPrice(c *gin.Context) {
	var req struct {
		PricePLN *float64 `json:"price_pln"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PricePLN == nil || !validPrice(*req.PricePLN) {
		goodFail(c, http.StatusBadRequest, "bad_lot_price")
		return
	}
	var l models.StockLot
	if err := db.First(&l, "id = ?", c.Param("id")).Error; err != nil {
		goodFail(c, http.StatusNotFound, "lot_not_found")
		return
	}
	if l.Kind == models.LotSupply {
		goodFail(c, http.StatusConflict, "lot_supply")
		return
	}
	price := models.GroszFromPLN(*req.PricePLN)
	if err := db.Model(&models.StockLot{}).Where("id = ?", l.ID).Update("price_grosz", price).Error; err != nil {
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	was := "без цены"
	if l.PriceGrosz != nil {
		was = plnRu(*l.PriceGrosz) + " zł"
	}
	var g models.Good
	db.Select("id, name").First(&g, "id = ?", l.GoodID)
	logAdminAction(c, "lot_price", nil, fmt.Sprintf("%s · %s · %d шт — %s zł за штуку (было %s)", g.Name, lotTitle(l, nil), l.QtyIn, plnRu(price), was))
	var fresh models.StockLot
	db.First(&fresh, "id = ?", l.ID)
	c.JSON(http.StatusOK, gin.H{"lot": lotsOut([]models.StockLot{fresh})[0]})
}

// lossesByCost — «Потери» по себестоимости за период: что корректировки в минус
// взяли из партий с ценой — по цене партии (гроши) и сколько это штук. Остальное
// из потерь — «без себестоимости» (партии без цены, движения до партий).
func lossesByCost(from, to time.Time) (int64, int64) {
	var r struct {
		Cost  int64
		Units int64
	}
	db.Raw(`SELECT COALESCE(SUM(-lm.qty * l.price_grosz), 0) AS cost, COALESCE(SUM(-lm.qty), 0) AS units `+lossCostFrom, from, to).Scan(&r)
	return r.Cost, r.Units
}

// lossCostFrom — списанное корректировкой из партий с ценой за [from, to): потери по себестоимости — в «Деньгах»,
// в «Прибыли» и по дням «Обзора» (З12-и4) одними условиями.
const lossCostFrom = `FROM stock_moves m JOIN lot_moves lm ON lm.stock_move_id = m.id JOIN stock_lots l ON l.id = lm.lot_id
		WHERE m.created_at >= ? AND m.created_at < ? AND m.reason = 'adjust' AND m.delta < 0 AND l.price_grosz IS NOT NULL`

// supplyTimes — время приёмки поставки партии (для «поставка от …»).
func supplyTimes(id *uuid.UUID) map[uuid.UUID]time.Time {
	out := map[uuid.UUID]time.Time{}
	if id != nil {
		var s models.Supply
		if db.Select("id, created_at").First(&s, "id = ?", *id).Error == nil {
			out[s.ID] = s.CreatedAt
		}
	}
	return out
}
