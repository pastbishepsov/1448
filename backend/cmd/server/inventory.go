package main

// З23-и3 «Инвентаризация — сервер и акт» (OWNER.md, трек З; №54 «в», З-Р28; миграция 098).
//
// POST /admin/goods/inventory — акт пересчёта (персонал): по строке — позиция, сколько было в системе, когда начали
// считать (expected), и сколько насчитали (counted). Акт проводится целиком или никак: позиции — под замком, и если
// остаток какой-то из них с начала пересчёта изменился (продали, приняли поставку) — 409 stock_moved со списком, ничего
// не записано: пересчитать эти позиции или поставить новое «в системе». Разница — корректировка (reason adjust) с номером
// акта (stock_moves.inventory_id): недостача — со старых партий первыми (FIFO) или, у владельца, из выбранной партии
// (как «бой», З10-и7); излишек — партия «нашлось» без цены (Д5), её цену владелец ставит в «Закупках». Сошлось — строка
// акта без движения. Акт не правится и не отменяется, как бумажный: ошибку исправляет корректировка или новый акт. Номер
// — следующий по клубу (1, 2, 3 …); акты проводятся по одному (pg_advisory_xact_lock).
//
// GET /admin/goods/inventories — акты, новые первыми; GET /admin/goods/inventories/:id — акт: строки в порядке
// пересчёта, разницы штуками и по цене продажи (цена в момент акта), у владельца — себестоимость недостачи по партиям,
// из которых ушло, и откуда; админ себестоимости не видит (З-Р29). GET /admin/goods/inventories/:id/export — файл акта
// движком выгрузки З13 (workbook.go; владелец): лист «Акт» — реквизиты, номер, кто и когда, итоги, как читать; лист
// «Позиции» — таблица с итогом.
//
// «Потери → по инвентаризациям» — losses.inventories в «Деньгах» (reports.go): недостачи актов — корректировки, в
// «Потерях» склада они уже есть; здесь — какая их часть пришла из актов, по актам.

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxInventoryLines = 1000 // строк в акте: ценник клуба — сотни позиций
	maxInventoryList  = 100  // актов в списке и в «Потерях → по инвентаризациям»
	// inventoryLock — ключ pg_advisory_xact_lock: акты проводятся по одному — номер следующий, замки позиций двух
	// пересчётов разом не переплетаются
	inventoryLock int64 = 14480823
)

var (
	errStockMoved = errors.New("stock_moved")
	errInvGood    = errors.New("inv_good")
	errInvLot     = errors.New("inv_lot")
)

func init() {
	goodErrors["bad_lines"] = fmt.Sprintf("В акте — от 1 до %d позиций", maxInventoryLines)
	goodErrors["bad_line"] = fmt.Sprintf("У строки акта — позиция, «в системе» и «насчитали» от 0 до %d штук", maxStockQty)
	goodErrors["dup_line"] = "Позиция в акте дважды — оставь одну строку"
	goodErrors["bad_inv_lot"] = "Партию выбирают только у недостачи — насчитали меньше, чем в системе, — и только партию этой позиции"
	goodErrors["stock_moved"] = "Пока считали, остаток изменился — пересчитай эти позиции или поставь новое «в системе»"
	goodErrors["inventory_busy"] = "Склад как раз менялся продажей — проведи акт ещё раз"
	goodErrors["inventory_not_found"] = "Акт инвентаризации не найден"
	goodErrors["no_club"] = "Клуб не найден"
}

type inventoryLineReq struct {
	GoodID   string  `json:"good_id"`
	Expected *int    `json:"expected"`
	Counted  *int    `json:"counted"`
	LotID    *string `json:"lot_id"` // недостача — из этой партии (владелец)
}

type inventoryRequest struct {
	Note  string             `json:"note"`
	Lines []inventoryLineReq `json:"lines"`
}

// invLine — строка акта после разбора запроса.
type invLine struct {
	GoodID   uuid.UUID
	Expected int
	Counted  int
	Lot      *uuid.UUID
}

// inventoryLines — разбор строк акта (чистая, тест): от 1 до maxInventoryLines строк; у каждой — позиция, «в системе» и
// «насчитали» от 0 до maxStockQty; позиция — один раз; партия — только у владельца (owner) и только у недостачи
// (насчитали меньше, чем в системе). Ошибка — код и номер строки с нуля (−1 — про весь акт).
func inventoryLines(req []inventoryLineReq, owner bool) ([]invLine, string, int) {
	if len(req) == 0 || len(req) > maxInventoryLines {
		return nil, "bad_lines", -1
	}
	out, seen := make([]invLine, 0, len(req)), map[uuid.UUID]bool{}
	for i, r := range req {
		id, err := uuid.Parse(strings.TrimSpace(r.GoodID))
		if err != nil || r.Expected == nil || r.Counted == nil || *r.Expected < 0 || *r.Counted < 0 ||
			*r.Expected > maxStockQty || *r.Counted > maxStockQty {
			return nil, "bad_line", i
		}
		if seen[id] {
			return nil, "dup_line", i
		}
		seen[id] = true
		l := invLine{GoodID: id, Expected: *r.Expected, Counted: *r.Counted}
		if r.LotID != nil && strings.TrimSpace(*r.LotID) != "" {
			if !owner {
				return nil, "lot_owner_only", i
			}
			lid, err := uuid.Parse(strings.TrimSpace(*r.LotID))
			if err != nil || l.Counted >= l.Expected {
				return nil, "bad_inv_lot", i
			}
			l.Lot = &lid
		}
		out = append(out, l)
	}
	return out, "", -1
}

// invMoveNote — примечание корректировки из акта (чистая, тест): «инвентаризация №12» и заметка акта.
func invMoveNote(no int, note string) string {
	return fmt.Sprintf("инвентаризация №%d", no) + noteSuffix(note)
}

// invLot — откуда ушла недостача или куда лёг излишек: вид партии, когда она пришла (у поставки — приёмка), штук (по
// модулю), цена за штуку в грошах (nil — без цены).
type invLot struct {
	Kind  string
	At    time.Time
	Qty   int
	Grosz *int64
}

// invRow — строка акта для ответа и файла.
type invRow struct {
	GoodID            *uuid.UUID
	Name, Category    string
	Expected, Counted int
	Grosz             int64 // цена единицы в момент акта, гроши
	Lots              []invLot
}

func (r invRow) delta() int { return r.Counted - r.Expected }

// cost — себестоимость недостачи по партиям с ценой (гроши) и сколько её штук ушло из партий без цены (чистая, тест);
// у излишка и сошедшейся строки — 0 и 0.
func (r invRow) cost() (int64, int) {
	if r.delta() >= 0 {
		return 0, 0
	}
	var g int64
	costed := 0
	for _, l := range r.Lots {
		if l.Grosz != nil {
			g += int64(l.Qty) * *l.Grosz
			costed += l.Qty
		}
	}
	return g, -r.delta() - costed
}

// invSum — итоги акта: позиций, сошлось, недостача и излишек — позициями, штуками и по цене продажи; себестоимость
// недостачи и её штуки без цены; «в системе» и «насчитали» суммой.
type invSum struct {
	Positions, Matched, ShortPos, SurplusPos int
	ShortUnits, SurplusUnits                 int
	ShortGrosz, SurplusGrosz                 int64
	Expected, Counted                        int
	Cost                                     int64
	NoCost                                   int
}

// invTotals — итоги строк акта (чистая, тест).
func invTotals(rows []invRow) invSum {
	var s invSum
	for _, r := range rows {
		s.Positions++
		s.Expected += r.Expected
		s.Counted += r.Counted
		switch d := r.delta(); {
		case d == 0:
			s.Matched++
		case d < 0:
			c, n := r.cost()
			s.ShortPos, s.ShortUnits, s.ShortGrosz = s.ShortPos+1, s.ShortUnits-d, s.ShortGrosz-int64(d)*r.Grosz
			s.Cost, s.NoCost = s.Cost+c, s.NoCost+n
		default:
			s.SurplusPos, s.SurplusUnits, s.SurplusGrosz = s.SurplusPos+1, s.SurplusUnits+d, s.SurplusGrosz+int64(d)*r.Grosz
		}
	}
	return s
}

// invAuditText — строка журнала (чистая, тест) — без денег: журнал корректировок видит и админ.
func invAuditText(no int, s invSum, note string) string {
	t := fmt.Sprintf("Инвентаризация №%d · %d %s: сошлось %d", no, s.Positions, plural(s.Positions, "позиция", "позиции", "позиций"), s.Matched)
	if s.ShortPos > 0 {
		t += fmt.Sprintf(", недостача — %d %s (%d шт)", s.ShortPos, plural(s.ShortPos, "позиция", "позиции", "позиций"), s.ShortUnits)
	}
	if s.SurplusPos > 0 {
		t += fmt.Sprintf(", излишек — %d %s (%d шт)", s.SurplusPos, plural(s.SurplusPos, "позиция", "позиции", "позиций"), s.SurplusUnits)
	}
	return t + noteSuffix(note)
}

// invLotTitle — партия словами (чистая, тест): «поставка от 28.09 18:40», «начальный остаток», «нашлось 11.11 14:05»; по
// lang 1 — по-польски.
func invLotTitle(l invLot, lang int) string {
	at := l.At.In(clubLocation).Format("02.01 15:04")
	switch l.Kind {
	case models.LotInitial:
		return []string{"начальный остаток", "stan początkowy"}[lang]
	case models.LotFound:
		return []string{"нашлось ", "nadwyżka z "}[lang] + at
	}
	return []string{"поставка от ", "dostawa z "}[lang] + at
}

// invLotsText — откуда списана недостача или куда лёг излишек, одной строкой (чистая, тест).
func invLotsText(lots []invLot, lang int) string {
	parts := make([]string, 0, len(lots))
	for _, l := range lots {
		price := []string{"без цены", "bez ceny"}[lang]
		if l.Grosz != nil {
			price = plnRu(*l.Grosz)
		}
		parts = append(parts, fmt.Sprintf("%s: %d × %s", invLotTitle(l, lang), l.Qty, price))
	}
	return strings.Join(parts, "; ")
}

// loadInventoryRows — строки актов ids по порядку пересчёта; lots — и партии их движений (владельцу: себестоимость и
// откуда ушло).
func loadInventoryRows(ids []uuid.UUID, lots bool) map[uuid.UUID][]invRow {
	out := map[uuid.UUID][]invRow{}
	if len(ids) == 0 {
		return out
	}
	var lines []models.InventoryLine
	db.Where("inventory_id IN ?", ids).Order("inventory_id, sort, id").Find(&lines)
	moves := []uuid.UUID{}
	for _, l := range lines {
		if l.StockMoveID != nil {
			moves = append(moves, *l.StockMoveID)
		}
	}
	byMove := map[uuid.UUID][]invLot{}
	if lots && len(moves) > 0 {
		var lm []struct {
			StockMoveID uuid.UUID
			Qty         int
			PriceGrosz  *int64
			Kind        string
			SupplyAt    *time.Time
			ReceivedAt  time.Time
		}
		db.Raw(`SELECT lm.stock_move_id, lm.qty, l.price_grosz, l.kind, s.created_at AS supply_at, l.received_at
		          FROM lot_moves lm JOIN stock_lots l ON l.id = lm.lot_id LEFT JOIN supplies s ON s.id = l.supply_id
		         WHERE lm.stock_move_id IN ? ORDER BY l.received_at, l.part, l.id`, moves).Scan(&lm)
		for _, x := range lm {
			at, q := x.ReceivedAt, x.Qty
			if x.SupplyAt != nil {
				at = *x.SupplyAt
			}
			if q < 0 {
				q = -q
			}
			byMove[x.StockMoveID] = append(byMove[x.StockMoveID], invLot{Kind: x.Kind, At: at, Qty: q, Grosz: x.PriceGrosz})
		}
	}
	for _, l := range lines {
		r := invRow{GoodID: l.GoodID, Name: l.Name, Category: l.Category, Expected: l.Expected, Counted: l.Counted,
			Grosz: int64(math.Round(l.PricePLN * 100))}
		if l.StockMoveID != nil {
			r.Lots = byMove[*l.StockMoveID]
		}
		out[l.InventoryID] = append(out[l.InventoryID], r)
	}
	return out
}

// invSumOut — итоги акта в ответе; себестоимость — владельцу.
func invSumOut(s invSum, owner bool) gin.H {
	out := gin.H{"positions": s.Positions, "matched": s.Matched, "short_positions": s.ShortPos, "short_units": s.ShortUnits,
		"short_pln": models.PLNFromGrosz(s.ShortGrosz), "surplus_positions": s.SurplusPos, "surplus_units": s.SurplusUnits,
		"surplus_pln": models.PLNFromGrosz(s.SurplusGrosz), "net_pln": models.PLNFromGrosz(s.SurplusGrosz - s.ShortGrosz)}
	if owner {
		out["cost_pln"], out["no_cost_units"] = models.PLNFromGrosz(s.Cost), s.NoCost
	}
	return out
}

// inventoryOut — акт в ответе (чистая, тест): шапка, строки по порядку пересчёта, итоги; owner — с себестоимостью
// недостачи (ни одной штуки из партий с ценой — null) и партиями.
func inventoryOut(inv models.Inventory, by string, rows []invRow, owner bool) gin.H {
	lines := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		d := r.delta()
		x := gin.H{"good_id": r.GoodID, "name": r.Name, "category": r.Category, "expected": r.Expected, "counted": r.Counted,
			"delta": d, "price_pln": models.PLNFromGrosz(r.Grosz), "delta_pln": models.PLNFromGrosz(int64(d) * r.Grosz)}
		if owner {
			x["cost_pln"], x["no_cost_units"] = nil, 0
			if c, n := r.cost(); d < 0 {
				x["no_cost_units"] = n
				if n < -d {
					x["cost_pln"] = models.PLNFromGrosz(c)
				}
			}
			ls := make([]gin.H, 0, len(r.Lots))
			for _, l := range r.Lots {
				var p any
				if l.Grosz != nil {
					p = models.PLNFromGrosz(*l.Grosz)
				}
				ls = append(ls, gin.H{"title": invLotTitle(l, 0), "kind": l.Kind, "qty": l.Qty, "price_pln": p})
			}
			x["lots"] = ls
		}
		lines = append(lines, x)
	}
	return gin.H{"id": inv.ID, "no": inv.No, "created_at": inv.CreatedAt, "by": by, "note": inv.Note, "lines": lines,
		"totals": invSumOut(invTotals(rows), owner)}
}

func isOwnerCtx(c *gin.Context) bool { return c.GetString("user_role") == string(models.UserRoleOwner) }

// inventoryAct — акт из базы для ответа.
func inventoryAct(inv models.Inventory, owner bool) gin.H {
	by := ""
	if inv.CreatedBy != nil {
		by = nicknamesByID([]string{inv.CreatedBy.String()})[inv.CreatedBy.String()]
	}
	return inventoryOut(inv, by, loadInventoryRows([]uuid.UUID{inv.ID}, owner)[inv.ID], owner)
}

// invFail — отказ по акту: код, сообщение, номер строки (с нуля) и что ещё нужно экрану.
func invFail(c *gin.Context, status int, code string, line int, extra gin.H) {
	out := gin.H{"code": code, "message": goodErrors[code]}
	if line >= 0 {
		out["line"] = line
	}
	for k, v := range extra {
		out[k] = v
	}
	c.JSON(status, out)
}

// POST /admin/goods/inventory — провести акт пересчёта (персонал; партию недостачи выбирает владелец).
func handleInventoryCreate(c *gin.Context) {
	var req inventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		goodFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	owner := isOwnerCtx(c)
	lines, code, at := inventoryLines(req.Lines, owner)
	if code != "" {
		status := http.StatusBadRequest
		if code == "lot_owner_only" {
			status = http.StatusForbidden
		}
		invFail(c, status, code, at, nil)
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > maxStockNote {
		goodFail(c, http.StatusBadRequest, "bad_note")
		return
	}
	club, ok := defaultClub()
	if !ok {
		goodFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	inv := models.Inventory{ClubID: club.ID, Note: note, CreatedBy: &adminID}
	var moved []gin.H
	bad, short := -1, gin.H{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", inventoryLock).Error; err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(lines))
		for i, l := range lines {
			ids[i] = l.GoodID
		}
		var goods []models.Good // под замком: продажа и приход этих позиций подождут конца акта
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, name, category, stock, price_pln").
			Where("id IN ?", ids).Order("id").Find(&goods).Error; err != nil {
			return err
		}
		byID := make(map[uuid.UUID]models.Good, len(goods))
		for _, g := range goods {
			byID[g.ID] = g
		}
		for i, l := range lines {
			g, ok := byID[l.GoodID]
			if !ok {
				bad = i
				return errInvGood
			}
			if g.Stock != l.Expected {
				moved = append(moved, gin.H{"line": i, "good_id": g.ID, "name": g.Name, "expected": l.Expected, "stock": g.Stock})
			}
		}
		if len(moved) > 0 {
			return errStockMoved
		}
		for i, l := range lines { // выбранная партия — этой позиции, и в ней не меньше недостачи
			if l.Lot == nil {
				continue
			}
			var lot models.StockLot
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lot, "id = ? AND good_id = ?", *l.Lot, l.GoodID).Error; err != nil {
				bad = i
				return errInvLot
			}
			if lot.QtyLeft < l.Expected-l.Counted {
				bad, short = i, gin.H{"left": lot.QtyLeft, "message": fmt.Sprintf("«%s»: в партии «%s» осталось %d шт — меньше недостачи (%d)",
					byID[l.GoodID].Name, lotTitle(lot, supplyTimes(lot.SupplyID)), lot.QtyLeft, l.Expected-l.Counted)}
				return errLotShort
			}
		}
		if err := tx.Raw("SELECT COALESCE(MAX(no), 0) + 1 FROM inventories WHERE club_id = ?", club.ID).Row().Scan(&inv.No); err != nil {
			return err
		}
		if err := tx.Create(&inv).Error; err != nil {
			return err
		}
		moveNote := invMoveNote(inv.No, note)
		for i, l := range lines {
			g := byID[l.GoodID]
			line := models.InventoryLine{InventoryID: inv.ID, GoodID: &g.ID, Name: g.Name, Category: g.Category,
				Expected: l.Expected, Counted: l.Counted, PricePLN: g.PricePLN, Sort: i + 1}
			if d := l.Counted - l.Expected; d != 0 {
				if err := stockMove(tx, stockOp{ClubID: club.ID, GoodID: g.ID, Delta: d, Reason: "adjust", Note: moveNote,
					By: &adminID, Price: g.PricePLN, Cap: true, Lot: l.Lot, InventoryID: &inv.ID}); err != nil {
					return err
				}
				var mid uuid.UUID
				if err := tx.Raw("SELECT id FROM stock_moves WHERE inventory_id = ? AND good_id = ?", inv.ID, g.ID).Row().Scan(&mid); err != nil {
					return err
				}
				line.StockMoveID = &mid
			}
			if err := tx.Create(&line).Error; err != nil {
				return err
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, errStockMoved):
		names := make([]string, 0, 5)
		for i, m := range moved {
			if i == 5 {
				names = append(names, fmt.Sprintf("и ещё %d", len(moved)-5))
				break
			}
			names = append(names, fmt.Sprintf("«%s» — было %d, стало %d", m["name"], m["expected"], m["stock"]))
		}
		invFail(c, http.StatusConflict, "stock_moved", -1, gin.H{"lines": moved,
			"message": "Пока считали, остаток изменился: " + strings.Join(names, ", ") + ". Пересчитай эти позиции или поставь новое «в системе»"})
		return
	case errors.Is(err, errInvGood):
		invFail(c, http.StatusNotFound, "good_not_found", bad, gin.H{"message": "Позиция не найдена — её удалили, пока считали?"})
		return
	case errors.Is(err, errInvLot):
		invFail(c, http.StatusBadRequest, "bad_inv_lot", bad, nil)
		return
	case errors.Is(err, errLotShort):
		invFail(c, http.StatusConflict, "lot_short", bad, short)
		return
	case err != nil && strings.Contains(err.Error(), "40P01"): // взаимная блокировка с чеком из нескольких позиций
		goodFail(c, http.StatusConflict, "inventory_busy")
		return
	case err != nil:
		goodFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	rows := loadInventoryRows([]uuid.UUID{inv.ID}, owner)[inv.ID]
	logAdminAction(c, "inventory", nil, invAuditText(inv.No, invTotals(rows), note))
	by := nicknamesByID([]string{adminID.String()})[adminID.String()]
	c.JSON(http.StatusCreated, gin.H{"inventory": inventoryOut(inv, by, rows, owner)})
}

// GET /admin/goods/inventories — акты, новые первыми (персонал; последние maxInventoryList): номер, когда, кто,
// заметка, итоги; себестоимость — владельцу.
func handleInventories(c *gin.Context) {
	owner := isOwnerCtx(c)
	var invs []models.Inventory
	db.Order("created_at DESC, no DESC").Limit(maxInventoryList).Find(&invs)
	ids, users := make([]uuid.UUID, 0, len(invs)), []string{}
	for _, i := range invs {
		ids = append(ids, i.ID)
		if i.CreatedBy != nil {
			users = append(users, i.CreatedBy.String())
		}
	}
	rows, nick := loadInventoryRows(ids, owner), nicknamesByID(users)
	out := make([]gin.H, 0, len(invs))
	for _, i := range invs {
		out = append(out, gin.H{"id": i.ID, "no": i.No, "created_at": i.CreatedAt, "by": nickOf(nick, i.CreatedBy), "note": i.Note,
			"totals": invSumOut(invTotals(rows[i.ID]), owner)})
	}
	c.JSON(http.StatusOK, gin.H{"inventories": out, "count": len(out)})
}

// inventoryByParam — акт по :id; нет — 404 и false.
func inventoryByParam(c *gin.Context) (models.Inventory, bool) {
	var inv models.Inventory
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || db.First(&inv, "id = ?", id).Error != nil {
		goodFail(c, http.StatusNotFound, "inventory_not_found")
		return inv, false
	}
	return inv, true
}

// GET /admin/goods/inventories/:id — акт (персонал; себестоимость и партии — владельцу).
func handleInventory(c *gin.Context) {
	if inv, ok := inventoryByParam(c); ok {
		c.JSON(http.StatusOK, gin.H{"inventory": inventoryAct(inv, isOwnerCtx(c))})
	}
}

// ── файл акта: движок выгрузки З13 ──────────────────────────────────────────────────────────────

// invWords — слова файла акта: [русский, польский].
var invWords = map[string][2]string{
	"sheet_about": {"Акт", "Arkusz spisu"}, "sheet_lines": {"Позиции", "Pozycje"},
	"club": {"Клуб", "Klub"}, "firm": {"Фирма", "Firma"}, "nip": {"NIP", "NIP"}, "address": {"Адрес", "Adres"},
	"act": {"Акт инвентаризации", "Arkusz spisu z natury"}, "act_v": {"№ %d", "nr %d"},
	"at": {"Дата и время пересчёта", "Data i godzina spisu"}, "by": {"Провёл", "Spis przeprowadził"},
	"note":      {"Заметка", "Uwagi"},
	"positions": {"Позиций посчитано", "Pozycji spisanych"}, "matched": {"Сошлось, позиций", "Zgodne — pozycji"},
	"short_pos": {"Недостача, позиций", "Niedobór — pozycji"}, "short_units": {"Недостача, штук", "Niedobór — sztuk"},
	"short_pln":   {"Недостача по цене продажи", "Niedobór wg cen sprzedaży"},
	"short_cost":  {"Недостача по себестоимости", "Niedobór wg cen zakupu"},
	"no_cost":     {"Недостача без себестоимости, штук", "Niedobór bez ceny zakupu — sztuk"},
	"surplus_pos": {"Излишек, позиций", "Nadwyżka — pozycji"}, "surplus_units": {"Излишек, штук", "Nadwyżka — sztuk"},
	"surplus_pln": {"Излишек по цене продажи", "Nadwyżka wg cen sprzedaży"},
	"made":        {"Файл сделан", "Wygenerowano"},
	"how":         {"Как читать", "Jak czytać"},
	"how_v": {"«В системе» — остаток по программе, когда начали считать; «Насчитали» — сколько нашли; разница — «Насчитали» " +
		"минус «В системе». Недостача списана корректировкой с номером акта — со старых партий первыми или из партии, " +
		"выбранной владельцем; по себестоимости — по цене этих партий. Излишек принят партией «нашлось» без цены закупки.",
		"„Stan ewidencyjny” — stan w programie w chwili rozpoczęcia spisu; „Stan rzeczywisty” — ile znaleziono; różnica — " +
			"stan rzeczywisty minus stan ewidencyjny. Niedobór zdjęto korektą z numerem arkusza — najpierw z najstarszych partii " +
			"(FIFO) albo z partii wybranej przez właściciela; wg cen zakupu — po cenach tych partii. Nadwyżkę przyjęto jako " +
			"partię bez ceny zakupu."},
	"h_n": {"№", "Lp."}, "h_good": {"Позиция", "Nazwa towaru"}, "h_unit": {"Ед.", "J.m."}, "unit": {"шт", "szt."},
	"h_cat": {"Категория", "Kategoria"}, "h_expected": {"В системе", "Stan ewidencyjny"},
	"h_counted": {"Насчитали", "Stan rzeczywisty"}, "h_delta": {"Разница", "Różnica"},
	"h_price": {"Цена продажи", "Cena sprzedaży"}, "h_delta_pln": {"Разница по цене продажи", "Wartość różnicy wg cen sprzedaży"},
	"h_cost": {"Недостача по себестоимости", "Wartość niedoboru wg cen zakupu"},
	"h_lots": {"Откуда списано, куда легло", "Partie"}, "h_id": {"ID позиции", "ID towaru"},
	"total": {"Итого", "Razem"},
}

func iw(lang int, key string) string { return invWords[key][lang] }

// invFirm — реквизиты в шапке акта: клуб, фирма, NIP, адрес.
type invFirm struct{ Club, Firm, NIP, Address string }

// inventoryAbout — лист «Акт» (чистая, тест): реквизиты, номер, когда и кто провёл, заметка, итоги, когда и кем сделан
// файл, как читать.
func inventoryAbout(f invFirm, inv models.Inventory, by string, s invSum, made time.Time, madeBy string, lang int) xsheet {
	sh := xsheet{name: iw(lang, "sheet_about"), file: []string{"akt", "arkusz"}[lang],
		head: []string{tr(lang, "h_item"), tr(lang, "h_value")}, widths: []int{36, 110}}
	add := func(key string, v xcell) { sh.rows = append(sh.rows, xrow{cells: []xcell{cText(iw(lang, key)), v}}) }
	num := func(n int) xcell { return cInt(int64(n)) }
	add("club", cText(f.Club))
	add("firm", cText(f.Firm))
	add("nip", cText(f.NIP))
	add("address", cText(f.Address))
	add("act", cText(fmt.Sprintf(iw(lang, "act_v"), inv.No)))
	add("at", cText(inv.CreatedAt.In(clubLocation).Format("02.01.2006 15:04")))
	add("by", cText(by))
	if inv.Note != "" {
		add("note", cText(inv.Note))
	}
	add("positions", num(s.Positions))
	add("matched", num(s.Matched))
	add("short_pos", num(s.ShortPos))
	add("short_units", num(s.ShortUnits))
	add("short_pln", cMoney(s.ShortGrosz))
	add("short_cost", cMoney(s.Cost))
	if s.NoCost > 0 {
		add("no_cost", num(s.NoCost))
	}
	add("surplus_pos", num(s.SurplusPos))
	add("surplus_units", num(s.SurplusUnits))
	add("surplus_pln", cMoney(s.SurplusGrosz))
	add("made", cText(made.In(clubLocation).Format("02.01.2006 15:04")+" · "+madeBy))
	add("how", cText(iw(lang, "how_v")))
	sh.n = len(sh.rows)
	return sh
}

// inventorySheet — лист «Позиции» (чистая, тест): №, позиция, ед., категория, в системе, насчитали, разница, цена
// продажи, разница по цене продажи, себестоимость недостачи (ни одной штуки из партий с ценой — пусто), партии, ID;
// внизу — «Итого».
func inventorySheet(rows []invRow, s invSum, lang int) xsheet {
	sh := xsheet{name: iw(lang, "sheet_lines"), file: []string{"pozicii", "pozycje"}[lang], table: true,
		head: []string{iw(lang, "h_n"), iw(lang, "h_good"), iw(lang, "h_unit"), iw(lang, "h_cat"), iw(lang, "h_expected"),
			iw(lang, "h_counted"), iw(lang, "h_delta"), iw(lang, "h_price"), iw(lang, "h_delta_pln"), iw(lang, "h_cost"),
			iw(lang, "h_lots"), iw(lang, "h_id")},
		widths: []int{5, 30, 6, 16, 12, 12, 9, 12, 16, 16, 48, 38}}
	for i, r := range rows {
		d, cost, id := r.delta(), xcell{}, ""
		if c, n := r.cost(); d < 0 && n < -d {
			cost = cMoney(c)
		}
		if r.GoodID != nil {
			id = r.GoodID.String()
		}
		sh.rows = append(sh.rows, xrow{cells: []xcell{cInt(int64(i + 1)), cText(r.Name), cText(iw(lang, "unit")), cText(r.Category),
			cInt(int64(r.Expected)), cInt(int64(r.Counted)), cInt(int64(d)), cMoney(r.Grosz), cMoney(int64(d) * r.Grosz), cost,
			cText(invLotsText(r.Lots, lang)), cText(id)}})
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(iw(lang, "total")), {}, {}, {}, cInt(int64(s.Expected)),
		cInt(int64(s.Counted)), cInt(int64(s.Counted - s.Expected)), {}, cMoney(s.SurplusGrosz - s.ShortGrosz), cMoney(s.Cost)}})
	return sh
}

// inventoryFileName — имя файла акта (чистая, тест): «1448-arkusz-spisu-12-2026-11-11.xlsx», по-русски —
// «1448-akt-inventarizacii-12-…»; архив CSV — «.zip».
func inventoryFileName(no int, at time.Time, format string, lang int) string {
	ext := ".xlsx"
	if format == "csv" {
		ext = ".zip"
	}
	return fmt.Sprintf("1448-%s-%d-%s%s", []string{"akt-inventarizacii", "arkusz-spisu"}[lang], no, at.In(clubLocation).Format("2006-01-02"), ext)
}

// GET /admin/goods/inventories/:id/export?format=xlsx|csv&lang=pl|ru — файл акта (владелец): XLSX одним файлом или
// архив CSV — как выгрузка бухгалтеру (разделитель, десятичный знак, кодировка, даты — её настройками).
func handleInventoryExport(c *gin.Context) {
	inv, ok := inventoryByParam(c)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "xlsx")))
	if format != "xlsx" && format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_format", "message": "Формат: xlsx или csv"})
		return
	}
	lang := 1
	switch strings.ToLower(strings.TrimSpace(c.DefaultQuery("lang", "pl"))) {
	case "pl":
	case "ru":
		lang = 0
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_lang", "message": "Язык заголовков: pl или ru"})
		return
	}
	rows := loadInventoryRows([]uuid.UUID{inv.ID}, true)[inv.ID]
	var club models.Club
	db.Where("is_active = ?", true).Order("name").First(&club) // тот же «первый активный клуб», что у выгрузки
	req := requisitesNow()
	firm, address := firmOf(req, club)
	nip := ta(lang, "nip_none")
	if req.NIP != "" {
		nip = req.NIP
	}
	me, users := c.GetString("user_id"), []string{c.GetString("user_id")}
	if inv.CreatedBy != nil {
		users = append(users, inv.CreatedBy.String())
	}
	nick, now, s := nicknamesByID(users), time.Now(), invTotals(rows)
	sheets := []xsheet{inventoryAbout(invFirm{club.Name, firm, nip, address}, inv, nickOf(nick, inv.CreatedBy), s, now, nick[me], lang),
		inventorySheet(rows, s, lang)}
	var buf bytes.Buffer
	var err error
	ctype := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if format == "xlsx" {
		err = writeXLSX(&buf, sheets, exportFormat(now))
	} else {
		ctype = "application/zip"
		err = writeCSVZip(&buf, sheets, exportFormat(now))
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "export_failed", "message": "Файл не собрался — попробуй ещё раз"})
		return
	}
	logAdminAction(c, "inventory_export", nil, fmt.Sprintf("акт инвентаризации №%d · %s · %s", inv.No, strings.ToUpper(format), []string{"RU", "PL"}[lang]))
	c.Header("Content-Disposition", `attachment; filename="`+inventoryFileName(inv.No, inv.CreatedAt, format, lang)+`"`)
	c.Data(http.StatusOK, ctype, buf.Bytes())
}

// ── «Потери → по инвентаризациям» ───────────────────────────────────────────────────────────────

// inventoryLossesIn — акты за [from, to) для «Денег» (владелец): сколько, недостача штуками, по цене продажи (цена
// движения, как «Потери», З6-и2) и по себестоимости партий (З10-и7), её штуки без цены, излишек; по актам — номер,
// когда, кто и то же (последние maxInventoryList).
func inventoryLossesIn(from, to time.Time) gin.H {
	var acts []struct {
		ID           uuid.UUID
		No           int
		CreatedAt    time.Time
		CreatedBy    *uuid.UUID
		Positions    int
		ShortUnits   int
		ShortGrosz   int64
		SurplusUnits int
		SurplusGrosz int64
	}
	db.Raw(`SELECT i.id, i.no, i.created_at, i.created_by,
	               (SELECT count(*) FROM inventory_lines il WHERE il.inventory_id = i.id) AS positions,
	               COALESCE(SUM(-m.delta) FILTER (WHERE m.delta < 0), 0) AS short_units,
	               COALESCE(ROUND(SUM(-m.delta * m.price_pln * 100) FILTER (WHERE m.delta < 0)), 0)::bigint AS short_grosz,
	               COALESCE(SUM(m.delta) FILTER (WHERE m.delta > 0), 0) AS surplus_units,
	               COALESCE(ROUND(SUM(m.delta * m.price_pln * 100) FILTER (WHERE m.delta > 0)), 0)::bigint AS surplus_grosz
	          FROM inventories i LEFT JOIN stock_moves m ON m.inventory_id = i.id
	         WHERE i.created_at >= ? AND i.created_at < ?
	         GROUP BY i.id ORDER BY i.created_at DESC, i.no DESC`, from, to).Scan(&acts)
	ids, users := make([]uuid.UUID, 0, len(acts)), []string{}
	for _, a := range acts {
		ids = append(ids, a.ID)
		if a.CreatedBy != nil {
			users = append(users, a.CreatedBy.String())
		}
	}
	cost := map[uuid.UUID][2]int64{} // себестоимость недостачи, гроши, и сколько её штук — из партий с ценой
	if len(ids) > 0 {
		var cs []struct {
			InventoryID uuid.UUID
			Cost        int64
			Costed      int64
		}
		db.Raw(`SELECT m.inventory_id, COALESCE(SUM(-lm.qty * l.price_grosz), 0)::bigint AS cost, COALESCE(SUM(-lm.qty), 0)::bigint AS costed
		          FROM stock_moves m JOIN lot_moves lm ON lm.stock_move_id = m.id JOIN stock_lots l ON l.id = lm.lot_id
		         WHERE m.inventory_id IN ? AND m.delta < 0 AND l.price_grosz IS NOT NULL
		         GROUP BY m.inventory_id`, ids).Scan(&cs)
		for _, x := range cs {
			cost[x.InventoryID] = [2]int64{x.Cost, x.Costed}
		}
	}
	nick := nicknamesByID(users)
	var su, pu, nc int
	var sg, pg, cg int64
	items := make([]gin.H, 0, len(acts))
	for _, a := range acts {
		k := cost[a.ID]
		noCost := a.ShortUnits - int(k[1])
		su, sg, pu, pg, cg, nc = su+a.ShortUnits, sg+a.ShortGrosz, pu+a.SurplusUnits, pg+a.SurplusGrosz, cg+k[0], nc+noCost
		if len(items) < maxInventoryList {
			items = append(items, gin.H{"id": a.ID, "no": a.No, "created_at": a.CreatedAt, "by": nickOf(nick, a.CreatedBy),
				"positions": a.Positions, "short_units": a.ShortUnits, "short_pln": models.PLNFromGrosz(a.ShortGrosz),
				"cost_pln": models.PLNFromGrosz(k[0]), "no_cost_units": noCost, "surplus_units": a.SurplusUnits,
				"surplus_pln": models.PLNFromGrosz(a.SurplusGrosz)})
		}
	}
	return gin.H{"acts": len(acts), "short_units": su, "short_pln": models.PLNFromGrosz(sg), "cost_pln": models.PLNFromGrosz(cg),
		"no_cost_units": nc, "surplus_units": pu, "surplus_pln": models.PLNFromGrosz(pg), "items": items}
}
