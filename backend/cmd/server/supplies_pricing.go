package main

// З10-и5 (OWNER.md, трек З; №68, №69; решение основателя 29.09): цены поставки.
// Владелец оценивает поставку: у каждой строки одна или несколько цен за штуку
// («20 × 2,50 + 10 × 2,80»), штук в сумме — сколько пришло после правок приёмки;
// контрагент, номер и дата документа, способ оплаты, «оплачен», категория
// расхода (закупочная, по умолчанию «Закупка товаров» — решение основателя
// 29.09), VAT документа. Строка разбивается на куски-партии с ценами, а всё, что
// уже ушло со склада, раскладывается по кускам заново (lotSplit): проданное — по
// первой цене, возврат — туда, откуда взяло его списание. Поставка сама заводит
// один расход со ссылкой на себя; его можно поправить вручную — после правки
// суммы поставка его больше не перезаписывает, а расхождение отдаёт флагом.
// Повторная оценка — та же разбивка всех движений строки заново. Цена за штуку —
// брутто, как все суммы расходов (сколько заплатили за штуку); VAT — одной суммой
// по документу, в расход. Всё — только владельцу: админ цен закупки не видит.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxPricePieces    = 10      // цен у строки
	maxSupplyPricePLN = 100_000 // цена за штуку
)

func init() {
	for k, v := range map[string]string{
		"bad_prices":       fmt.Sprintf("У строки — от 1 до %d цен: штуки — целые от 1, цена за штуку — от 0 до %d zł, не больше двух знаков после запятой", maxPricePieces, maxSupplyPricePLN),
		"pricing_lines":    "Цены нужны у каждой строки поставки, где пришло больше нуля, — по одной записи на позицию",
		"pricing_total":    fmt.Sprintf("Сумма поставки — до %d zł", maxExpensePLN),
		"pricing_moves":    "Движения строки не сходятся с партиями — оценка не записана",
		"bad_doc":          fmt.Sprintf("Номер документа — до %d символов", maxExpenseDoc),
		"bad_doc_date":     "Дата документа — ГГГГ-ММ-ДД, не раньше чем 5 лет назад и не позже чем через год",
		"bad_method":       "Способ: till — из кассы, bank — банк, card — карта, owner — личные деньги владельца",
		"method_off":       expenseErrors["method_off"],
		"bad_vat":          "VAT — от 0 до суммы поставки, не больше двух знаков после запятой",
		"bad_category":     "Такой категории нет",
		"not_purchase":     "Категория расхода поставки — закупочная: «Закупка товаров», «Закупка кухни» или своя с галочкой «закупка»",
		"category_hidden":  expenseErrors["category_hidden"],
		"bad_counterparty": "Такого контрагента нет",
	} {
		supplyErrors[k] = v
	}
}

type pricePieceReq struct {
	Qty      int     `json:"qty"`
	PricePLN float64 `json:"price_pln"`
}

type priceLineReq struct {
	GoodID string          `json:"good_id"`
	Prices []pricePieceReq `json:"prices"`
}

// pricingRequest — оценка целиком: строки с ценами обязательны, способ — тоже;
// остальное не прислано — пусто (контрагент, документ, VAT) или по умолчанию
// («оплачен», «Закупка товаров»). Повторная оценка присылает всё заново.
type pricingRequest struct {
	Lines          []priceLineReq `json:"lines"`
	CounterpartyID *string        `json:"counterparty_id"`
	DocNo          *string        `json:"doc_no"`
	DocDate        *string        `json:"doc_date"`
	Method         *string        `json:"method"`
	Paid           *bool          `json:"paid"`
	CategoryID     *string        `json:"category_id"`
	VatPLN         *float64       `json:"vat_pln"`
}

// pricingFail — отказ с кодом и словами (строка, штуки).
type pricingFail struct {
	Code, Message string
}

func pricingRefuse(code, name, format string, args ...any) *pricingFail {
	msg := supplyErrors[code]
	if format != "" {
		msg = fmt.Sprintf(format, args...)
	}
	if name != "" {
		msg = "«" + name + "»: " + msg
	}
	return &pricingFail{code, msg}
}

// validPrice — цена за штуку: 0 (подарок поставщика) или от 0,01 до потолка,
// не больше двух знаков.
func validPrice(v float64) bool {
	return v == 0 || validatePLN(v, maxSupplyPricePLN)
}

// pricingPieces — цены строк из запроса (чистая, тест). have — сколько пришло по
// позиции после правок приёмки, names — имена для сообщений. Каждая строка, где
// пришло больше нуля, — ровно один раз; где ноль — не присылается; у строки — от 1
// до maxPricePieces цен, штуки — целые от 1, штук в сумме — сколько пришло.
// Возвращает цены по позициям (гроши за штуку, по порядку) и сумму поставки.
func pricingPieces(req []priceLineReq, have map[uuid.UUID]int, names map[uuid.UUID]string) (map[uuid.UUID][]lotPrice, int64, *pricingFail) {
	out, total := map[uuid.UUID][]lotPrice{}, int64(0)
	for _, l := range req {
		gid, err := uuid.Parse(strings.TrimSpace(l.GoodID))
		if err != nil {
			return nil, 0, pricingRefuse("pricing_lines", "", "")
		}
		qtyIn, ok := have[gid]
		if !ok {
			return nil, 0, pricingRefuse("pricing_lines", "", "Этой позиции в поставке нет")
		}
		if _, dup := out[gid]; dup {
			return nil, 0, pricingRefuse("pricing_lines", names[gid], "строка — один раз, цен в ней — сколько нужно")
		}
		if qtyIn == 0 && len(l.Prices) == 0 {
			continue // строку исправили в ноль — цен ей не нужно
		}
		if len(l.Prices) == 0 || len(l.Prices) > maxPricePieces {
			return nil, 0, pricingRefuse("bad_prices", names[gid], "")
		}
		ps, sum := make([]lotPrice, 0, len(l.Prices)), 0
		for _, p := range l.Prices {
			if p.Qty < 1 || p.Qty > maxStockQty || !validPrice(p.PricePLN) {
				return nil, 0, pricingRefuse("bad_prices", names[gid], "")
			}
			g := models.GroszFromPLN(p.PricePLN)
			ps, sum, total = append(ps, lotPrice{p.Qty, g}), sum+p.Qty, total+int64(p.Qty)*g
		}
		if sum != qtyIn {
			return nil, 0, pricingRefuse("pricing_qty", names[gid], "цен на %d шт, а пришло %d", sum, qtyIn)
		}
		out[gid] = ps
	}
	for gid, n := range have {
		if _, ok := out[gid]; !ok && n > 0 {
			return nil, 0, pricingRefuse("pricing_lines", names[gid], "нет цен — у каждой строки поставки нужна цена")
		}
	}
	if total > int64(maxExpensePLN)*100 {
		return nil, 0, pricingRefuse("pricing_total", "", "")
	}
	return out, total, nil
}

// supplyDoc — документ и расход поставки из запроса.
type supplyDoc struct {
	DocNo          string
	DocDate        *time.Time
	Method         string
	Paid           bool
	Vat            int64
	CategoryID     *uuid.UUID // nil — «Закупка товаров»
	CounterpartyID *uuid.UUID
}

// pricingDocOf — документ из запроса (чистая, тест): today — сегодня по календарю
// клуба, total — сумма поставки в грошах (VAT не больше неё).
func pricingDocOf(req pricingRequest, today time.Time, total int64) (supplyDoc, string) {
	d := supplyDoc{Paid: true}
	if req.DocNo != nil {
		d.DocNo = strings.TrimSpace(*req.DocNo)
		if len([]rune(d.DocNo)) > maxExpenseDoc {
			return d, "bad_doc"
		}
	}
	if req.DocDate != nil && strings.TrimSpace(*req.DocDate) != "" {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*req.DocDate), today.Location())
		if err != nil || t.Before(today.AddDate(-5, 0, 0)) || t.After(today.AddDate(1, 0, 0)) {
			return d, "bad_doc_date"
		}
		d.DocDate = &t
	}
	if req.Method == nil {
		return d, "bad_method"
	}
	if _, ok := expenseMethods[*req.Method]; !ok {
		return d, "bad_method"
	}
	d.Method = *req.Method
	if req.Paid != nil {
		d.Paid = *req.Paid
	}
	if req.VatPLN != nil && *req.VatPLN != 0 {
		v := *req.VatPLN
		if v < 0 || !validatePLN(v, maxExpensePLN) {
			return d, "bad_vat"
		}
		d.Vat = models.GroszFromPLN(v)
	}
	if d.Vat > total {
		return d, "bad_vat"
	}
	if req.CategoryID != nil && strings.TrimSpace(*req.CategoryID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.CategoryID))
		if err != nil {
			return d, "bad_category"
		}
		d.CategoryID = &id
	}
	if req.CounterpartyID != nil && strings.TrimSpace(*req.CounterpartyID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.CounterpartyID))
		if err != nil {
			return d, "bad_counterparty"
		}
		d.CounterpartyID = &id
	}
	return d, ""
}

// lineMove — движение склада, тронувшее партии строки поставки: сколько по
// партиям строки суммой.
type lineMove struct {
	ID             uuid.UUID
	CreatedAt      time.Time
	Reason         string
	SaleID         *uuid.UUID
	KitchenOrderID *uuid.UUID
	Qty            int
}

// lineEntries — движения строки для разбивки (чистая, тест): moves — по времени,
// qtyIn — «пришло» строки после правок. Приход — первое движение прихода строки,
// с «пришло» целиком: правки приёмки (З10-и4) складываются с ним и отдельными
// движениями не идут. Возврат знает своё списание в строке по продаже или заказу
// кухни (нет его в строке — продажа до партий — источника нет).
func lineEntries(moves []lineMove, qtyIn int) ([]lotEntry, error) {
	out, in := []lotEntry{}, false
	for _, m := range moves {
		switch {
		case m.Reason == "supply":
			if !in && m.Qty > 0 {
				out, in = append(out, lotEntry{Move: m.ID, Qty: qtyIn, In: true}), true
			}
		case m.Qty < 0:
			out = append(out, lotEntry{Move: m.ID, Qty: m.Qty})
		case m.Qty > 0:
			e := lotEntry{Move: m.ID, Qty: m.Qty}
			for _, o := range moves {
				if o.Qty < 0 && o.Reason != "supply" && ((m.SaleID != nil && o.SaleID != nil && *o.SaleID == *m.SaleID) ||
					(m.KitchenOrderID != nil && o.KitchenOrderID != nil && *o.KitchenOrderID == *m.KitchenOrderID)) {
					id := o.ID
					e.Source = &id
					break
				}
			}
			out = append(out, e)
		}
	}
	if !in {
		return nil, errLotSplit // у строки нет прихода
	}
	return out, nil
}

var errPricingMoves = errors.New("pricing_moves")

// pricingLine — разбить строку поставки (партии одной позиции, по part) на цены и
// переписать доли движений; остаток позиции не меняется. Под замком позиции.
func pricingLine(tx *gorm.DB, lots []models.StockLot, prices []lotPrice) error {
	ids, qtyIn, left := make([]uuid.UUID, len(lots)), 0, 0
	for i, l := range lots {
		ids[i], qtyIn, left = l.ID, qtyIn+l.QtyIn, left+l.QtyLeft
	}
	var moves []lineMove
	if err := tx.Raw(`SELECT m.id, m.created_at, m.reason, m.sale_id, m.kitchen_order_id, SUM(lm.qty) AS qty
		FROM lot_moves lm JOIN stock_moves m ON m.id = lm.stock_move_id WHERE lm.lot_id IN ?
		GROUP BY m.id, m.created_at, m.reason, m.sale_id, m.kitchen_order_id ORDER BY m.created_at, m.id`, ids).Scan(&moves).Error; err != nil {
		return err
	}
	entries, err := lineEntries(moves, qtyIn)
	if err != nil {
		return errPricingMoves
	}
	pieces, allocs, err := lotSplit(prices, entries)
	if err != nil {
		return errPricingMoves
	}
	got := 0
	for _, p := range pieces {
		got += p.Left
	}
	if got != left { // разбивка разошлась с остатком строки — не пишем
		log.Printf("цены поставки: у позиции %s разбивка дала остаток %d, а в партиях %d", lots[0].GoodID, got, left)
		return errPricingMoves
	}
	if err := tx.Where("lot_id IN ?", ids).Delete(&models.LotMove{}).Error; err != nil {
		return err
	}
	pieceIDs := make([]uuid.UUID, len(pieces))
	for k, p := range pieces {
		price := p.Price
		if k < len(lots) {
			if err := tx.Model(&models.StockLot{}).Where("id = ?", lots[k].ID).Updates(map[string]any{
				"part": k + 1, "price_grosz": price, "qty_in": p.In, "qty_left": p.Left}).Error; err != nil {
				return err
			}
			pieceIDs[k] = lots[k].ID
			continue
		}
		l := models.StockLot{ClubID: lots[0].ClubID, GoodID: lots[0].GoodID, Kind: models.LotSupply, SupplyID: lots[0].SupplyID,
			QtyIn: p.In, QtyLeft: p.Left, PriceGrosz: &price, ReceivedAt: lots[0].ReceivedAt, Part: k + 1}
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		pieceIDs[k] = l.ID
	}
	for i, e := range entries {
		for _, a := range allocs[i] {
			if a.Qty == 0 {
				continue
			}
			if err := tx.Create(&models.LotMove{StockMoveID: e.Move, LotID: pieceIDs[a.Piece], Qty: a.Qty}).Error; err != nil {
				return err
			}
		}
	}
	if len(lots) > len(pieces) { // лишние куски прежней оценки — без движений
		extra := []uuid.UUID{}
		for _, l := range lots[len(pieces):] {
			extra = append(extra, l.ID)
		}
		return tx.Where("id IN ?", extra).Delete(&models.StockLot{}).Error
	}
	return nil
}

// plnRu — «2,50» (гроши по-русски, для журнала).
func plnRu(g int64) string {
	return strings.Replace(fmt.Sprintf("%.2f", models.PLNFromGrosz(g)), ".", ",", 1)
}

// pricesText — «Cola 20 × 2,50 + 10 × 2,80, Lay's 20 × 3,10» (для журнала).
func pricesText(order []uuid.UUID, names map[uuid.UUID]string, prices map[uuid.UUID][]lotPrice) string {
	parts := []string{}
	for _, gid := range order {
		ps := prices[gid]
		if len(ps) == 0 {
			continue
		}
		p := make([]string, len(ps))
		for i, x := range ps {
			p[i] = fmt.Sprintf("%d × %s", x.Qty, plnRu(x.Price))
		}
		parts = append(parts, names[gid]+" "+strings.Join(p, " + "))
	}
	return strings.Join(parts, ", ")
}

// supplyExpenseNote — заметка расхода поставки: «поставка от 28.09 18:40: Cola ×30, …».
func supplyExpenseNote(s *models.Supply, lines string) string {
	note := supplyTitle(s.CreatedAt) + ": " + lines
	if r := []rune(note); len(r) > maxExpenseNote {
		note = string(r[:maxExpenseNote-1]) + "…"
	}
	return note
}

// Что поставка сделала со своим расходом.
const (
	expCreated = "created" // завела
	expUpdated = "updated" // переписала по новой оценке
	expSame    = "same"    // переписывать нечего
	expEdited  = "edited"  // сумму поправили вручную — не трогает
	expStorno  = "storno"  // сторнирован — не трогает
	expDropped = "dropped" // оценка в ноль — сторнировала свой расход
	expNone    = "none"    // сумма ноль — расхода нет
)

// supplyExpenseSync — расход поставки по оценке (под замком ящика): нет — заводит;
// есть, не сторнирован и сумма та, что поставка в него записала, — переписывает
// (место в ящике — как при правке расхода); сумму поправили вручную или
// сторнировали — не трогает. Сумма ноль — расхода нет (свой — сторно).
func supplyExpenseSync(tx *gorm.DB, s *models.Supply, want models.Expense) (string, *models.Expense, []string, error) {
	now := time.Now()
	var cur models.Expense
	has := s.ExpenseID != nil && tx.Session(&gorm.Session{QueryFields: true}).First(&cur, "id = ?", *s.ExpenseID).Error == nil
	switch {
	case !has && want.GrossGrosz == 0:
		return expNone, nil, nil, nil
	case !has:
		want.CreatedAt = now
		placeTill(tx, &want, now)
		return expCreated, &want, nil, tx.Create(&want).Error
	case cur.StornoAt != nil:
		return expStorno, &cur, nil, nil
	case s.ExpenseGrosz == nil || cur.GrossGrosz != *s.ExpenseGrosz:
		return expEdited, &cur, nil, nil
	case want.GrossGrosz == 0:
		err := tx.Model(&models.Expense{}).Where("id = ? AND storno_at IS NULL", cur.ID).
			Updates(map[string]any{"storno_at": now, "storno_by": want.CreatedBy, "storno_reason": "поставка оценена в 0 zł"}).Error
		return expDropped, &cur, nil, err
	}
	next := cur
	next.OpDate, next.CategoryID, next.CounterpartyID = want.OpDate, want.CategoryID, want.CounterpartyID
	next.GrossGrosz, next.VatGrosz, next.Method, next.Paid = want.GrossGrosz, want.VatGrosz, want.Method, want.Paid
	next.DocNo, next.Note = want.DocNo, want.Note
	names := categoryNames()
	for k, v := range counterpartyNames() {
		names[k] = v
	}
	changes := expenseChanges(cur, next, names)
	if len(changes) == 0 {
		return expSame, &cur, nil, nil
	}
	switch { // место в ящике — как при правке расхода (З9-и3а)
	case tillPaid(&cur) && tillPaid(&next) && cur.OpDate.Equal(next.OpDate):
	case !cur.Paid && next.Paid:
		placeTill(tx, &next, now)
	default:
		placeTill(tx, &next, cur.CreatedAt)
	}
	err := tx.Model(&models.Expense{}).Where("id = ?", cur.ID).Updates(map[string]any{
		"op_date": next.OpDate.Format("2006-01-02"), "category_id": next.CategoryID, "counterparty_id": next.CounterpartyID,
		"gross_grosz": next.GrossGrosz, "vat_grosz": next.VatGrosz, "method": next.Method, "paid": next.Paid,
		"doc_no": next.DocNo, "note": next.Note, "updated_at": now, "drawer_at": next.DrawerAt, "explains_closeout": next.ExplainsCloseout}).Error
	return expUpdated, &next, changes, err
}

// defaultPurchaseCategory — «Закупка товаров» (стандартная, код goods).
func defaultPurchaseCategory() (uuid.UUID, bool) {
	var cat models.ExpenseCategory
	if db.Where("code = ?", "goods").First(&cat).Error != nil {
		return uuid.Nil, false
	}
	return cat.ID, true
}

// PUT /admin/supplies/:id/pricing — оценить поставку (owner): цены строк, документ,
// способ оплаты, категория расхода. Ждущая — становится оценённой, оценённая —
// переоценивается заново; отменённую не оценить.
func handleSupplyPricingPut(c *gin.Context) {
	var s models.Supply
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		supplyFail(c, http.StatusNotFound, "supply_not_found")
		return
	}
	if s.Status == models.SupplyCancelled {
		supplyFail(c, http.StatusConflict, "supply_cancelled")
		return
	}
	var req pricingRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Lines) == 0 {
		supplyFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	ownerID, _ := uuid.Parse(c.GetString("user_id"))
	repriced := s.Status == models.SupplyPriced
	var (
		fail    *pricingFail
		failed  string
		prices  map[uuid.UUID][]lotPrice
		order   []uuid.UUID
		names   = map[uuid.UUID]string{}
		action  string
		exp     *models.Expense
		changes []string
		doc     supplyDoc
		total   int64
		catName string
	)
	err := db.Transaction(func(tx *gorm.DB) error {
		var cur models.Supply
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cur, "id = ?", s.ID).Error; err != nil {
			return err
		}
		if cur.Status == models.SupplyCancelled {
			fail = pricingRefuse("supply_cancelled", "", "")
			return errPricingMoves
		}
		s = cur
		// позиции поставки — под замок (продажи идут по очереди), потом её партии
		var gids []uuid.UUID
		if err := tx.Model(&models.StockLot{}).Where("supply_id = ?", s.ID).Distinct().Pluck("good_id", &gids).Error; err != nil {
			return err
		}
		var goods []models.Good
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", gids).Order("id").Find(&goods).Error; err != nil {
			return err
		}
		for _, g := range goods {
			names[g.ID] = g.Name
		}
		var lots []models.StockLot
		if err := tx.Where("supply_id = ?", s.ID).Order("part, id").Find(&lots).Error; err != nil {
			return err
		}
		byGood, have := map[uuid.UUID][]models.StockLot{}, map[uuid.UUID]int{}
		for _, l := range lots {
			if _, ok := byGood[l.GoodID]; !ok {
				order = append(order, l.GoodID)
			}
			byGood[l.GoodID], have[l.GoodID] = append(byGood[l.GoodID], l), have[l.GoodID]+l.QtyIn
		}
		sort.SliceStable(order, func(a, b int) bool { return strings.ToLower(names[order[a]]) < strings.ToLower(names[order[b]]) })
		if prices, total, fail = pricingPieces(req.Lines, have, names); fail != nil {
			return errPricingMoves
		}
		var code string
		if doc, code = pricingDocOf(req, expenseToday(), total); code != "" {
			fail = pricingRefuse(code, "", "")
			return errPricingMoves
		}
		// расход поставки: категория, контрагент, способ — как у расхода (З9-и4)
		var prev *models.Expense
		if s.ExpenseID != nil {
			var e models.Expense
			if tx.Session(&gorm.Session{QueryFields: true}).First(&e, "id = ?", *s.ExpenseID).Error == nil {
				prev = &e
			}
		}
		catID, ok := uuid.Nil, false
		if doc.CategoryID != nil {
			catID, ok = *doc.CategoryID, true
		} else if catID, ok = defaultPurchaseCategory(); !ok {
			fail = pricingRefuse("bad_category", "", "")
			return errPricingMoves
		}
		var cat models.ExpenseCategory
		if tx.First(&cat, "id = ?", catID).Error != nil {
			fail = pricingRefuse("bad_category", "", "")
			return errPricingMoves
		}
		catName = cat.Name
		switch {
		case !cat.Purchase:
			fail = pricingRefuse("not_purchase", "", "")
		case cat.Hidden && (prev == nil || prev.CategoryID != cat.ID):
			fail = pricingRefuse("category_hidden", "", "")
		case (prev == nil || prev.Method != doc.Method) && !expenseMethodsOn()[doc.Method]:
			fail = pricingRefuse("method_off", "", "")
		}
		if fail == nil && doc.CounterpartyID != nil {
			var n int64
			if tx.Model(&models.Counterparty{}).Where("id = ?", *doc.CounterpartyID).Count(&n); n == 0 {
				fail = pricingRefuse("bad_counterparty", "", "")
			}
		}
		if fail != nil {
			return errPricingMoves
		}
		for _, gid := range order {
			if have[gid] == 0 {
				continue // строку исправили в ноль — нечего оценивать
			}
			if err := pricingLine(tx, byGood[gid], prices[gid]); err != nil {
				failed = names[gid]
				return err
			}
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil { // расход — как все расходы (З9-и3а)
			return err
		}
		opDate := clubDayOf(s.CreatedAt.In(clubLocation), int(settingInt64("report_hour", 8))) // без даты документа — день приёмки
		if doc.DocDate != nil {
			opDate = *doc.DocDate
		}
		want := models.Expense{ClubID: s.ClubID, OpDate: opDate, Paid: doc.Paid, CategoryID: catID, CounterpartyID: doc.CounterpartyID,
			GrossGrosz: total, VatGrosz: doc.Vat, Method: doc.Method, DocNo: doc.DocNo, SupplyID: &s.ID, CreatedBy: &ownerID,
			Note: supplyExpenseNote(&s, linesText(supplyLinesOf(tx, []uuid.UUID{s.ID})[s.ID]))}
		var err error
		if action, exp, changes, err = supplyExpenseSync(tx, &s, want); err != nil {
			return err
		}
		upd := map[string]any{"status": models.SupplyPriced, "counterparty_id": doc.CounterpartyID, "doc_no": doc.DocNo,
			"doc_date": doc.DocDate, "method": doc.Method, "priced_by": ownerID, "priced_at": time.Now(), "updated_at": time.Now()}
		switch action {
		case expCreated, expUpdated, expSame:
			upd["expense_id"], upd["expense_grosz"] = exp.ID, exp.GrossGrosz
		case expDropped, expNone:
			upd["expense_id"], upd["expense_grosz"] = nil, nil
		}
		return tx.Model(&models.Supply{}).Where("id = ?", s.ID).Updates(upd).Error
	})
	switch {
	case fail != nil:
		status := http.StatusBadRequest
		if fail.Code == "supply_cancelled" {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"code": fail.Code, "message": fail.Message})
		return
	case errors.Is(err, errPricingMoves):
		c.JSON(http.StatusConflict, gin.H{"code": "pricing_moves", "message": "«" + failed + "»: " + supplyErrors["pricing_moves"]})
		return
	case err != nil:
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.Supply
	db.First(&fresh, "id = ?", s.ID)
	head := supplyTitle(s.CreatedAt) + ": "
	if repriced {
		head = "переоценка — " + head
	}
	line := head + pricesText(order, names, prices) + " · " + plnRu(total) + " zł · " + expenseMethods[doc.Method]
	if doc.DocNo != "" {
		line += " · " + doc.DocNo
	}
	logAdminAction(c, "supply_price", nil, line)
	switch action {
	case expCreated:
		logAdminAction(c, "expense_create", nil, expenseLine(exp, catName)+" · "+supplyTitle(s.CreatedAt))
	case expUpdated:
		logAdminAction(c, "expense_update", nil, catName+fmt.Sprintf(" · %.2f zł: ", models.PLNFromGrosz(exp.GrossGrosz))+
			strings.Join(changes, "; ")+" · переоценка — "+supplyTitle(s.CreatedAt))
	case expDropped:
		logAdminAction(c, "expense_storno", nil, expenseLine(exp, catName)+" — «поставка оценена в 0 zł»")
	}
	c.JSON(http.StatusOK, gin.H{"supply": pricingOut([]models.Supply{fresh})[0], "expense_action": action, "repriced": repriced})
}

// pricingOut — поставки владельцу: строки с кусками (штуки, цена, осталось),
// сумма, документ, расход и его флаги: edited — сумму расхода поправили вручную
// (поставка его не перезаписывает), diff — расход минус поставка.
func pricingOut(list []models.Supply) []gin.H {
	base := suppliesOut(list)
	ids, who, cps, exps := []uuid.UUID{}, []string{}, []uuid.UUID{}, []uuid.UUID{}
	for _, s := range list {
		ids = append(ids, s.ID)
		if s.PricedBy != nil {
			who = append(who, s.PricedBy.String())
		}
		if s.CounterpartyID != nil {
			cps = append(cps, *s.CounterpartyID)
		}
		if s.ExpenseID != nil {
			exps = append(exps, *s.ExpenseID)
		}
	}
	var lots []models.StockLot
	if len(ids) > 0 {
		db.Where("supply_id IN ?", ids).Order("part, id").Find(&lots)
	}
	pieces := map[uuid.UUID]map[uuid.UUID][]gin.H{} // поставка → позиция → куски
	totals := map[uuid.UUID]int64{}
	for _, l := range lots {
		sid := *l.SupplyID
		if pieces[sid] == nil {
			pieces[sid] = map[uuid.UUID][]gin.H{}
		}
		p := gin.H{"qty": l.QtyIn, "left": l.QtyLeft, "price_pln": nil}
		if l.PriceGrosz != nil {
			p["price_pln"] = models.PLNFromGrosz(*l.PriceGrosz)
			totals[sid] += int64(l.QtyIn) * *l.PriceGrosz
		}
		pieces[sid][l.GoodID] = append(pieces[sid][l.GoodID], p)
	}
	cpNames := map[string]string{}
	if len(cps) > 0 {
		var list []models.Counterparty
		db.Select("id, name").Where("id IN ?", cps).Find(&list)
		for _, cp := range list {
			cpNames[cp.ID.String()] = cp.Name
		}
	}
	expByID := map[uuid.UUID]models.Expense{}
	if len(exps) > 0 {
		var list []models.Expense
		expenseQ().Where("id IN ?", exps).Find(&list)
		for _, e := range list {
			expByID[e.ID] = e
		}
	}
	cats, nick := categoryNames(), nicknamesByID(who)
	for i, s := range list {
		row := base[i]
		lines := []gin.H{}
		for _, l := range row["lines"].([]supplyLine) {
			ps := pieces[s.ID][l.GoodID]
			if ps == nil {
				ps = []gin.H{}
			}
			lines = append(lines, gin.H{"good_id": l.GoodID, "name": l.Name, "qty": l.Qty, "received": l.Received, "gone": max(l.Gone, 0), "pieces": ps})
		}
		row["lines"] = lines
		if s.Status == models.SupplyPriced {
			row["total_pln"] = models.PLNFromGrosz(totals[s.ID])
		}
		row["doc_no"] = s.DocNo
		if s.DocDate != nil {
			row["doc_date"] = s.DocDate.Format("2006-01-02")
		}
		if s.Method != nil {
			row["method"] = *s.Method
		}
		if s.CounterpartyID != nil {
			row["counterparty_id"], row["counterparty"] = *s.CounterpartyID, cpNames[s.CounterpartyID.String()]
		}
		if s.PricedAt != nil {
			row["priced_at"] = *s.PricedAt
			if s.PricedBy != nil {
				row["priced_by"] = nick[s.PricedBy.String()]
			}
		}
		if s.ExpenseID != nil {
			if e, ok := expByID[*s.ExpenseID]; ok {
				row["expense"] = gin.H{"id": e.ID, "amount_pln": models.PLNFromGrosz(e.GrossGrosz), "vat_pln": models.PLNFromGrosz(e.VatGrosz),
					"category_id": e.CategoryID, "category": cats[e.CategoryID.String()], "paid": e.Paid, "op_date": e.OpDate.Format("2006-01-02"),
					"storno": e.StornoAt != nil, "edited": s.ExpenseGrosz == nil || e.GrossGrosz != *s.ExpenseGrosz,
					"diff_pln": models.PLNFromGrosz(e.GrossGrosz - totals[s.ID])}
			}
		}
		base[i] = row
	}
	return base
}

// GET /admin/supplies/:id/pricing — поставка с ценами (owner).
func handleSupplyPricing(c *gin.Context) {
	var s models.Supply
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		supplyFail(c, http.StatusNotFound, "supply_not_found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"supply": pricingOut([]models.Supply{s})[0]})
}

// GET /admin/purchases — закупки (owner): ждут цен (старые первыми) и оценённые
// (последние 100, свежие первыми) — для «Бухгалтерии» → «Закупки» (З10-и6).
func handlePurchases(c *gin.Context) {
	var waiting, priced []models.Supply
	if err := db.Where("status = ?", models.SupplyWaiting).Order("created_at").Find(&waiting).Error; err != nil {
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if err := db.Where("status = ?", models.SupplyPriced).Order("priced_at DESC").Limit(100).Find(&priced).Error; err != nil {
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"waiting": pricingOut(waiting), "priced": pricingOut(priced)})
}
