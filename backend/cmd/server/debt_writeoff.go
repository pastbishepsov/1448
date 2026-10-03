package main

// З11-и4 «Списание долга и дебиторка» (OWNER.md, трек З; №15; З-Р12; решения
// основателя 30.09).
//
// Долг гостя — выданное и не рассчитанное (settle.go): бар «в долг» и выданные
// заказы кухни «при выходе», только гостю за ПК; других долгов нет. Гость ушёл
// и не заплатил — владелец списывает весь его долг одной кнопкой, без причины
// (решение основателя 30.09). Списание — строка debt_write_offs и ссылка на неё
// у каждой продажи и заказа (миграция 080). Склад не трогаем — товар съеден;
// выручки нет; сумма — «Потери → долги» по дню списания. Списанное не висит ни
// в карточке, ни в ленте продаж, ни в «ждут оплаты» у кухни, ни у гостя в
// киоске; рассчитать или отменить его нельзя, пока списание стоит.
//
// Гость потом заплатил — владелец отменяет списание в карточке гостя (решение
// основателя 30.09): ссылки снимаются, долг снова открыт и рассчитывается как
// обычно — выручка в день расчёта; отменённое списание — не потеря.
//
// «Дебиторка сейчас» — открытый долг всех гостей на этот момент («Деньги»,
// владельцу): у кого, сколько, с какого времени и где гость — за ПК
// (рассчитается при выходе) или ушёл, не рассчитавшись.

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

const writtenOffMsg = "Долг списан — сначала владелец возвращает его в карточке гостя («Вернуть долг»)"

// writeOffParts — из чего списание, словами для журнала (чистая, тест):
// «кухня 8.00 zł (1), бар 10.00 zł (2)».
func writeOffParts(w *models.DebtWriteOff) string {
	parts := make([]string, 0, 2)
	if w.KitchenCount > 0 {
		parts = append(parts, fmt.Sprintf("кухня %.2f zł (%d)", models.PLNFromGrosz(w.KitchenGrosz), w.KitchenCount))
	}
	if w.BarCount > 0 {
		parts = append(parts, fmt.Sprintf("бар %.2f zł (%d)", models.PLNFromGrosz(w.BarGrosz), w.BarCount))
	}
	return strings.Join(parts, ", ")
}

// writeOffOut — списание для экрана; nick — ники гостей и сотрудников.
func writeOffOut(w *models.DebtWriteOff, nick map[string]string) gin.H {
	row := gin.H{
		"id": w.ID, "user_id": w.UserID, "nickname": nick[w.UserID.String()],
		"total_pln":     models.PLNFromGrosz(w.TotalGrosz),
		"kitchen_count": w.KitchenCount, "kitchen_pln": models.PLNFromGrosz(w.KitchenGrosz),
		"bar_count": w.BarCount, "bar_pln": models.PLNFromGrosz(w.BarGrosz),
		"created_at": w.CreatedAt, "cancelled_at": w.CancelledAt,
	}
	if w.CreatedBy != nil {
		row["by"] = nick[w.CreatedBy.String()]
	}
	return row
}

// writeOffUsers — гости и сотрудники списаний: для nicknamesByID.
func writeOffUsers(rows []models.DebtWriteOff) []string {
	ids := make([]string, 0, len(rows)*2)
	for i := range rows {
		ids = append(ids, rows[i].UserID.String())
		if rows[i].CreatedBy != nil {
			ids = append(ids, rows[i].CreatedBy.String())
		}
	}
	return ids
}

// activeWriteOffs — списанный и не возвращённый долг гостя: строка «Долг
// списан … · Вернуть долг» в карточке (кнопка — у владельца).
func activeWriteOffs(userID uuid.UUID) []gin.H {
	var rows []models.DebtWriteOff
	db.Where("user_id = ? AND cancelled_at IS NULL", userID).Order("created_at DESC").Limit(5).Find(&rows)
	nick := nicknamesByID(writeOffUsers(rows))
	out := make([]gin.H, 0, len(rows))
	for i := range rows {
		out = append(out, writeOffOut(&rows[i], nick))
	}
	return out
}

// POST /admin/users/:id/debt/write-off — списать весь открытый долг гостя
// (owner; без причины — решение основателя 30.09).
func handleDebtWriteOff(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	wo := models.DebtWriteOff{UserID: user.ID, CreatedBy: &adminID, CreatedAt: time.Now()}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&wo).Error; err != nil {
			return err
		}
		// CAS тем же правилом, что у расчёта: параллельный расчёт или отмена
		// заберут строку раньше — её здесь не будет
		if err := tx.Model(&models.Sale{}).Where("user_id = ? AND "+barDebtWhere, user.ID).
			Update("write_off_id", wo.ID).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.KitchenOrder{}).Where("user_id = ? AND "+kitchenDebtWhere, user.ID).
			Update("write_off_id", wo.ID).Error; err != nil {
			return err
		}
		var bar, kit struct {
			Cnt   int
			Grosz int64
		}
		const sum = "SELECT COUNT(*) AS cnt, COALESCE(SUM(ROUND(total_pln * 100)), 0)::bigint AS grosz FROM "
		if err := tx.Raw(sum+"sales WHERE write_off_id = ?", wo.ID).Scan(&bar).Error; err != nil {
			return err
		}
		if err := tx.Raw(sum+"kitchen_orders WHERE write_off_id = ?", wo.ID).Scan(&kit).Error; err != nil {
			return err
		}
		if bar.Cnt+kit.Cnt == 0 {
			return errNoDebt // откат: пустого списания не остаётся
		}
		wo.BarCount, wo.BarGrosz, wo.KitchenCount, wo.KitchenGrosz = bar.Cnt, bar.Grosz, kit.Cnt, kit.Grosz
		wo.TotalGrosz = bar.Grosz + kit.Grosz
		return tx.Model(&models.DebtWriteOff{}).Where("id = ?", wo.ID).Updates(map[string]any{
			"bar_count": wo.BarCount, "bar_grosz": wo.BarGrosz, "kitchen_count": wo.KitchenCount,
			"kitchen_grosz": wo.KitchenGrosz, "total_grosz": wo.TotalGrosz}).Error
	})
	if errors.Is(err, errNoDebt) {
		c.JSON(http.StatusConflict, gin.H{"code": "no_debt", "message": "Гость ничего не должен — списывать нечего"})
		return
	}
	if err != nil {
		respondDBError(c, err)
		return
	}
	target := user.ID
	logAdminAction(c, "debt_write_off", &target, fmt.Sprintf("%.2f zł · %s", models.PLNFromGrosz(wo.TotalGrosz), writeOffParts(&wo)))
	hub.AdminBroadcast("sale", map[string]any{"kind": "write_off", "nickname": user.Nickname,
		"total_pln": models.PLNFromGrosz(wo.TotalGrosz)})
	nick := nicknamesByID(writeOffUsers([]models.DebtWriteOff{wo}))
	c.JSON(http.StatusOK, gin.H{"write_off": writeOffOut(&wo, nick), "nickname": user.Nickname, "debt": guestDebt(user.ID)})
}

// POST /admin/debt-write-offs/:id/cancel — вернуть списанный долг (owner;
// решение основателя 30.09): долг снова открыт и рассчитывается как обычно.
func handleDebtWriteOffCancel(c *gin.Context) {
	var wo models.DebtWriteOff
	if err := db.First(&wo, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "write_off_not_found", "message": "Списание не найдено"})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.DebtWriteOff{}).Where("id = ? AND cancelled_at IS NULL", wo.ID).
			Updates(map[string]any{"cancelled_at": now, "cancelled_by": adminID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errAlreadyVoid
		}
		if err := tx.Model(&models.Sale{}).Where("write_off_id = ?", wo.ID).Update("write_off_id", nil).Error; err != nil {
			return err
		}
		return tx.Model(&models.KitchenOrder{}).Where("write_off_id = ?", wo.ID).Update("write_off_id", nil).Error
	})
	if errors.Is(err, errAlreadyVoid) {
		c.JSON(http.StatusConflict, gin.H{"code": "already_cancelled", "message": "Списание уже отменено — долг вернули раньше"})
		return
	}
	if err != nil {
		respondDBError(c, err)
		return
	}
	wo.CancelledAt, wo.CancelledBy = &now, &adminID
	nick := nicknamesByID(writeOffUsers([]models.DebtWriteOff{wo}))
	target := wo.UserID
	logAdminAction(c, "debt_restore", &target, fmt.Sprintf("вернули в долг %.2f zł · списан %s",
		models.PLNFromGrosz(wo.TotalGrosz), wo.CreatedAt.Format("02.01 15:04")))
	hub.AdminBroadcast("sale", map[string]any{"kind": "write_off_cancel", "nickname": nick[wo.UserID.String()],
		"total_pln": models.PLNFromGrosz(wo.TotalGrosz)})
	c.JSON(http.StatusOK, gin.H{"restored": writeOffOut(&wo, nick), "nickname": nick[wo.UserID.String()], "debt": guestDebt(wo.UserID)})
}

// receivable — открытый долг одного гостя сейчас.
type receivable struct {
	UserID       uuid.UUID
	KitchenCount int64
	KitchenGrosz int64
	BarCount     int64
	BarGrosz     int64
	Since        time.Time // с какого времени висит самое старое
	Computer     string    // за каким ПК гость сейчас; пусто — ушёл, не рассчитавшись
}

func (r receivable) total() int64 { return r.KitchenGrosz + r.BarGrosz }

// sortReceivables — порядок «Дебиторки» (чистая, тест): сперва ушедшие, не
// рассчитавшись, потом сидящие за ПК; внутри — самый старый долг первым.
func sortReceivables(rows []receivable) {
	sort.SliceStable(rows, func(i, j int) bool {
		gi, gj := rows[i].Computer == "", rows[j].Computer == ""
		if gi != gj {
			return gi
		}
		return rows[i].Since.Before(rows[j].Since)
	})
}

// receivablesSQL — открытый долг по гостям: бар и кухня тем же правилом, что в
// карточке (barDebtWhere, kitchenDebtWhere).
const receivablesSQL = `
SELECT user_id,
       COUNT(*) FILTER (WHERE kind = 'kitchen') AS kitchen_count, COALESCE(SUM(g) FILTER (WHERE kind = 'kitchen'), 0)::bigint AS kitchen_grosz,
       COUNT(*) FILTER (WHERE kind = 'bar') AS bar_count, COALESCE(SUM(g) FILTER (WHERE kind = 'bar'), 0)::bigint AS bar_grosz,
       MIN(created_at) AS since
  FROM (SELECT user_id, 'bar' AS kind, ROUND(total_pln * 100)::bigint AS g, created_at FROM sales
         WHERE user_id IS NOT NULL AND ` + barDebtWhere + `
        UNION ALL
        SELECT user_id, 'kitchen', ROUND(total_pln * 100)::bigint, created_at FROM kitchen_orders WHERE ` + kitchenDebtWhere + `) d
 GROUP BY user_id`

const maxReceivableItems = 100

// receivablesTotal — итог «Дебиторки сейчас» и у скольких гостей, без списка (З12-и1: вычитается из
// «Обязательств клуба»).
func receivablesTotal() (int64, int64) {
	var r struct {
		Grosz  int64
		Guests int64
	}
	db.Raw(`SELECT COALESCE(SUM(kitchen_grosz + bar_grosz), 0)::bigint AS grosz, COUNT(*) AS guests FROM (` + receivablesSQL + `) r`).Scan(&r)
	return r.Grosz, r.Guests
}

// receivablesNow — «Дебиторка сейчас»: итог, сколько из него у ушедших и список.
func receivablesNow() gin.H {
	var rows []receivable
	db.Raw(receivablesSQL).Scan(&rows)
	ids := make([]string, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].UserID.String())
	}
	if len(ids) > 0 { // кто сейчас за ПК — рассчитается при выходе
		var seats []struct {
			UserID uuid.UUID
			Name   string
		}
		db.Raw(`SELECT s.user_id, c.name FROM sessions s JOIN computers c ON c.id = s.computer_id
		         WHERE s.status = ? AND s.user_id IN ?`, models.SessionStatusActive, ids).Scan(&seats)
		at := map[uuid.UUID]string{}
		for _, s := range seats {
			at[s.UserID] = s.Name
		}
		for i := range rows {
			rows[i].Computer = at[rows[i].UserID]
		}
	}
	sortReceivables(rows)
	nick := nicknamesByID(ids)
	var total, gone int64
	goneGuests := 0
	items := make([]gin.H, 0, min(len(rows), maxReceivableItems))
	for i, r := range rows {
		total += r.total()
		if r.Computer == "" {
			gone += r.total()
			goneGuests++
		}
		if i < maxReceivableItems {
			items = append(items, gin.H{"user_id": r.UserID, "nickname": nick[r.UserID.String()],
				"total_pln": models.PLNFromGrosz(r.total()), "kitchen_count": r.KitchenCount, "kitchen_pln": models.PLNFromGrosz(r.KitchenGrosz),
				"bar_count": r.BarCount, "bar_pln": models.PLNFromGrosz(r.BarGrosz), "since": r.Since, "computer": r.Computer})
		}
	}
	return gin.H{"total_pln": models.PLNFromGrosz(total), "guests": len(rows),
		"gone_pln": models.PLNFromGrosz(gone), "gone_guests": goneGuests, "items": items}
}

const maxWriteOffItems = 50

// writtenOffIn — списания периода, не отменённые: «Потери → долги» по дню
// списания (отменённое списание — не потеря).
func writtenOffIn(from, to time.Time) gin.H {
	var rows []models.DebtWriteOff
	db.Where("created_at >= ? AND created_at < ? AND cancelled_at IS NULL", from, to).Order("created_at DESC").Find(&rows)
	var total int64
	guests := map[uuid.UUID]bool{}
	for i := range rows {
		total += rows[i].TotalGrosz
		guests[rows[i].UserID] = true
	}
	shown := rows[:min(len(rows), maxWriteOffItems)]
	nick := nicknamesByID(writeOffUsers(shown))
	items := make([]gin.H, 0, len(shown))
	for i := range shown {
		items = append(items, writeOffOut(&shown[i], nick))
	}
	return gin.H{"pln": models.PLNFromGrosz(total), "count": len(rows), "guests": len(guests), "items": items}
}
