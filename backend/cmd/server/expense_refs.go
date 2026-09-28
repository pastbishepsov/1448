package main

// З9-и4 (OWNER.md, трек З; ревью 23.09 №61, №62, №66): справочники расходов —
// владельцу. Категории: свои (закупка или нет), переименовать, скрыть; у
// стандартных закупка не меняется — по ней «Прибыль» отделит себестоимость
// (Д9). Контрагенты: правка, скрыть, NIP — 10 цифр с контрольной суммой.
// Способы оплаты: лишние выключаются настройкой (№62), хотя бы один остаётся.
// Скрытая категория и выключенный способ новому расходу не даются (400); у
// старых расходов остаются как были — правка без их смены проходит.

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxCategoryName = 64

// expenseMethodOrder — способы по порядку, как в форме расхода.
var expenseMethodOrder = []string{models.ExpenseTill, models.ExpenseBank, models.ExpenseCard, models.ExpenseOwner}

// nipValid — контрольная сумма NIP (чистая, тест): веса 6 5 7 2 3 4 5 6 7 к
// первым девяти цифрам, сумма по модулю 11 — десятая цифра (остаток 10 — такого
// NIP не бывает). n — уже 10 цифр (normNIP).
func nipValid(n string) bool {
	if len(n) != 10 {
		return false
	}
	w := [9]int{6, 5, 7, 2, 3, 4, 5, 6, 7}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(n[i]-'0') * w[i]
	}
	return sum%11 != 10 && sum%11 == int(n[9]-'0')
}

// checkNIP — NIP из формы: "" — без NIP, иначе 10 цифр со сходящейся суммой.
func checkNIP(s string) (string, string) {
	n, ok := normNIP(s)
	if !ok {
		return "", "bad_nip"
	}
	if n != "" && !nipValid(n) {
		return "", "bad_nip_sum"
	}
	return n, ""
}

// parseMethods — включённые способы из настройки (чистая, тест): "" — все;
// неизвестное пропускается; пустой итог — все (выключить всё нельзя).
func parseMethods(v string) map[string]bool {
	on := map[string]bool{}
	for _, m := range strings.Split(v, ",") {
		if _, ok := expenseMethods[strings.TrimSpace(m)]; ok {
			on[strings.TrimSpace(m)] = true
		}
	}
	if len(on) == 0 {
		for _, m := range expenseMethodOrder {
			on[m] = true
		}
	}
	return on
}

// checkMethods — что включить (чистая, тест): известные коды, хотя бы один.
// Возвращает строку настройки в порядке формы или код ошибки.
func checkMethods(list []string) (string, string) {
	want := map[string]bool{}
	for _, m := range list {
		if _, ok := expenseMethods[m]; !ok {
			return "", "bad_method"
		}
		want[m] = true
	}
	out := []string{}
	for _, m := range expenseMethodOrder {
		if want[m] {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return "", "need_method"
	}
	return strings.Join(out, ","), ""
}

func expenseMethodsOn() map[string]bool { return parseMethods(settingString("expense_methods", "")) }

// expenseAllowed — способ включён и категория не скрыта (для записи и правки);
// у правки прежние способ и категория годятся как есть — их выключили или
// скрыли уже потом.
func expenseAllowed(e, cur *models.Expense) string {
	if (cur == nil || cur.Method != e.Method) && !expenseMethodsOn()[e.Method] {
		return "method_off"
	}
	if cur == nil || cur.CategoryID != e.CategoryID {
		var cat models.ExpenseCategory
		if db.Select("id, hidden").First(&cat, "id = ?", e.CategoryID).Error == nil && cat.Hidden {
			return "category_hidden"
		}
	}
	return ""
}

func methodsOut() []gin.H {
	on := expenseMethodsOn()
	out := make([]gin.H, 0, len(expenseMethodOrder))
	for _, m := range expenseMethodOrder {
		out = append(out, gin.H{"code": m, "name": expenseMethods[m], "on": on[m]})
	}
	return out
}

// GET /admin/expense-methods — способы оплаты расходов, какие включены (owner).
func handleExpenseMethods(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"methods": methodsOut()})
}

// PUT /admin/expense-methods {on: [...]} — какие способы включены (owner).
func handleExpenseMethodsPut(c *gin.Context) {
	var req struct {
		On []string `json:"on"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.On == nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	v, code := checkMethods(req.On)
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	was := expenseMethodsOn()
	if err := settingSetString("expense_methods", v); err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	on, off := []string{}, []string{}
	for _, m := range expenseMethodOrder {
		if strings.Contains(","+v+",", ","+m+",") {
			on = append(on, expenseMethods[m])
		} else {
			off = append(off, expenseMethods[m])
		}
	}
	changed := false
	for _, m := range expenseMethodOrder {
		if was[m] != strings.Contains(","+v+",", ","+m+",") {
			changed = true
		}
	}
	if changed {
		line := "способы оплаты: " + strings.Join(on, ", ")
		if len(off) > 0 {
			line += " (выключены: " + strings.Join(off, ", ") + ")"
		}
		logAdminAction(c, "expense_methods", nil, line)
	}
	c.JSON(http.StatusOK, gin.H{"methods": methodsOut(), "changed": changed})
}

// POST /admin/expense-categories — своя категория (owner): имя без повторов
// (регистр не важен, то же имя — 409 с id), purchase — закупка. Встаёт после
// стандартных.
func handleExpenseCategoryCreate(c *gin.Context) {
	var req struct {
		Name     string `json:"name"`
		Purchase bool   `json:"purchase"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name := strings.TrimSpace(req.Name)
	if n := len([]rune(name)); n == 0 || n > maxCategoryName {
		expenseFail(c, http.StatusBadRequest, "bad_category_name")
		return
	}
	var same models.ExpenseCategory
	if db.Where("lower(name) = lower(?)", name).First(&same).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "category_exists", "message": expenseErrors["category_exists"], "id": same.ID})
		return
	}
	var top int
	db.Model(&models.ExpenseCategory{}).Select("COALESCE(MAX(sort), 0)").Scan(&top)
	cat := models.ExpenseCategory{ID: uuid.New(), Name: name, Purchase: req.Purchase, Sort: top + 10}
	if err := db.Create(&cat).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	what := name
	if cat.Purchase {
		what += " (закупка)"
	}
	logAdminAction(c, "expense_category_create", nil, what)
	c.JSON(http.StatusCreated, gin.H{"category": cat})
}

// PATCH /admin/expense-categories/:id {name?, hidden?, purchase?} —
// переименовать, скрыть или вернуть, закупка — только у своих (owner).
// Последнюю видимую категорию не скрыть — расход без категории не внести.
func handleExpenseCategoryUpdate(c *gin.Context) {
	var cat models.ExpenseCategory
	if err := db.First(&cat, "id = ?", c.Param("id")).Error; err != nil {
		expenseFail(c, http.StatusNotFound, "category_not_found")
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Hidden   *bool   `json:"hidden"`
		Purchase *bool   `json:"purchase"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	upd, changes := map[string]any{}, []string{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if n := len([]rune(name)); n == 0 || n > maxCategoryName {
			expenseFail(c, http.StatusBadRequest, "bad_category_name")
			return
		}
		if name != cat.Name {
			var same models.ExpenseCategory
			if db.Where("lower(name) = lower(?) AND id <> ?", name, cat.ID).First(&same).Error == nil {
				c.JSON(http.StatusConflict, gin.H{"code": "category_exists", "message": expenseErrors["category_exists"], "id": same.ID})
				return
			}
			upd["name"] = name
			changes = append(changes, "«"+cat.Name+"» → «"+name+"»")
		}
	}
	if req.Purchase != nil && *req.Purchase != cat.Purchase {
		if cat.Code != nil {
			expenseFail(c, http.StatusBadRequest, "purchase_fixed")
			return
		}
		upd["purchase"] = *req.Purchase
		changes = append(changes, map[bool]string{true: "закупка", false: "не закупка"}[*req.Purchase])
	}
	if req.Hidden != nil && *req.Hidden != cat.Hidden {
		if *req.Hidden {
			var visible int64
			db.Model(&models.ExpenseCategory{}).Where("NOT hidden AND id <> ?", cat.ID).Count(&visible)
			if visible == 0 {
				expenseFail(c, http.StatusConflict, "last_category")
				return
			}
		}
		upd["hidden"] = *req.Hidden
		changes = append(changes, map[bool]string{true: "скрыта", false: "снова в списке"}[*req.Hidden])
	}
	if len(changes) == 0 {
		c.JSON(http.StatusOK, gin.H{"category": cat, "changed": 0})
		return
	}
	if err := db.Model(&models.ExpenseCategory{}).Where("id = ?", cat.ID).Updates(upd).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.ExpenseCategory
	db.First(&fresh, "id = ?", cat.ID)
	logAdminAction(c, "expense_category_update", nil, fresh.Name+": "+strings.Join(changes, "; "))
	c.JSON(http.StatusOK, gin.H{"category": fresh, "changed": len(changes)})
}

// PATCH /admin/counterparties/:id {name?, nip?, contact?, note?, hidden?} —
// поправить контрагента (owner): имя без повторов, NIP с контрольной суммой
// ("" — убрать), скрыть или вернуть в подсказки.
func handleCounterpartyUpdate(c *gin.Context) {
	var cp models.Counterparty
	if err := db.First(&cp, "id = ?", c.Param("id")).Error; err != nil {
		expenseFail(c, http.StatusNotFound, "counterparty_not_found")
		return
	}
	var req struct {
		Name    *string `json:"name"`
		NIP     *string `json:"nip"`
		Contact *string `json:"contact"`
		Note    *string `json:"note"`
		Hidden  *bool   `json:"hidden"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	upd, changes := map[string]any{}, []string{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if n := len([]rune(name)); n == 0 || n > maxCounterparty {
			expenseFail(c, http.StatusBadRequest, "bad_name")
			return
		}
		if name != cp.Name {
			var same models.Counterparty
			if db.Where("lower(name) = lower(?) AND id <> ?", name, cp.ID).First(&same).Error == nil {
				c.JSON(http.StatusConflict, gin.H{"code": "counterparty_exists", "message": expenseErrors["counterparty_exists"], "id": same.ID})
				return
			}
			upd["name"] = name
			changes = append(changes, "«"+cp.Name+"» → «"+name+"»")
		}
	}
	if req.NIP != nil {
		nip, code := checkNIP(*req.NIP)
		if code != "" {
			expenseFail(c, http.StatusBadRequest, code)
			return
		}
		old := ""
		if cp.NIP != nil {
			old = *cp.NIP
		}
		if nip != old {
			if nip == "" {
				upd["nip"] = nil
			} else {
				upd["nip"] = nip
			}
			changes = append(changes, "NIP "+dash(old)+" → "+dash(nip))
		}
	}
	if req.Contact != nil {
		contact := strings.TrimSpace(*req.Contact)
		if len([]rune(contact)) > maxCPContact {
			expenseFail(c, http.StatusBadRequest, "bad_contact")
			return
		}
		if contact != cp.Contact {
			upd["contact"] = contact
			changes = append(changes, "контакты изменены")
		}
	}
	if req.Note != nil {
		note := strings.TrimSpace(*req.Note)
		if len([]rune(note)) > maxExpenseNote {
			expenseFail(c, http.StatusBadRequest, "bad_note")
			return
		}
		if note != cp.Note {
			upd["note"] = note
			changes = append(changes, "заметка изменена")
		}
	}
	if req.Hidden != nil && *req.Hidden != cp.Hidden {
		upd["hidden"] = *req.Hidden
		changes = append(changes, map[bool]string{true: "скрыт", false: "снова в подсказках"}[*req.Hidden])
	}
	if len(changes) == 0 {
		c.JSON(http.StatusOK, gin.H{"counterparty": cp, "changed": 0})
		return
	}
	if err := db.Model(&models.Counterparty{}).Where("id = ?", cp.ID).Updates(upd).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.Counterparty
	db.First(&fresh, "id = ?", cp.ID)
	logAdminAction(c, "counterparty_update", nil, fresh.Name+": "+strings.Join(changes, "; "))
	c.JSON(http.StatusOK, gin.H{"counterparty": fresh, "changed": len(changes)})
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
