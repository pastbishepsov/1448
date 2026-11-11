package main

// З22-и1 (OWNER.md, трек З; №59, З-Р28): движок алертов. Алерт — служебное сообщение канала персонала (З1-и2,
// postSystem) с защитой от повторов: вид шлёт свой ключ — «раз за событие» (id сдачи, отмены, позиции) или «раз за
// день» (клубные сутки и предмет, alertDayKey), — и журнал alerts_sent (миграция 097) пропускает ключ один раз: второй
// прогон и гонка двух запросов упираются в первичный ключ. Видимость — по виду: деньги и конкретные люди — «видит
// только владелец» (уточнение 23.09), остальное — всем. Алерты целиком включает владелец в «Экономике» → «Алерты»
// (alerts_on, по умолчанию — присылать); там же проверочный. Виды — З22-и2 (события), З22-и3 (периодические),
// З22-и4 (смены): каждый — строка в alertKinds, свой выключатель и пороги — в той же группе «Экономики».

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// alertKind — вид алерта: видит только владелец или все; Setting — его собственный выключатель ("" — только общий).
type alertKind struct {
	OwnerOnly bool
	Setting   string
}

// alertKinds — все виды алертов.
var alertKinds = map[string]alertKind{
	"test": {OwnerOnly: true}, // проверочный — кнопкой из «Экономики»
	// З22-и3: периодические — деньги и люди, владельцу; вид включает его порог (0 — выкл, alerts_periodic.go)
	"debt":        {OwnerOnly: true},
	"void_share":  {OwnerOnly: true},
	"revenue_low": {OwnerOnly: true},
	// З22-и4: напоминание о смене — сотруднику по нику, видят все; «завтра никто не назначен» — владельцу
	"shift_soon":   {},
	"shift_nobody": {OwnerOnly: true},
}

// alertsEnabled — алерты вида k присылаются: общий выключатель и, если есть, свой.
func alertsEnabled(k alertKind) bool {
	if settingInt64("alerts_on", 1) == 0 {
		return false
	}
	return k.Setting == "" || settingInt64(k.Setting, 1) != 0
}

// alertSend — прислать алерт вида kind с ключом key, если вид включён, а ключ ещё не уходил; true — ушёл. Журнал не
// записался — не уходит: лучше промолчать, чем повторять одно и то же каждым прогоном.
func alertSend(kind, key, text string) bool {
	k, ok := alertKinds[kind]
	if !ok || !alertsEnabled(k) {
		return false
	}
	res := db.Exec("INSERT INTO alerts_sent (kind, key) VALUES (?, ?) ON CONFLICT DO NOTHING", kind, key)
	if res.Error != nil || res.RowsAffected == 0 {
		return false
	}
	postSystem(text, k.OwnerOnly, map[string]any{"alert": kind})
	return true
}

// alertDayKey — ключ «раз за день»: клубные сутки момента t (ночь до границы — вчерашние) и предмет. Чистая функция
// (тест).
func alertDayKey(t time.Time, reportHour int, subject string) string {
	return clubDayOf(t, reportHour).Format("2006-01-02") + "/" + subject
}

// alertTestText — текст проверочного алерта.
const alertTestText = "Проверка алертов: так алерт выглядит в канале персонала. Этот видит только владелец."

// POST /admin/alerts/test — проверочный алерт (owner, З22-и1): раз за клубные сутки. Не ушёл — sent: false и
// причина: off — алерты выключены, already — сегодня проверочный уже был.
func handleAlertTest(c *gin.Context) {
	if !alertsEnabled(alertKinds["test"]) {
		c.JSON(http.StatusOK, gin.H{"sent": false, "reason": "off"})
		return
	}
	if !alertSend("test", alertDayKey(time.Now(), clubDayHour(), "test"), alertTestText) {
		c.JSON(http.StatusOK, gin.H{"sent": false, "reason": "already"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true})
}
