package main

// З10-и4 (OWNER.md, трек З; №78; решения основателя 28.09): приёмка поставки.
// Админ или владелец принимает поставку строками «товар × штук» с заметкой:
// остаток растёт, у каждой строки — партия без цены, поставка «ждёт цен», в
// канал всем — «Принята поставка: Cola ×30, Lay's ×20 · admin1». Пока поставка
// ждёт цен, её можно исправить: штуки в строке (не меньше того, что из строки
// уже ушло) или отменить целиком, если из неё ничего не ушло; у исправленной —
// пометка «исправлено когда и кем», что именно — в журнале и в строках («было»).
// Правка и отмена — движения прихода со знаком по партии своей строки: в
// «Потери» не идут. Цен закупки в ответах нет ни у кого: их даст оценка
// (З10-и5), админ их не увидит нигде. Владельцу — число «ждут цен» (overview,
// бейдж «Бухгалтерии» вместе с регулярными расходами).

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
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxSupplyLines = 50 // строк в одной поставке

var supplyErrors = map[string]string{
	"invalid_request":  "Некорректный запрос",
	"bad_lines":        fmt.Sprintf("Строки поставки: позиция и штуки, от 1 до %d строк, позиция — один раз", maxSupplyLines),
	"bad_qty":          fmt.Sprintf("Штук в строке — от 1 до %d", maxStockQty),
	"bad_note":         fmt.Sprintf("Заметка — до %d символов", maxStockNote),
	"supply_not_found": "Поставка не найдена",
	"supply_priced":    "Поставку уже оценили — исправить штуки нельзя",
	"supply_cancelled": "Поставка отменена",
	"line_unknown":     "Этой позиции в поставке нет — прими её отдельной поставкой",
	"supply_empty":     "Во всех строках ноль — отмени поставку целиком",
	"supply_sold":      "Из поставки уже ушёл товар — отменить целиком нельзя, исправь штуки",
	"line_sold":        "Из строки уже ушло больше — меньше нельзя",
	"stock_limit":      fmt.Sprintf("Остаток позиции вышел бы больше %d шт", maxStockQty),
	"db_error":         "Не получилось, попробуй ещё раз",
	"no_club":          "Клуб не настроен",
	"good_not_found":   "Позиция не найдена",
}

var errSupplyState = errors.New("supply_state")

// supplyRefusal — отказ правки или отмены, найденный под замком поставки: ответ
// целиком (код, сообщение, сколько ушло).
type supplyRefusal struct {
	status int
	body   gin.H
}

func (e *supplyRefusal) Error() string { return fmt.Sprint(e.body["code"]) }

func supplyRefuse(status int, code string) *supplyRefusal {
	return &supplyRefusal{status, gin.H{"code": code, "message": supplyErrors[code]}}
}

var errNoChange = errors.New("no_change")

func supplyFail(c *gin.Context, status int, code string) {
	msg := supplyErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

type supplyLineReq struct {
	GoodID string `json:"good_id"`
	Qty    int    `json:"qty"`
}

type supplyRequest struct {
	Lines []supplyLineReq `json:"lines"`
	Note  *string         `json:"note"`
}

// supplyLines — строки из запроса (чистая, тест): позиция — uuid, штуки — от
// minQty до maxStockQty, строк — от 1 до maxSupplyLines. merge — повтор позиции
// складывается (приёмка); иначе повтор — ошибка (правка задаёт штуки строки).
func supplyLines(req []supplyLineReq, minQty int, merge bool) ([]supplyLineReq, string) {
	if len(req) == 0 || len(req) > maxSupplyLines*4 {
		return nil, "bad_lines"
	}
	out, idx := []supplyLineReq{}, map[string]int{}
	for _, l := range req {
		id := strings.TrimSpace(l.GoodID)
		if _, err := uuid.Parse(id); err != nil {
			return nil, "bad_lines"
		}
		if l.Qty < minQty || l.Qty > maxStockQty {
			return nil, "bad_qty"
		}
		if i, ok := idx[id]; ok {
			if !merge {
				return nil, "bad_lines"
			}
			if out[i].Qty += l.Qty; out[i].Qty > maxStockQty {
				return nil, "bad_qty"
			}
			continue
		}
		idx[id] = len(out)
		out = append(out, supplyLineReq{id, l.Qty})
	}
	if len(out) > maxSupplyLines {
		return nil, "bad_lines"
	}
	return out, ""
}

// supplyTitle — «поставка от 28.09 18:40» для журнала и канала (чистая, тест).
func supplyTitle(at time.Time) string {
	return "поставка от " + at.In(clubLocation).Format("02.01 15:04")
}

// supplyLine — строка поставки в ответе: позиция, сколько сейчас, сколько было
// при приёмке, сколько из строки уже ушло.
type supplyLine struct {
	GoodID   uuid.UUID `json:"good_id"`
	Name     string    `json:"name"`
	Qty      int       `json:"qty"`
	Received int       `json:"received"`
	Gone     int       `json:"gone"`
}

// supplyLinesOf — строки поставок: партии по позициям (после разбивки на цены
// кусков у позиции несколько — складываются), «было» — первое движение прихода
// каждой партии. q — база или транзакция (под замком поставки — её строки как есть).
func supplyLinesOf(q *gorm.DB, ids []uuid.UUID) map[uuid.UUID][]supplyLine {
	out := map[uuid.UUID][]supplyLine{}
	if len(ids) == 0 {
		return out
	}
	var lots []models.StockLot
	q.Where("supply_id IN ?", ids).Order("received_at, part, id").Find(&lots)
	lotIDs, goodIDs := []uuid.UUID{}, []uuid.UUID{}
	for _, l := range lots {
		lotIDs, goodIDs = append(lotIDs, l.ID), append(goodIDs, l.GoodID)
	}
	var firsts []struct {
		LotID uuid.UUID
		Qty   int
	}
	if len(lotIDs) > 0 {
		q.Raw(`SELECT DISTINCT ON (lm.lot_id) lm.lot_id, lm.qty FROM lot_moves lm JOIN stock_moves m ON m.id = lm.stock_move_id
			WHERE lm.lot_id IN ? AND m.reason = 'supply' AND lm.qty > 0 ORDER BY lm.lot_id, m.created_at`, lotIDs).Scan(&firsts)
	}
	first := map[uuid.UUID]int{}
	for _, f := range firsts {
		first[f.LotID] = f.Qty
	}
	var goods []models.Good
	q.Select("id, name").Where("id IN ?", goodIDs).Find(&goods)
	names := map[uuid.UUID]string{}
	for _, g := range goods {
		names[g.ID] = g.Name
	}
	at := map[uuid.UUID]map[uuid.UUID]int{} // поставка → позиция → номер строки
	for _, l := range lots {
		sid := *l.SupplyID
		if at[sid] == nil {
			at[sid] = map[uuid.UUID]int{}
		}
		i, ok := at[sid][l.GoodID]
		if !ok {
			i = len(out[sid])
			at[sid][l.GoodID] = i
			out[sid] = append(out[sid], supplyLine{GoodID: l.GoodID, Name: names[l.GoodID]})
		}
		ln := &out[sid][i]
		ln.Qty += l.QtyIn
		ln.Received += first[l.ID]
		ln.Gone += l.QtyIn - l.QtyLeft
	}
	for sid := range out {
		sort.SliceStable(out[sid], func(a, b int) bool { return strings.ToLower(out[sid][a].Name) < strings.ToLower(out[sid][b].Name) })
	}
	return out
}

// suppliesOut — поставки в ответ: кто и когда принял, заметка, статус, пометка
// «исправлено» и строки. Цен нет ни у кого.
func suppliesOut(list []models.Supply) []gin.H {
	ids, who := []uuid.UUID{}, []string{}
	for _, s := range list {
		ids = append(ids, s.ID)
		if s.ReceivedBy != nil {
			who = append(who, s.ReceivedBy.String())
		}
		if s.EditedBy != nil {
			who = append(who, s.EditedBy.String())
		}
	}
	lines, nick := supplyLinesOf(db, ids), nicknamesByID(who)
	out := make([]gin.H, 0, len(list))
	for _, s := range list {
		ls := lines[s.ID]
		if ls == nil {
			ls = []supplyLine{}
		}
		units := 0
		for _, l := range ls {
			units += l.Qty
		}
		row := gin.H{"id": s.ID, "status": s.Status, "note": s.Note, "created_at": s.CreatedAt, "lines": ls, "units": units}
		if s.ReceivedBy != nil {
			row["received_by"] = nick[s.ReceivedBy.String()]
		}
		if s.EditedAt != nil { // пометка «исправлено» (решение основателя 28.09)
			row["edited_at"] = *s.EditedAt
			if s.EditedBy != nil {
				row["edited_by"] = nick[s.EditedBy.String()]
			}
		}
		out = append(out, row)
	}
	return out
}

// linesText — «Cola ×30, Lay's ×20» для журнала и канала.
func linesText(ls []supplyLine) string {
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		if l.Qty > 0 {
			parts = append(parts, fmt.Sprintf("%s ×%d", l.Name, l.Qty))
		}
	}
	return strings.Join(parts, ", ")
}

func waitingSupplies() int64 {
	var n int64
	db.Model(&models.Supply{}).Where("status = ?", models.SupplyWaiting).Count(&n)
	return n
}

// GET /admin/supplies?status=waiting|all — поставки (staff): по умолчанию все,
// что ждут цен, старые первыми; all — последние 50 любых.
func handleSupplies(c *gin.Context) {
	var list []models.Supply
	q := db.Order("created_at").Where("status = ?", models.SupplyWaiting)
	if c.Query("status") == "all" {
		q = db.Order("created_at DESC").Limit(50)
	}
	if err := q.Find(&list).Error; err != nil {
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"supplies": suppliesOut(list), "count": len(list), "waiting": waitingSupplies()})
}

// POST /admin/supplies — принять поставку (staff): строки «позиция × штук»,
// заметка. Остаток растёт движением прихода по каждой строке, у строки —
// партия без цены, поставка ждёт цен; в канал всем — строка.
func handleSupplyCreate(c *gin.Context) {
	var req supplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		supplyFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	lines, code := supplyLines(req.Lines, 1, true)
	if code != "" {
		supplyFail(c, http.StatusBadRequest, code)
		return
	}
	note := ""
	if req.Note != nil {
		note = strings.TrimSpace(*req.Note)
	}
	if len([]rune(note)) > maxStockNote {
		supplyFail(c, http.StatusBadRequest, "bad_note")
		return
	}
	goods := make([]models.Good, len(lines))
	for i, l := range lines {
		if err := db.First(&goods[i], "id = ?", l.GoodID).Error; err != nil {
			supplyFail(c, http.StatusNotFound, "good_not_found")
			return
		}
	}
	club, ok := defaultClub()
	if !ok {
		supplyFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	s := models.Supply{ClubID: club.ID, Status: models.SupplyWaiting, Note: note, ReceivedBy: &adminID}
	var over *models.Good
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&s).Error; err != nil {
			return err
		}
		for i, l := range lines {
			err := stockMove(tx, stockOp{ClubID: club.ID, GoodID: goods[i].ID, Delta: l.Qty, Reason: "supply", Note: note,
				By: &adminID, Price: goods[i].PricePLN, Cap: true, SupplyID: &s.ID})
			if errors.Is(err, errStockLimit) {
				over = &goods[i]
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errStockLimit) && over != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "stock_limit", "message": fmt.Sprintf("«%s»: остаток вышел бы больше %d шт", over.Name, maxStockQty)})
		return
	}
	if err != nil {
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	out := suppliesOut([]models.Supply{s})[0]
	text := linesText(out["lines"].([]supplyLine))
	logAdminAction(c, "supply_receive", nil, text+noteSuffix(note))
	postSystem("📦 Принята поставка: "+text+" · "+nicknamesByID([]string{adminID.String()})[adminID.String()], false,
		map[string]any{"supply": true, "supply_id": s.ID, "by": adminID})
	c.JSON(http.StatusCreated, gin.H{"supply": out})
}

// supplyForChange — поставка, которую ещё можно исправить (ждёт цен); иначе ответ
// и nil.
func supplyForChange(c *gin.Context) *models.Supply {
	var s models.Supply
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		supplyFail(c, http.StatusNotFound, "supply_not_found")
		return nil
	}
	switch s.Status {
	case models.SupplyPriced:
		supplyFail(c, http.StatusConflict, "supply_priced")
		return nil
	case models.SupplyCancelled:
		supplyFail(c, http.StatusConflict, "supply_cancelled")
		return nil
	}
	return &s
}

// lockWaiting — в транзакции: поставка под замком и всё ещё ждёт цен (две правки
// разом идут по очереди, вторая считает от того, что оставила первая; правка
// после оценки или отмены не пройдёт).
func lockWaiting(tx *gorm.DB, id uuid.UUID) (*models.Supply, error) {
	var s models.Supply
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, "id = ?", id).Error; err != nil {
		return nil, err
	}
	switch s.Status {
	case models.SupplyWaiting:
		return &s, nil
	case models.SupplyCancelled:
		return nil, supplyRefuse(http.StatusConflict, "supply_cancelled")
	}
	return nil, errSupplyState
}

// supplyLots — партии строк поставки по позициям (до оценки — одна на позицию).
func supplyLots(q *gorm.DB, id uuid.UUID) (map[uuid.UUID]models.StockLot, []models.StockLot) {
	var lots []models.StockLot
	q.Where("supply_id = ?", id).Order("received_at, part, id").Find(&lots)
	by := map[uuid.UUID]models.StockLot{}
	for _, l := range lots {
		if _, ok := by[l.GoodID]; !ok {
			by[l.GoodID] = l
		}
	}
	return by, lots
}

// PATCH /admin/supplies/:id — исправить приёмку (staff), пока поставка ждёт цен:
// штуки строк (не меньше ушедшего из строки) и заметка. Пометка «исправлено» —
// когда и кем; в журнал и в канал — что было и что стало. Строки и «сколько ушло»
// читаются под замком поставки: две правки разом не сложатся.
func handleSupplyUpdate(c *gin.Context) {
	s := supplyForChange(c)
	if s == nil {
		return
	}
	var req supplyRequest
	if err := c.ShouldBindJSON(&req); err != nil || (len(req.Lines) == 0 && req.Note == nil) {
		supplyFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	var lines []supplyLineReq
	if len(req.Lines) > 0 {
		var code string
		if lines, code = supplyLines(req.Lines, 0, false); code != "" {
			supplyFail(c, http.StatusBadRequest, code)
			return
		}
	}
	var newNote *string
	if req.Note != nil {
		n := strings.TrimSpace(*req.Note)
		if len([]rune(n)) > maxStockNote {
			supplyFail(c, http.StatusBadRequest, "bad_note")
			return
		}
		newNote = &n
	}
	type change struct {
		lot   models.StockLot
		delta int
	}
	var changes []change
	var names map[uuid.UUID]string
	noteChanged := false
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	var failed string
	err := db.Transaction(func(tx *gorm.DB) error {
		cur, err := lockWaiting(tx, s.ID)
		if err != nil {
			return err
		}
		note := cur.Note
		if newNote != nil && *newNote != cur.Note {
			note, noteChanged = *newNote, true
		}
		names = map[uuid.UUID]string{}
		for _, l := range supplyLinesOf(tx, []uuid.UUID{s.ID})[s.ID] {
			names[l.GoodID] = l.Name
		}
		byGood, all := supplyLots(tx, s.ID)
		total := 0
		for _, l := range all {
			total += l.QtyIn
		}
		for _, ln := range lines {
			gid := uuid.MustParse(ln.GoodID)
			lot, ok := byGood[gid]
			if !ok {
				return supplyRefuse(http.StatusBadRequest, "line_unknown")
			}
			if gone := lot.QtyIn - lot.QtyLeft; ln.Qty < gone {
				return &supplyRefusal{http.StatusConflict, gin.H{"code": "line_sold", "gone": gone,
					"message": fmt.Sprintf("«%s»: из строки уже ушло %d шт — меньше нельзя", names[gid], gone)}}
			}
			if d := ln.Qty - lot.QtyIn; d != 0 {
				changes = append(changes, change{lot, d})
				total += d
			}
		}
		if total <= 0 {
			return supplyRefuse(http.StatusBadRequest, "supply_empty")
		}
		if len(changes) == 0 && !noteChanged {
			return errNoChange
		}
		for _, ch := range changes {
			var g models.Good
			if err := tx.First(&g, "id = ?", ch.lot.GoodID).Error; err != nil {
				return err
			}
			lot := ch.lot.ID
			err := stockMove(tx, stockOp{ClubID: s.ClubID, GoodID: g.ID, Delta: ch.delta, Reason: "supply", Note: "приёмка: правка",
				By: &adminID, Price: g.PricePLN, Cap: ch.delta > 0, Lot: &lot})
			if err != nil {
				failed = g.Name
				return err
			}
		}
		return tx.Model(&models.Supply{}).Where("id = ?", s.ID).
			Updates(map[string]any{"note": note, "edited_at": now, "edited_by": adminID, "updated_at": now}).Error
	})
	var refusal *supplyRefusal
	switch {
	case errors.As(err, &refusal):
		c.JSON(refusal.status, refusal.body)
		return
	case errors.Is(err, errSupplyState):
		supplyFail(c, http.StatusConflict, "supply_priced")
		return
	case errors.Is(err, errLineSold), errors.Is(err, errNegativeStock):
		c.JSON(http.StatusConflict, gin.H{"code": "line_sold", "message": fmt.Sprintf("«%s»: из строки уже ушло больше — меньше нельзя", failed)})
		return
	case errors.Is(err, errStockLimit):
		c.JSON(http.StatusConflict, gin.H{"code": "stock_limit", "message": fmt.Sprintf("«%s»: остаток вышел бы больше %d шт", failed, maxStockQty)})
		return
	case err != nil && !errors.Is(err, errNoChange):
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.Supply
	db.First(&fresh, "id = ?", s.ID)
	if errors.Is(err, errNoChange) {
		c.JSON(http.StatusOK, gin.H{"supply": suppliesOut([]models.Supply{fresh})[0], "changed": 0})
		return
	}
	parts := []string{}
	for _, ch := range changes {
		parts = append(parts, fmt.Sprintf("%s %d → %d", names[ch.lot.GoodID], ch.lot.QtyIn, ch.lot.QtyIn+ch.delta))
	}
	if noteChanged {
		parts = append(parts, "заметка изменена")
	}
	what := strings.Join(parts, ", ")
	logAdminAction(c, "supply_edit", nil, supplyTitle(s.CreatedAt)+": "+what)
	postSystem("✏️ Исправлена "+supplyTitle(s.CreatedAt)+": "+what+" · "+nicknamesByID([]string{adminID.String()})[adminID.String()], false,
		map[string]any{"supply": true, "supply_id": s.ID, "by": adminID})
	c.JSON(http.StatusOK, gin.H{"supply": suppliesOut([]models.Supply{fresh})[0], "changed": len(parts)})
}

// POST /admin/supplies/:id/cancel — отменить приёмку целиком (staff): пока
// поставка ждёт цен и из неё ничего не ушло (проверка — под замком поставки).
// Остаток уменьшается движением прихода со знаком по каждой строке, поставка —
// «отменена», с пометкой.
func handleSupplyCancel(c *gin.Context) {
	s := supplyForChange(c)
	if s == nil {
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	var text string
	err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockWaiting(tx, s.ID); err != nil {
			return err
		}
		_, lots := supplyLots(tx, s.ID)
		gone := 0
		for _, l := range lots {
			gone += l.QtyIn - l.QtyLeft
		}
		if gone > 0 {
			return &supplyRefusal{http.StatusConflict, gin.H{"code": "supply_sold", "gone": gone, "message": supplyErrors["supply_sold"]}}
		}
		text = linesText(supplyLinesOf(tx, []uuid.UUID{s.ID})[s.ID])
		for _, l := range lots {
			if l.QtyIn == 0 {
				continue
			}
			var g models.Good
			if err := tx.First(&g, "id = ?", l.GoodID).Error; err != nil {
				return err
			}
			lot := l.ID
			if err := stockMove(tx, stockOp{ClubID: s.ClubID, GoodID: g.ID, Delta: -l.QtyIn, Reason: "supply", Note: "приёмка: отмена",
				By: &adminID, Price: g.PricePLN, Lot: &lot}); err != nil {
				return err
			}
		}
		return tx.Model(&models.Supply{}).Where("id = ?", s.ID).
			Updates(map[string]any{"status": models.SupplyCancelled, "edited_at": now, "edited_by": adminID, "updated_at": now}).Error
	})
	var refusal *supplyRefusal
	switch {
	case errors.As(err, &refusal):
		c.JSON(refusal.status, refusal.body)
		return
	case errors.Is(err, errSupplyState):
		supplyFail(c, http.StatusConflict, "supply_priced")
		return
	case errors.Is(err, errLineSold), errors.Is(err, errNegativeStock):
		supplyFail(c, http.StatusConflict, "supply_sold")
		return
	case err != nil:
		supplyFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.Supply
	db.First(&fresh, "id = ?", s.ID)
	logAdminAction(c, "supply_cancel", nil, supplyTitle(s.CreatedAt)+": "+text)
	postSystem("🚫 Отменена "+supplyTitle(s.CreatedAt)+": "+text+" · "+nicknamesByID([]string{adminID.String()})[adminID.String()], false,
		map[string]any{"supply": true, "supply_id": s.ID, "by": adminID})
	c.JSON(http.StatusOK, gin.H{"supply": suppliesOut([]models.Supply{fresh})[0]})
}

// overviewSuppliesWaiting — сколько поставок ждут цен: владельцу число, админу —
// nil (цены — дело владельца).
func overviewSuppliesWaiting(c *gin.Context) any {
	if c.GetString("user_role") != string(models.UserRoleOwner) {
		return nil
	}
	return waitingSupplies()
}
