package main

// З9-и3б «Долг клуба владельцу» (OWNER.md, трек З; З-Р30, ревью 23.09 №62,
// миграция 073).
//
// Расход способом «личные деньги владельца» (expenses.method = owner) — клуб
// потратил деньги владельца и должен их вернуть:
//
//	клуб должен владельцу = оплаченные расходы личными (без сторно)
//	                        − возвраты владельцу (без отмены)
//
// Возврат — событие минуты записи, как инкассация: из кассы (till) — деньги
// ушли из ящика, «должно быть в ящике» меньше с этой минуты (drawer.go); с
// банка (bank) — ящик не трогает. Больше долга вернуть нельзя (409 over_debt);
// проверка и запись — под замком ящика, как сдачи: два одновременных возврата
// не проскочат мимо остатка долга. Ошибочный возврат отменяет владелец: строка
// остаётся с пометкой, долг и ящик возвращаются. Всё это — бухгалтерия, только
// владельцу (З-Р29): ручки и строки журнала; админ видит одну сумму «минус
// возвраты владельцу из кассы» в «Моей смене» — деньги ушли из его ящика.

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

// ownerDebtSums — долг клуба владельцу, гроши: потрачено личными и возвращено.
type ownerDebtSums struct {
	Spent, Repaid int64
}

// Debt — сколько клуб должен владельцу; меньше нуля — вернули больше, чем он
// потратил (например, расход личными потом сторнировали).
func (d ownerDebtSums) Debt() int64 { return d.Spent - d.Repaid }

func (d ownerDebtSums) out() gin.H {
	pln := models.PLNFromGrosz
	return gin.H{"debt_pln": pln(d.Debt()), "spent_pln": pln(d.Spent), "repaid_pln": pln(d.Repaid)}
}

func ownerDebtOf(q *gorm.DB) ownerDebtSums {
	var d ownerDebtSums
	q.Raw("SELECT COALESCE(SUM(gross_grosz), 0)::bigint FROM expenses WHERE method = ? AND paid AND storno_at IS NULL",
		models.ExpenseOwner).Scan(&d.Spent)
	q.Raw("SELECT COALESCE(SUM(amount_grosz), 0)::bigint FROM owner_repayments WHERE voided_at IS NULL").Scan(&d.Repaid)
	return d
}

var repayErrors = map[string]string{
	"invalid_request":     "Нужны сумма и откуда вернули",
	"bad_amount":          "Сумма — от 0,01 до 1 000 000 zł, не больше двух знаков после запятой",
	"bad_method":          "Откуда: till — из кассы, bank — с банка",
	"bad_note":            "Заметка — до 200 символов",
	"over_debt":           "Больше долга вернуть нельзя",
	"no_club":             "Клуб не настроен",
	"repayment_not_found": "Возврат не найден",
	"already_void":        "Возврат уже отменён",
	"db_error":            "Не удалось записать — попробуй ещё раз",
}

func repayFail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"code": code, "message": repayErrors[code]})
}

type repaymentRequest struct {
	AmountPLN *float64 `json:"amount_pln"`
	Method    string   `json:"method"` // till | bank
	Note      string   `json:"note"`
}

// checkRepayment — что не так с возвратом (чистая, тест); "" — можно писать.
// Остаток долга проверяется отдельно, под замком.
func checkRepayment(req repaymentRequest) string {
	if req.AmountPLN == nil || !validatePLN(*req.AmountPLN, maxCashPLN) {
		return "bad_amount"
	}
	if req.Method != models.RepayTill && req.Method != models.RepayBank {
		return "bad_method"
	}
	if len([]rune(strings.TrimSpace(req.Note))) > maxCashNote {
		return "bad_note"
	}
	return ""
}

var repayMethodRU = map[string]string{models.RepayTill: "из кассы", models.RepayBank: "с банка"}

// repaymentWhat — возврат словами для журнала (чистая, тест): «200.00 zł из кассы · заметка».
func repaymentWhat(r *models.OwnerRepayment) string {
	return fmt.Sprintf("%.2f zł %s", models.PLNFromGrosz(r.AmountGrosz), repayMethodRU[r.Method]) + noteSuffix(r.Note)
}

// repaymentWhen — когда внесён отменяемый возврат, для строки журнала (чистая, тест): « (внесён 27.09 21:40)».
func repaymentWhen(r *models.OwnerRepayment) string {
	return " (внесён " + r.CreatedAt.In(clubLocation).Format("02.01 15:04") + ")"
}

// overDebtMessage — отказ «больше долга» с суммой (чистая, тест).
func overDebtMessage(debt int64) string {
	if debt <= 0 {
		return "Клуб ничего не должен владельцу — возвращать нечего"
	}
	return fmt.Sprintf("Клуб должен владельцу %.2f zł — больше вернуть нельзя", models.PLNFromGrosz(debt))
}

func repaymentOut(r *models.OwnerRepayment, nick map[string]string) gin.H {
	row := gin.H{"id": r.ID, "amount_pln": models.PLNFromGrosz(r.AmountGrosz), "method": r.Method, "note": r.Note,
		"created_at": r.CreatedAt}
	if r.CreatedBy != nil {
		row["nickname"] = nick[r.CreatedBy.String()]
	}
	if r.VoidedAt != nil {
		row["voided_at"] = r.VoidedAt
		if r.VoidedBy != nil {
			row["voided_by_nickname"] = nick[r.VoidedBy.String()]
		}
	}
	return row
}

// repayNicks — ники тех, кто вносил и отменял возвраты.
func repayNicks(list []models.OwnerRepayment) map[string]string {
	ids := []string{}
	for i := range list {
		for _, u := range []*uuid.UUID{list[i].CreatedBy, list[i].VoidedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	return nicknamesByID(ids)
}

// ownerDebtNowOut — долг и «должно быть в ящике» сейчас: ответ ручек возврата,
// чтобы экран не переспрашивал.
func ownerDebtNowOut(h gin.H) gin.H {
	h["owner_debt"] = ownerDebtOf(db).out()
	return drawerNowOut(h)
}

// GET /admin/owner-debt — клуб должен владельцу: итог и возвраты, свежие
// сверху (последние 100), отменённые — с пометкой (owner).
func handleOwnerDebt(c *gin.Context) {
	var list []models.OwnerRepayment
	db.Order("created_at DESC").Limit(100).Find(&list)
	nick := repayNicks(list)
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, repaymentOut(&list[i], nick))
	}
	c.JSON(http.StatusOK, drawerNowOut(gin.H{"owner_debt": ownerDebtOf(db).out(), "repayments": out}))
}

// POST /admin/owner-repayments — вернул владельцу: из кассы или с банка (owner).
func handleOwnerRepay(c *gin.Context) {
	var req repaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		repayFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if code := checkRepayment(req); code != "" {
		repayFail(c, http.StatusBadRequest, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		repayFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	r := models.OwnerRepayment{ClubID: club.ID, AmountGrosz: models.GroszFromPLN(*req.AmountPLN), Method: req.Method,
		Note: strings.TrimSpace(req.Note), CreatedBy: &adminID}
	var debt int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil { // как сдачи и расходы
			return err
		}
		if debt = ownerDebtOf(tx).Debt(); r.AmountGrosz > debt {
			return errOverDebt
		}
		r.CreatedAt = time.Now()
		return tx.Create(&r).Error
	})
	if errors.Is(err, errOverDebt) {
		c.JSON(http.StatusConflict, gin.H{"code": "over_debt", "message": overDebtMessage(debt),
			"owner_debt": ownerDebtOf(db).out()})
		return
	}
	if err != nil {
		repayFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "owner_repay", nil, repaymentWhat(&r)) // вид строки — «долг владельцу: возврат»
	c.JSON(http.StatusCreated, ownerDebtNowOut(gin.H{"repayment": repaymentOut(&r, repayNicks([]models.OwnerRepayment{r}))}))
}

var errOverDebt = errors.New("over_debt")

// POST /admin/owner-repayments/:id/void — отменить ошибочный возврат (owner):
// строка остаётся с пометкой, долг и ящик возвращаются.
func handleOwnerRepayVoid(c *gin.Context) {
	var r models.OwnerRepayment
	if err := db.First(&r, "id = ?", c.Param("id")).Error; err != nil {
		repayFail(c, http.StatusNotFound, "repayment_not_found")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	res := db.Model(&models.OwnerRepayment{}).Where("id = ? AND voided_at IS NULL", r.ID).
		Updates(map[string]any{"voided_at": time.Now(), "voided_by": adminID})
	if res.Error != nil {
		repayFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if res.RowsAffected == 0 {
		repayFail(c, http.StatusConflict, "already_void")
		return
	}
	logAdminAction(c, "owner_repay_void", nil, repaymentWhat(&r)+repaymentWhen(&r)) // «долг владельцу: отмена возврата»
	c.JSON(http.StatusOK, ownerDebtNowOut(gin.H{"voided": r.ID}))
}
