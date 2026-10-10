package main

// З9-и1 «Расходы: схема и API» (OWNER.md, трек З; З-Р30, ревью 23.09 №61, №62,
// №65, №66). Расходы вносит и видит только владелец (З-Р29): все ручки — за
// ownerMiddleware, строки журнала — только владельцу. Удаления нет — сторно:
// строка остаётся с пометкой, кто, когда и почему, и уходит из сумм. Правка —
// с журналом «было → стало». Фото документа — до 500 КБ, как у товара, отдаётся
// только владельцу. Экран — З9-и2, расходы из кассы и долг клуба владельцу —
// З9-и3, справочники (свои категории, скрытие, NIP с контрольной суммой) — З9-и4.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxExpensePLN   = 1_000_000 // один расход — до миллиона злотых
	maxExpenseDoc   = 64        // номер документа
	maxExpenseNote  = 500
	maxStornoReason = 200
	maxCounterparty = 128 // имя контрагента
	maxCPContact    = 200
)

var expenseErrors = map[string]string{
	"invalid_request":     "Нужны дата, категория, сумма и способ оплаты",
	"bad_date":            "Дата операции — ГГГГ-ММ-ДД, не раньше чем 5 лет назад и не позже чем через год",
	"bad_category":        "Такой категории нет",
	"bad_counterparty":    "Такого контрагента нет",
	"bad_amount":          fmt.Sprintf("Сумма брутто — от 0,01 до %d zł, не больше двух знаков после запятой", maxExpensePLN),
	"bad_vat":             "VAT — от 0 до суммы брутто, не больше двух знаков после запятой",
	"bad_method":          "Способ: till — из кассы, bank — банк, card — карта, owner — личные деньги владельца",
	"bad_doc":             fmt.Sprintf("Номер документа — до %d символов", maxExpenseDoc),
	"bad_note":            fmt.Sprintf("Заметка — до %d символов", maxExpenseNote),
	"need_reason":         "У сторно нужна причина",
	"bad_reason":          fmt.Sprintf("Причина сторно — до %d символов", maxStornoReason),
	"expense_not_found":   "Расход не найден",
	"storno":              "Расход сторнирован — его не правят; внеси новый",
	"already_storno":      "Расход уже сторнирован",
	"bad_name":            fmt.Sprintf("Имя контрагента — 1–%d символов", maxCounterparty),
	"bad_nip":             "NIP — 10 цифр",
	"bad_nip_sum":         "NIP не сходится по контрольной сумме — проверь цифры",
	"bad_contact":         fmt.Sprintf("Контакты — до %d символов", maxCPContact),
	"counterparty_exists": "Такой контрагент уже есть",
	"no_club":             "Клуб не настроен",
	"db_error":            "Не удалось записать — попробуй ещё раз",
	"bad_photo":           "Фото — JPEG, PNG или WebP",
	"photo_too_big":       fmt.Sprintf("Фото — до %d КБ (сожми на клиенте)", maxPhotoBytes>>10),
	"photo_not_found":     "У расхода нет фото",
	"bad_closeout":        "«Взяли до сдачи» — только у оплаченного расхода из кассы и только до сдачи с пересчётом того же дня",
	// З9-и4: справочники
	"bad_category_name":      fmt.Sprintf("Название категории — 1–%d символов", maxCategoryName),
	"category_exists":        "Такая категория уже есть",
	"category_not_found":     "Категория не найдена",
	"category_hidden":        "Категория скрыта — верни её в «Справочниках» или выбери другую",
	"purchase_fixed":         "У стандартной категории «закупка» не меняется — по ней «Прибыль» считает себестоимость",
	"last_category":          "Это последняя категория в списке — без неё расход не внести",
	"counterparty_not_found": "Контрагент не найден",
	"method_off":             "Этот способ оплаты выключен — включи его в «Справочниках» или выбери другой",
	"need_method":            "Хотя бы один способ оплаты должен остаться включённым",
	// З9-и5: регулярные расходы
	"bad_template_name":  fmt.Sprintf("Название шаблона — 1–%d символов", maxTemplateName),
	"template_exists":    "Шаблон с таким названием уже есть",
	"template_not_found": "Шаблон не найден",
	"bad_day":            "Число месяца — от 1 до 31 (нет такого в месяце — последний день)",
	"bad_template":       "Такого шаблона нет",
	"bad_template_month": "Месяц шаблона — с первого месяца шаблона по следующий за текущим (заранее)",
	"template_entered":   "За этот месяц по шаблону уже внесено — сторнируй тот расход, если он ошибочный",
}

func expenseFail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"code": code, "message": expenseErrors[code]})
}

// expenseRequest — поля расхода. На создании обязательны дата, категория, сумма
// и способ; на правке каждое поле необязательно — меняется то, что прислано.
type expenseRequest struct {
	OpDate         *string  `json:"op_date"`
	CategoryID     *string  `json:"category_id"`
	CounterpartyID *string  `json:"counterparty_id"` // "" — убрать контрагента
	AmountPLN      *float64 `json:"amount_pln"`
	VatPLN         *float64 `json:"vat_pln"`
	Method         *string  `json:"method"`
	Paid           *bool    `json:"paid"`
	DocNo          *string  `json:"doc_no"`
	Note           *string  `json:"note"`
	// З9-и3в: расход «из кассы» взяли до этой сдачи (id сдачи с пересчётом того же
	// дня) — объясняет её; "" — после сдач: «должно быть» меньше с этой минуты.
	// Нет поля — место по правилу (tillPlace).
	BeforeCloseout *string `json:"before_closeout"`
	// З9-и5: по шаблону регулярного расхода за месяц (только при записи).
	TemplateID    *string `json:"template_id"`
	TemplateMonth *string `json:"template_month"`
}

var expenseMethods = map[string]string{
	models.ExpenseTill: "из кассы", models.ExpenseBank: "банк",
	models.ExpenseCard: "карта", models.ExpenseOwner: "личные деньги владельца",
}

// checkExpense — поля расхода поверх текущих (чистая, тест): cur — расход до
// правки (на создании — пустой), today — сегодня по календарю клуба. Возвращает
// код ошибки и итоговые значения. Категория и контрагент проверяются базой.
func checkExpense(req expenseRequest, cur models.Expense, today time.Time) (string, models.Expense) {
	e := cur
	if req.OpDate != nil {
		d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*req.OpDate), today.Location())
		if err != nil || d.Before(today.AddDate(-5, 0, 0)) || d.After(today.AddDate(1, 0, 0)) {
			return "bad_date", e
		}
		e.OpDate = d
	}
	if req.AmountPLN != nil {
		if !validatePLN(*req.AmountPLN, maxExpensePLN) {
			return "bad_amount", e
		}
		e.GrossGrosz = models.GroszFromPLN(*req.AmountPLN)
	}
	if req.VatPLN != nil {
		v := *req.VatPLN
		if v < 0 || (v > 0 && !validatePLN(v, maxExpensePLN)) {
			return "bad_vat", e
		}
		e.VatGrosz = models.GroszFromPLN(v)
	}
	if e.VatGrosz > e.GrossGrosz {
		return "bad_vat", e
	}
	if req.Method != nil {
		if _, ok := expenseMethods[*req.Method]; !ok {
			return "bad_method", e
		}
		e.Method = *req.Method
	}
	if req.Paid != nil {
		e.Paid = *req.Paid
	}
	if req.DocNo != nil {
		doc := strings.TrimSpace(*req.DocNo)
		if len([]rune(doc)) > maxExpenseDoc {
			return "bad_doc", e
		}
		e.DocNo = doc
	}
	if req.Note != nil {
		note := strings.TrimSpace(*req.Note)
		if len([]rune(note)) > maxExpenseNote {
			return "bad_note", e
		}
		e.Note = note
	}
	if e.OpDate.IsZero() || e.GrossGrosz <= 0 || e.Method == "" {
		return "invalid_request", e
	}
	return "", e
}

var nipDigits = regexp.MustCompile(`^\d{10}$`)

// normNIP — NIP без пробелов и дефисов (чистая, тест): «» — нет NIP, ok=false —
// не 10 цифр. Контрольная сумма — nipValid (З9-и4).
func normNIP(s string) (string, bool) {
	n := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(s))
	if n == "" {
		return "", true
	}
	return n, nipDigits.MatchString(n)
}

// expenseChanges — «было → стало» для журнала правки (чистая, тест): только
// изменённое, имена категорий и контрагентов — из names.
func expenseChanges(a, b models.Expense, names map[string]string) []string {
	out := []string{}
	pln := func(g int64) string { return fmt.Sprintf("%.2f", models.PLNFromGrosz(g)) }
	id := func(u *uuid.UUID) string {
		if u == nil {
			return "—"
		}
		return names[u.String()]
	}
	if !a.OpDate.Equal(b.OpDate) {
		out = append(out, "дата "+a.OpDate.Format("02.01.2006")+" → "+b.OpDate.Format("02.01.2006"))
	}
	if a.CategoryID != b.CategoryID {
		out = append(out, "категория "+names[a.CategoryID.String()]+" → "+names[b.CategoryID.String()])
	}
	if id(a.CounterpartyID) != id(b.CounterpartyID) {
		out = append(out, "контрагент "+id(a.CounterpartyID)+" → "+id(b.CounterpartyID))
	}
	if a.GrossGrosz != b.GrossGrosz {
		out = append(out, "сумма "+pln(a.GrossGrosz)+" → "+pln(b.GrossGrosz)+" zł")
	}
	if a.VatGrosz != b.VatGrosz {
		out = append(out, "VAT "+pln(a.VatGrosz)+" → "+pln(b.VatGrosz))
	}
	if a.Method != b.Method {
		out = append(out, "способ "+expenseMethods[a.Method]+" → "+expenseMethods[b.Method])
	}
	if a.Paid != b.Paid {
		st := map[bool]string{true: "оплачен", false: "не оплачен"}
		out = append(out, st[a.Paid]+" → "+st[b.Paid])
	}
	if a.DocNo != b.DocNo {
		out = append(out, "документ «"+a.DocNo+"» → «"+b.DocNo+"»")
	}
	if a.Note != b.Note {
		out = append(out, "заметка изменена")
	}
	return out
}

// expenseLine — расход одной строкой для журнала (чистая, тест).
func expenseLine(e *models.Expense, category string) string {
	s := fmt.Sprintf("%s · %.2f zł", category, models.PLNFromGrosz(e.GrossGrosz))
	if e.VatGrosz > 0 {
		s += fmt.Sprintf(" (VAT %.2f)", models.PLNFromGrosz(e.VatGrosz))
	}
	s += " · " + expenseMethods[e.Method] + " · " + e.OpDate.Format("02.01.2006")
	if !e.Paid {
		s += " · не оплачен"
	}
	if e.DocNo != "" {
		s += " · " + e.DocNo
	}
	return s
}

// ── справочники ──

func categoryNames() map[string]string {
	var cats []models.ExpenseCategory
	db.Find(&cats)
	out := map[string]string{}
	for _, c := range cats {
		out[c.ID.String()] = c.Name
	}
	return out
}

// GET /admin/expense-categories — все категории, стандартные первыми (owner).
func handleExpenseCategories(c *gin.Context) {
	var cats []models.ExpenseCategory
	db.Order("sort, name").Find(&cats)
	c.JSON(http.StatusOK, gin.H{"categories": cats})
}

// GET /admin/counterparties — контрагенты по имени (owner).
func handleCounterparties(c *gin.Context) {
	var list []models.Counterparty
	db.Order("lower(name)").Find(&list)
	c.JSON(http.StatusOK, gin.H{"counterparties": list})
}

// POST /admin/counterparties — новый контрагент (owner); то же имя — 409 с его id.
func handleCounterpartyCreate(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		NIP     string `json:"nip"`
		Contact string `json:"contact"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name := strings.TrimSpace(req.Name)
	if n := len([]rune(name)); n == 0 || n > maxCounterparty {
		expenseFail(c, http.StatusBadRequest, "bad_name")
		return
	}
	nip, code := checkNIP(req.NIP) // З9-и4: и контрольная сумма
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	contact := strings.TrimSpace(req.Contact)
	if len([]rune(contact)) > maxCPContact {
		expenseFail(c, http.StatusBadRequest, "bad_contact")
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > maxExpenseNote {
		expenseFail(c, http.StatusBadRequest, "bad_note")
		return
	}
	var same models.Counterparty
	if db.Where("lower(name) = lower(?)", name).First(&same).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "counterparty_exists", "message": expenseErrors["counterparty_exists"], "id": same.ID})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	cp := models.Counterparty{Name: name, Contact: contact, Note: note, CreatedBy: &adminID}
	if nip != "" {
		cp.NIP = &nip
	}
	if err := db.Create(&cp).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	what := name
	if nip != "" {
		what += " (NIP " + nip + ")"
	}
	logAdminAction(c, "counterparty_create", nil, what)
	c.JSON(http.StatusCreated, gin.H{"counterparty": cp})
}

// ── расходы ──

// expenseRefs — категория и контрагент существуют (базой).
func expenseRefs(e *models.Expense) string {
	var n int64
	if db.Model(&models.ExpenseCategory{}).Where("id = ?", e.CategoryID).Count(&n); n == 0 {
		return "bad_category"
	}
	if e.CounterpartyID != nil {
		if db.Model(&models.Counterparty{}).Where("id = ?", *e.CounterpartyID).Count(&n); n == 0 {
			return "bad_counterparty"
		}
	}
	return ""
}

// applyExpenseIDs — категория и контрагент из запроса (uuid; контрагент "" — убрать).
func applyExpenseIDs(req expenseRequest, e *models.Expense) string {
	if req.CategoryID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*req.CategoryID))
		if err != nil {
			return "bad_category"
		}
		e.CategoryID = id
	}
	if req.CounterpartyID != nil {
		s := strings.TrimSpace(*req.CounterpartyID)
		if s == "" {
			e.CounterpartyID = nil
		} else {
			id, err := uuid.Parse(s)
			if err != nil {
				return "bad_counterparty"
			}
			e.CounterpartyID = &id
		}
	}
	return ""
}

// ── З9-и3а: расход из кассы в ящике (решение основателя 27.09) ──

// tillPoint — сдача модели ящика для tillPlace: когда и пересчитали ли
// (начальный остаток — тоже пересчёт).
type tillPoint struct {
	At      time.Time
	Counted bool
}

// tillPlace — где расход «из кассы» ложится в ящик (чистая, тест). from/to —
// клубные сутки дня операции; anchor — когда расход внесён (при правке способа
// или дня — момент первой записи, «как если бы так и внесли»; при отметке
// «оплачен» — сейчас); now — сейчас; points — сдачи модели ящика до now, по
// возрастанию. Кто нехватку увидел — тот её и объясняет:
//   - сутки операции на момент записи ещё шли (или впереди) — деньги ушли тогда:
//     с тех пор не пересчитывали — ящик уменьшается сейчас (при записи это и
//     есть «должно быть меньше сразу»); пересчитали — объяснён первый пересчёт
//     после записи;
//   - сутки прошли — задним числом: берём последнюю сдачу тех суток, нет её —
//     первую после них; сдали без пересчёта — первый пересчёт после неё (без
//     пересчёта «должно быть» переносится как было); пересчёта нет — сейчас.
//
// Объяснённый пересчёт — второе значение (номер в points): расход ложится
// в миг перед ним, его расхождение уменьшается, текущий ящик не меняется.
func tillPlace(from, to, anchor, now time.Time, points []tillPoint) (time.Time, int) {
	pick := -1
	for i, p := range points {
		if !p.At.Before(now) {
			break
		}
		if anchor.Before(to) { // сутки ещё шли: первая сдача после записи
			if !p.At.Before(anchor) {
				pick = i
				break
			}
			continue
		}
		if p.At.Before(from) {
			continue
		}
		if p.At.Before(to) {
			pick = i // последняя сдача суток операции
			continue
		}
		if pick < 0 {
			pick = i // первая сдача после них
		}
		break
	}
	for ; pick >= 0 && pick < len(points) && points[pick].At.Before(now); pick++ {
		if points[pick].Counted {
			return points[pick].At.Add(-time.Microsecond), pick
		}
	}
	return now, -1
}

func tillPaid(e *models.Expense) bool { return e.Method == models.ExpenseTill && e.Paid }

// placeTill — поставить расход в ящик (под замком ящика): drawer_at и какую
// сдачу объясняет. Не из кассы или не оплачен — ящик не трогает. anchor — как
// у tillPlace.
func placeTill(tx *gorm.DB, e *models.Expense, anchor time.Time) {
	e.DrawerAt, e.ExplainsCloseout = nil, nil
	if !tillPaid(e) {
		return
	}
	from, to := tillDay(e.OpDate)
	now := time.Now()
	since := from
	if anchor.Before(since) {
		since = anchor
	}
	var cos []models.ShiftCloseout
	tx.Select("id, created_at, counted_grosz").Where("opening_grosz IS NOT NULL AND created_at >= ? AND created_at < ?", since, now).
		Order("created_at").Find(&cos)
	points := make([]tillPoint, len(cos))
	for i := range cos {
		points[i] = tillPoint{At: cos[i].CreatedAt, Counted: cos[i].CountedGrosz != nil}
	}
	at, i := tillPlace(from, to, anchor, now, points)
	e.DrawerAt = &at
	if i >= 0 {
		id := cos[i].ID
		e.ExplainsCloseout = &id
	}
}

// tillDay — клубные сутки дня операции: [report_hour того дня, report_hour следующего).
func tillDay(d time.Time) (from, to time.Time) {
	rh := int(settingInt64("report_hour", 8))
	return time.Date(d.Year(), d.Month(), d.Day(), rh, 0, 0, 0, time.Local),
		time.Date(d.Year(), d.Month(), d.Day()+1, rh, 0, 0, 0, time.Local)
}

var errBadCloseout = errors.New("bad_closeout")

// tillBefore — владелец сказал, что деньги взяли до сдачи id (З9-и3в, решение
// основателя 27.09): расход ложится в миг перед ней и объясняет её. Годится
// сдача с пересчётом (и начальный остаток) в клубных сутках дня операции, уже
// прошедшая; расход — оплаченный «из кассы». Под замком ящика; false — не годится.
func tillBefore(tx *gorm.DB, e *models.Expense, id string) bool {
	cid, err := uuid.Parse(id)
	if err != nil || !tillPaid(e) {
		return false
	}
	from, to := tillDay(e.OpDate)
	var co models.ShiftCloseout
	if tx.Select("id, created_at").Where("id = ? AND opening_grosz IS NOT NULL AND counted_grosz IS NOT NULL "+
		"AND created_at >= ? AND created_at < ? AND created_at < ?", cid, from, to, time.Now()).First(&co).Error != nil {
		return false
	}
	at := co.CreatedAt.Add(-time.Microsecond)
	e.DrawerAt, e.ExplainsCloseout = &at, &co.ID
	return true
}

// tillWhere — место расхода в ящике словами для журнала правки (чистая, тест).
func tillWhere(e *models.Expense) string {
	switch {
	case e.DrawerAt == nil:
		return "ящик не трогает"
	case e.ExplainsCloseout == nil:
		return "из ящика с " + e.DrawerAt.Local().Format("02.01 15:04")
	default:
		return "до сдачи " + e.DrawerAt.Add(time.Microsecond).Local().Format("02.01 15:04")
	}
}

// tillInfo — что расход из кассы сделал с ящиком (для ответа и тоста): nil — не
// из кассы; live — «должно быть» меньше с момента записи; иначе — какую сдачу
// объясняет (когда, кто, её расхождение; baseline — это начальный остаток:
// расход ушёл до того, как ящик завели, и уже в нём учтён).
func tillInfo(e *models.Expense) gin.H {
	if e.DrawerAt == nil {
		return nil
	}
	if e.ExplainsCloseout == nil {
		return gin.H{"live": true}
	}
	var co models.ShiftCloseout
	if db.First(&co, "id = ?", *e.ExplainsCloseout).Error != nil {
		return gin.H{"live": false}
	}
	out := gin.H{"live": false, "closeout_id": co.ID, "closeout_at": co.CreatedAt, "closeout_baseline": co.Baseline,
		"closeout_by": nicknamesByID([]string{co.UserID.String()})[co.UserID.String()]}
	if co.DiffGrosz != nil {
		out["closeout_diff_pln"] = models.PLNFromGrosz(*co.DiffGrosz)
	}
	return out
}

func expenseToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// expenseOut — расход в ответ: суммы в злотых, имена категории и контрагента.
func expenseOut(e *models.Expense, names map[string]string, cps map[string]string, nick map[string]string) gin.H {
	row := gin.H{
		"id": e.ID, "op_date": e.OpDate.Format("2006-01-02"), "paid": e.Paid,
		"category_id": e.CategoryID, "category": names[e.CategoryID.String()],
		"amount_pln": models.PLNFromGrosz(e.GrossGrosz), "vat_pln": models.PLNFromGrosz(e.VatGrosz),
		"net_pln": models.PLNFromGrosz(e.GrossGrosz - e.VatGrosz),
		"method":  e.Method, "doc_no": e.DocNo, "note": e.Note,
		"has_photo": e.PhotoAt != nil, "created_at": e.CreatedAt, "updated_at": e.UpdatedAt,
	}
	if e.PhotoAt != nil {
		row["photo"] = fmt.Sprintf("/api/v1/admin/expenses/%s/photo?v=%d", e.ID, e.PhotoAt.Unix())
	}
	if e.DrawerAt != nil { // З9-и3а: расход из кассы — в ящике
		row["drawer_at"] = e.DrawerAt
		if e.ExplainsCloseout != nil {
			row["explains_closeout"] = *e.ExplainsCloseout
		}
	}
	if e.TemplateID != nil && e.TemplateMonth != nil { // З9-и5: по шаблону за месяц
		row["template_id"], row["template_month"] = *e.TemplateID, e.TemplateMonth.Format("2006-01-02")
	}
	if e.SupplyID != nil { // З10-и6: расход поставки — его завела оценка в «Закупках»
		row["supply_id"] = *e.SupplyID
	}
	if e.CounterpartyID != nil {
		row["counterparty_id"], row["counterparty"] = *e.CounterpartyID, cps[e.CounterpartyID.String()]
	}
	if e.CreatedBy != nil {
		row["created_by"] = nick[e.CreatedBy.String()]
	}
	if e.StornoAt != nil {
		row["storno_at"], row["storno_reason"] = e.StornoAt, e.StornoReason
		if e.StornoBy != nil {
			row["storno_by"] = nick[e.StornoBy.String()]
		}
	}
	return row
}

func counterpartyNames() map[string]string {
	var list []models.Counterparty
	db.Select("id, name").Find(&list)
	out := map[string]string{}
	for _, cp := range list {
		out[cp.ID.String()] = cp.Name
	}
	return out
}

func expenseOutOne(e *models.Expense) gin.H {
	ids := []string{}
	for _, u := range []*uuid.UUID{e.CreatedBy, e.StornoBy} {
		if u != nil {
			ids = append(ids, u.String())
		}
	}
	row := expenseOut(e, categoryNames(), counterpartyNames(), nicknamesByID(ids))
	if t := tillInfo(e); t != nil { // З9-и3а
		row["till"] = t
	}
	return row
}

// expenseQ — расходы без байтов фото: модель их не знает, а SELECT * тянул бы их
// в каждом списке.
func expenseQ() *gorm.DB { return db.Session(&gorm.Session{QueryFields: true}) }

// reloadExpense — расход заново из базы в новую переменную: GORM, читая в уже
// заполненную структуру, не обнуляет указатели, у которых в базе NULL (фото убрали,
// а в ответе оно ещё есть).
func reloadExpense(id uuid.UUID) *models.Expense {
	var e models.Expense
	expenseQ().First(&e, "id = ?", id)
	return &e
}

// expensesOfPeriod — расходы периода по дню операции, со сторнированными: «Расходы» на экране и лист выгрузки
// «Расходы» (З13-и5) — одним запросом.
func expensesOfPeriod(p period) *gorm.DB {
	return expenseQ().Model(&models.Expense{}).Where("op_date >= ? AND op_date <= ?",
		p.FromDay.Format("2006-01-02"), p.ToDay.Format("2006-01-02"))
}

// GET /admin/expenses?preset=…|from=…&to=…[&category_id][&method][&counterparty_id]
// — расходы по дню операции (owner): список (сторнированные — с пометкой) и
// итоги без сторно — всего и по категориям.
func handleExpenseList(c *gin.Context) {
	p, _, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	q := expensesOfPeriod(p)
	for col, code := range map[string]string{"category_id": "bad_category", "counterparty_id": "bad_counterparty"} {
		if v := strings.TrimSpace(c.Query(col)); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				expenseFail(c, http.StatusBadRequest, code)
				return
			}
			q = q.Where(col+" = ?", id)
		}
	}
	if m := c.Query("method"); m != "" {
		if _, ok := expenseMethods[m]; !ok {
			expenseFail(c, http.StatusBadRequest, "bad_method")
			return
		}
		q = q.Where("method = ?", m)
	}
	var list []models.Expense
	q.Order("op_date DESC, created_at DESC").Find(&list)

	names, cps := categoryNames(), counterpartyNames()
	ids := []string{}
	for i := range list {
		for _, u := range []*uuid.UUID{list[i].CreatedBy, list[i].StornoBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(ids)
	out := make([]gin.H, 0, len(list))
	var gross, vat, count int64
	type catSum struct {
		gross, count int64
	}
	byCat, order := map[uuid.UUID]*catSum{}, []uuid.UUID{}
	for i := range list {
		e := &list[i]
		out = append(out, expenseOut(e, names, cps, nick))
		if e.StornoAt != nil {
			continue
		}
		gross, vat, count = gross+e.GrossGrosz, vat+e.VatGrosz, count+1
		if byCat[e.CategoryID] == nil {
			byCat[e.CategoryID] = &catSum{}
			order = append(order, e.CategoryID)
		}
		byCat[e.CategoryID].gross += e.GrossGrosz
		byCat[e.CategoryID].count++
	}
	cats := make([]gin.H, 0, len(order))
	for _, id := range order {
		s := byCat[id]
		cats = append(cats, gin.H{"category_id": id, "category": names[id.String()], "amount_pln": models.PLNFromGrosz(s.gross), "count": s.count})
	}
	c.JSON(http.StatusOK, gin.H{"period": p.out(), "expenses": out,
		"totals": gin.H{"count": count, "amount_pln": models.PLNFromGrosz(gross), "vat_pln": models.PLNFromGrosz(vat),
			"net_pln": models.PLNFromGrosz(gross - vat)},
		"by_category": cats, "owner_debt": ownerDebtOf(db).out(), // З9-и3б: клуб должен владельцу — на все даты
		"due": dueOut(dueList(expenseToday()), expenseToday())}) // З9-и5: ждут внесения
}

// POST /admin/expenses — новый расход (owner).
func handleExpenseCreate(c *gin.Context) {
	var req expenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	cur := models.Expense{Paid: true}
	if req.CategoryID == nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if code := applyExpenseIDs(req, &cur); code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	code, e := checkExpense(req, cur, expenseToday())
	if code == "" {
		code = expenseRefs(&e)
	}
	if code == "" {
		code = expenseAllowed(&e, nil) // З9-и4: способ включён, категория не скрыта
	}
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		expenseFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	e.ClubID, e.CreatedBy = club.ID, &adminID
	tcode, tname := "", "" // З9-и5: по шаблону
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil { // как сдачи (З9-и3а)
			return err
		}
		if b := req.TemplateID; b != nil && *b != "" { // З9-и5: по шаблону за месяц — один на месяц
			month := ""
			if req.TemplateMonth != nil {
				month = *req.TemplateMonth
			}
			code, t, m := expenseTemplateLink(tx, *b, month, expenseToday())
			if code != "" {
				tcode = code
				return errTemplate
			}
			e.TemplateID, e.TemplateMonth, tname = &t.ID, &m, t.Name
		}
		e.CreatedAt = time.Now()
		placeTill(tx, &e, e.CreatedAt) // при записи anchor — сейчас
		if b := req.BeforeCloseout; b != nil && *b != "" && !tillBefore(tx, &e, *b) {
			return errBadCloseout // З9-и3в: «взяли до сдачи» — выбор владельца
		}
		return tx.Create(&e).Error
	})
	if errors.Is(err, errBadCloseout) {
		expenseFail(c, http.StatusBadRequest, "bad_closeout")
		return
	}
	if errors.Is(err, errTemplate) || (err != nil && e.TemplateID != nil && isDuplicate(err)) {
		if tcode == "" || tcode == "template_entered" {
			expenseFail(c, http.StatusConflict, "template_entered") // гонка за тот же месяц — уникальный индекс
			return
		}
		expenseFail(c, http.StatusBadRequest, tcode)
		return
	}
	if err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	fresh := reloadExpense(e.ID)
	line := expenseLine(fresh, categoryNames()[fresh.CategoryID.String()])
	if fresh.TemplateID != nil && fresh.TemplateMonth != nil {
		line += " · по шаблону «" + tname + "» за " + fresh.TemplateMonth.Format("01.2006")
	}
	logAdminAction(c, "expense_create", nil, line)
	c.JSON(http.StatusCreated, gin.H{"expense": expenseOutOne(fresh), "owner_debt": ownerDebtOf(db).out()})
}

func loadExpense(c *gin.Context) (*models.Expense, bool) {
	var e models.Expense
	if err := expenseQ().First(&e, "id = ?", c.Param("id")).Error; err != nil {
		expenseFail(c, http.StatusNotFound, "expense_not_found")
		return nil, false
	}
	return &e, true
}

// PATCH /admin/expenses/:id — поправить (owner): меняется присланное, журнал —
// «было → стало»; сторнированный не правится.
func handleExpenseUpdate(c *gin.Context) {
	cur, ok := loadExpense(c)
	if !ok {
		return
	}
	if cur.StornoAt != nil {
		expenseFail(c, http.StatusConflict, "storno")
		return
	}
	var req expenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	next := *cur
	if code := applyExpenseIDs(req, &next); code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	code, e := checkExpense(req, next, expenseToday())
	if code == "" {
		code = expenseRefs(&e)
	}
	if code == "" {
		code = expenseAllowed(&e, cur) // З9-и4: прежние способ и категория — как есть
	}
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	names := categoryNames()
	for k, v := range counterpartyNames() {
		names[k] = v
	}
	changes := expenseChanges(*cur, e, names)
	if len(changes) == 0 && req.BeforeCloseout == nil {
		c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(cur), "changed": 0})
		return
	}
	var rows int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil {
			return err
		}
		// З9-и3а: место в ящике. Был и остался оплаченным «из кассы» с тем же днём — как было;
		// отметили оплату — от этой минуты; поправили способ или день — заново от момента первой
		// записи, как если бы так и внесли: пересчёт с тех пор увидел нехватку — его и объясняет.
		switch {
		case tillPaid(cur) && tillPaid(&e) && cur.OpDate.Equal(e.OpDate):
		case !cur.Paid && e.Paid:
			placeTill(tx, &e, time.Now())
		default:
			placeTill(tx, &e, cur.CreatedAt)
		}
		// З9-и3в: владелец сам сказал, когда взяли: до сдачи — объясняет её; после сдач — с этой минуты
		if b := req.BeforeCloseout; b != nil {
			if *b != "" {
				if !tillBefore(tx, &e, *b) {
					return errBadCloseout
				}
			} else if tillPaid(&e) {
				now := time.Now()
				e.DrawerAt, e.ExplainsCloseout = &now, nil
			}
			if tillWhere(cur) != tillWhere(&e) {
				changes = append(changes, "ящик: "+tillWhere(cur)+" → "+tillWhere(&e))
			}
		}
		if len(changes) == 0 {
			return nil // ни полей, ни места в ящике — нечего писать
		}
		res := tx.Model(&models.Expense{}).Where("id = ? AND storno_at IS NULL", cur.ID).Updates(map[string]any{
			"op_date": e.OpDate.Format("2006-01-02"), "category_id": e.CategoryID, "counterparty_id": e.CounterpartyID,
			"gross_grosz": e.GrossGrosz, "vat_grosz": e.VatGrosz, "method": e.Method, "paid": e.Paid,
			"doc_no": e.DocNo, "note": e.Note, "updated_at": time.Now(),
			"drawer_at": e.DrawerAt, "explains_closeout": e.ExplainsCloseout})
		rows = res.RowsAffected
		return res.Error
	})
	if errors.Is(err, errBadCloseout) {
		expenseFail(c, http.StatusBadRequest, "bad_closeout")
		return
	}
	if err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if len(changes) == 0 {
		c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(cur), "changed": 0})
		return
	}
	if rows == 0 { // сторнировали между чтением и записью
		expenseFail(c, http.StatusConflict, "storno")
		return
	}
	fresh := reloadExpense(cur.ID)
	logAdminAction(c, "expense_update", nil, names[fresh.CategoryID.String()]+fmt.Sprintf(" · %.2f zł: ", models.PLNFromGrosz(fresh.GrossGrosz))+strings.Join(changes, "; "))
	c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(fresh), "changed": len(changes), "owner_debt": ownerDebtOf(db).out()})
}

// POST /admin/expenses/:id/storno — сторно вместо удаления (owner): причина
// обязательна, строка остаётся, из сумм уходит.
func handleExpenseStorno(c *gin.Context) {
	e, ok := loadExpense(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		expenseFail(c, http.StatusBadRequest, "need_reason")
		return
	}
	if len([]rune(reason)) > maxStornoReason {
		expenseFail(c, http.StatusBadRequest, "bad_reason")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	res := db.Model(&models.Expense{}).Where("id = ? AND storno_at IS NULL", e.ID).
		Updates(map[string]any{"storno_at": time.Now(), "storno_by": adminID, "storno_reason": reason})
	if res.Error != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if res.RowsAffected == 0 {
		expenseFail(c, http.StatusConflict, "already_storno")
		return
	}
	e = reloadExpense(e.ID)
	logAdminAction(c, "expense_storno", nil, expenseLine(e, categoryNames()[e.CategoryID.String()])+" — «"+reason+"»")
	c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(e), "owner_debt": ownerDebtOf(db).out()})
}

// PUT /admin/expenses/:id/photo — фото документа (owner; сырое тело, ≤500 КБ).
func handleExpensePhotoPut(c *gin.Context) {
	e, ok := loadExpense(c)
	if !ok {
		return
	}
	ct := c.ContentType()
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		expenseFail(c, http.StatusBadRequest, "bad_photo")
		return
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPhotoBytes+1))
	if err != nil || len(data) == 0 {
		expenseFail(c, http.StatusBadRequest, "bad_photo")
		return
	}
	if len(data) > maxPhotoBytes {
		expenseFail(c, http.StatusRequestEntityTooLarge, "photo_too_big")
		return
	}
	if err := db.Exec("UPDATE expenses SET photo = ?, photo_type = ?, photo_at = now() WHERE id = ?", data, ct, e.ID).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	e = reloadExpense(e.ID)
	logAdminAction(c, "expense_update", nil, expenseLine(e, categoryNames()[e.CategoryID.String()])+": фото документа")
	c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(e)})
}

// GET /admin/expenses/:id/photo — фото документа, только владельцу.
func handleExpensePhoto(c *gin.Context) {
	var row struct {
		Photo     []byte
		PhotoType *string
	}
	if err := db.Raw("SELECT photo, photo_type FROM expenses WHERE id = ?", c.Param("id")).Scan(&row).Error; err != nil || len(row.Photo) == 0 {
		expenseFail(c, http.StatusNotFound, "photo_not_found")
		return
	}
	ct := "image/jpeg"
	if row.PhotoType != nil && *row.PhotoType != "" {
		ct = *row.PhotoType
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, ct, row.Photo)
}

// DELETE /admin/expenses/:id/photo — убрать фото (owner).
func handleExpensePhotoDelete(c *gin.Context) {
	e, ok := loadExpense(c)
	if !ok {
		return
	}
	if e.PhotoAt == nil {
		expenseFail(c, http.StatusNotFound, "photo_not_found")
		return
	}
	if err := db.Exec("UPDATE expenses SET photo = NULL, photo_type = NULL, photo_at = NULL WHERE id = ?", e.ID).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	e = reloadExpense(e.ID)
	logAdminAction(c, "expense_update", nil, expenseLine(e, categoryNames()[e.CategoryID.String()])+": фото убрано")
	c.JSON(http.StatusOK, gin.H{"expense": expenseOutOne(e)})
}
