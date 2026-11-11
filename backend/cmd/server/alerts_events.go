package main

// З22-и2 (OWNER.md, трек З; №59, З-Р28): событийные алерты — сразу по событию, раз на событие (журнал alerts_sent,
// З22-и1). «Смена закрылась сама, касса не сдана» — владельцу, когда табель сам закрывает забытую смену админа
// (closeForgotten: человек вошёл снова, а прошлую смену не закрыл), а сдачи кассы по ней нет; уход без пересчёта
// кнопкой и так пишет владельцу строкой сдачи (З1-и1) — его не повторяем (решение основателя 10.11). «Попытка отменить
// чужое или не из своей смены» — владельцу: такую отмену админу не даёт сервер (canVoidSale, 403), алерт — о попытке,
// раз на операцию и админа (решение основателя 10.11). «Товар заканчивается» — всем: движение склада опустило остаток
// позиции до её порога «заканчивается» (low_stock) или ниже, раз на такое движение. Выключатели — «Экономика» → «Алерты».

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ── смена закрылась сама ──

// forgotText — алерт о забытой смене (чистая функция, тест): чья, какая (имя шаблона — снимком, может не быть), когда
// табель закрыл её сам и по чему посчитал часы; end не позже start — смены по графику не было, часов ноль.
func forgotText(nick, shift string, start, end, now time.Time) string {
	what := start.Format("02.01") + " с " + start.Format("15:04")
	if shift != "" {
		what = "«" + shift + "» " + what
	}
	hours := "по графику — до " + end.Format("15:04")
	if !end.After(start) {
		hours = "смены по графику не было — часов ноль"
	}
	return fmt.Sprintf("🚪 Смена %s %s закрылась сама: уход не отмечен, касса не сдана. Табель закрыл её при следующем входе "+
		"(%s), %s. Проверь табель и ящик", nick, what, now.Format("02.01 15:04"), hours)
}

// alertForgot — табель сам закрыл смену w концом end (closeForgotten): админу без сдачи кассы по ней — владельцу, раз
// на запись табеля. Свою смену владелец знает сам.
func alertForgot(w *models.WorkEntry, end time.Time) {
	var u models.User
	if db.Select("id, nickname, role").First(&u, "id = ?", w.UserID).Error != nil || u.Role == models.UserRoleOwner {
		return
	}
	var n int64
	db.Model(&models.ShiftCloseout{}).Where("work_entry_id = ?", w.ID).Count(&n)
	if n > 0 {
		return
	}
	alertSend("shift_forgot", "w/"+w.ID.String(),
		forgotText(u.Nickname, w.ShiftName, w.StartedAt.In(time.Local), end.In(time.Local), time.Now()))
}

// ── попытка отменить чужое или не из своей смены ──

// voidDeniedText — алерт о попытке (чистая функция, тест): кто пытался, что (с суммой), от кого и когда операция; code —
// отказ canVoidSale: not_yours — чужое, иначе — не из его смены.
func voidDeniedText(actor, what, author string, at time.Time, code string) string {
	kind := "операцию не из своей смены"
	if code == "not_yours" {
		kind = "чужую операцию"
	}
	from := at.Format("02.01 15:04")
	if author != "" {
		from = "от " + author + ", " + from
	}
	return fmt.Sprintf("⛔ %s: попытка отменить %s — %s (%s); сервер не дал — такое отменяет владелец", actor, kind, what, from)
}

// alertVoidDenied — сервер не дал админу отменить операцию opID (canVoidSale → code): владельцу, раз на операцию и
// админа. what — что именно («продажу «Cola» 6,00 zł»), byID и at — кто и когда её провёл.
func alertVoidDenied(c *gin.Context, what, opID, byID string, at time.Time, code string) {
	actor := c.GetString("user_id")
	ids := []string{actor}
	if byID != "" {
		ids = append(ids, byID)
	}
	nick := nicknamesByID(ids)
	author := ""
	if byID != "" {
		author = nick[byID]
	}
	alertSend("void_denied", opID+"/"+actor, voidDeniedText(nick[actor], what, author, at.In(time.Local), code))
}

// saleWhat — продажа словами для алерта: «продажу «Cola» 6,00 zł».
func saleWhat(s *models.Sale) string {
	return fmt.Sprintf("продажу «%s» %s zł", s.Name, plnRU(s.TotalPLN))
}

// guestNick — ник гостя для текста алерта ("" — не нашли).
func guestNick(id uuid.UUID) string {
	return nicknamesByID([]string{id.String()})[id.String()]
}

// depositWhat — пополнение словами: «пополнение Guest31 на 50,00 zł».
func depositWhat(d *models.Deposit) string {
	return fmt.Sprintf("пополнение %s на %s zł", guestNick(d.UserID), plnRU(d.AmountPLN))
}

// packageWhat — выдача пакета словами: «пакет «3 часа» Guest31 за 30,00 zł».
func packageWhat(p *models.UserPackage) string {
	return fmt.Sprintf("пакет «%s» %s за %s zł", p.Name, guestNick(p.UserID), plnRU(p.PricePLN))
}

// redeemWhat — обмен монет словами: «обмен 500 монет на 60 мин у Guest31».
func redeemWhat(r *models.CoinRedemption) string {
	return fmt.Sprintf("обмен %d монет на %d мин у %s", r.Coins, r.Minutes, guestNick(r.UserID))
}

// ── товар заканчивается ──

// lowStockCrossed — движение на delta опустило остаток до порога low или ниже, а до движения он был выше (чистая
// функция, тест): порог 0 — не следим; приход и остаток, уже лежавший на пороге или ниже, — не событие.
func lowStockCrossed(after, delta, low int) bool {
	return low > 0 && delta < 0 && after <= low && after-delta > low
}

// lowStockText — алерт о позиции (чистая функция, тест).
func lowStockText(name string, stock, low int) string {
	if stock <= 0 {
		return fmt.Sprintf("📦 Закончилось «%s»: на складе 0, минимум — %d", name, low)
	}
	return fmt.Sprintf("📦 Заканчивается «%s»: на складе %d, минимум — %d", name, stock, low)
}

// lowStockWatch — после движения moveID позиции goodID: дождаться, что транзакция движения записалась (строка
// stock_moves видна; нет и через 5 с — откатилась, алерта нет), и, если остаток всё ещё не выше порога, — всем, раз на
// движение. Транзакция склада алерт не ждёт и в нём не участвует.
func lowStockWatch(goodID, moveID uuid.UUID) {
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)
		var n int64
		db.Model(&models.StockMove{}).Where("id = ?", moveID).Count(&n)
		if n == 0 {
			continue
		}
		var g models.Good
		if db.First(&g, "id = ?", goodID).Error != nil || !g.Active || g.LowStock <= 0 || g.Stock > g.LowStock {
			return
		}
		alertSend("low_stock", "m/"+moveID.String(), lowStockText(g.Name, g.Stock, g.LowStock))
		return
	}
}
