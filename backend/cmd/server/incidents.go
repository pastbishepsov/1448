package main

// Журнал проблем для стойки (KIOSK.md, К8; решение основателя Р4).
//
// Потерю агента, зависший ПК, свет автоматикой НЕ компенсируем: факт
// записывается (руками админа или хабом при обрыве агента во время сессии —
// и2), а компенсацию — минутами в запас гостя (coin_minutes) или деньгами на
// кошелёк (wallet adjust, ref=incident) — назначает админ. Компенсация —
// рост обязательства клуба, не выручка и не выручка со знаком минус (Р7
// трека Г): в отчёте «Деньги» она стоит отдельной строкой.
//
// Открытая запись = resolved_at IS NULL: админ ещё не решил. Закрыть можно
// компенсацией (compensate) или без неё (resolve). Автозапись обрыва агента
// (auto) получает ended_at/minutes при реконнекте агента или конце сессии;
// обрыв короче incidentBlipSec записью не считается — сетевой чих.

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const incidentBlipSec = 20 // обрыв агента короче — не проблема, а сетевой чих

var incidentKindNames = map[string]string{
	models.IncidentPCHang:    "ПК завис",
	models.IncidentAgentLost: "агент пропал",
	models.IncidentPower:     "свет/питание",
	models.IncidentOther:     "другое",
}

func incidentKindName(k string) string {
	if n, ok := incidentKindNames[k]; ok {
		return n
	}
	return k
}

// incidentDurationMin — минуты длительности, вверх (6 мин 10 с → 7).
func incidentDurationMin(from, to time.Time) int {
	sec := to.Sub(from).Seconds()
	if sec <= 0 {
		return 0
	}
	return int(math.Ceil(sec / 60))
}

// ── К8-и2: автозапись факта — хаб дёргает при обрыве и возврате агента ──

// agentLostOpen — агент ПК отвалился: если идёт оплачиваемая сессия — открытая
// автозапись (без компенсации, решает админ). Дубли на одну сессию не пишем.
func agentLostOpen(computerID string) {
	pcID, err := uuid.Parse(computerID)
	if err != nil || db == nil {
		return
	}
	var rows []models.Session
	db.Where("computer_id = ? AND status = ?", pcID, models.SessionStatusActive).Limit(1).Find(&rows)
	if len(rows) == 0 || sessionWaitingReady(&rows[0]) {
		return
	}
	s := rows[0]
	var n int64
	db.Model(&models.Incident{}).Where("session_id = ? AND auto AND ended_at IS NULL", s.ID).Count(&n)
	if n > 0 {
		return
	}
	now := time.Now()
	inc := models.Incident{ClubID: s.ClubID, ComputerID: &pcID, UserID: &s.UserID, SessionID: &s.ID,
		Kind: models.IncidentAgentLost, Auto: true, StartedAt: now}
	if err := db.Create(&inc).Error; err != nil {
		log.Printf("инциденты: автозапись обрыва агента не записалась: %v", err)
		return
	}
	log.Printf("инциденты: агент ПК %s пропал во время сессии %s — автозапись %s", computerID, s.ID, inc.ID)
	hub.AdminBroadcast("incident", map[string]any{"id": inc.ID, "kind": inc.Kind, "auto": true, "computer_id": computerID, "open": true})
}

// agentLostClose — агент вернулся: открытые автозаписи этого ПК получают
// длительность; короче incidentBlipSec — удаляются (это не проблема).
func agentLostClose(computerID string) {
	pcID, err := uuid.Parse(computerID)
	if err != nil || db == nil {
		return
	}
	closeAutoIncidents(db.Where("computer_id = ? AND auto AND ended_at IS NULL", pcID), time.Now())
}

// closeSessionIncidents — конец сессии закрывает её автозаписи длительностью
// (finishSession): агент мог так и не вернуться.
func closeSessionIncidents(sessionID uuid.UUID, now time.Time) {
	closeAutoIncidents(db.Where("session_id = ? AND auto AND ended_at IS NULL", sessionID), now)
}

func closeAutoIncidents(q *gorm.DB, now time.Time) {
	var open []models.Incident
	q.Find(&open)
	for i := range open {
		inc := &open[i]
		if now.Sub(inc.StartedAt).Seconds() < incidentBlipSec {
			db.Delete(inc) // сетевой чих — записью не считается
			continue
		}
		mins := incidentDurationMin(inc.StartedAt, now)
		db.Model(inc).Updates(map[string]any{"ended_at": now, "minutes": mins})
		log.Printf("инциденты: агент ПК %v вернулся — обрыв %d мин (%s)", inc.ComputerID, mins, inc.ID)
		hub.AdminBroadcast("incident", map[string]any{"id": inc.ID, "kind": inc.Kind, "auto": true, "minutes": mins, "open": true})
	}
}

// ── компенсация ──

type compensation struct {
	Minutes int
	Grosz   int64
}

func (cp compensation) empty() bool { return cp.Minutes <= 0 && cp.Grosz <= 0 }

func (cp compensation) text() string {
	parts := []string{}
	if cp.Minutes > 0 {
		parts = append(parts, fmt.Sprintf("+%d мин", cp.Minutes))
	}
	if cp.Grosz > 0 {
		parts = append(parts, fmt.Sprintf("+%.2f zł", models.PLNFromGrosz(cp.Grosz)))
	}
	return strings.Join(parts, " и ")
}

// applyCompensation — минуты в запас гостя и/или деньги на кошелёк, одной
// транзакцией с закрытием записи. Требует гостя у записи.
func applyCompensation(inc *models.Incident, cp compensation, adminID uuid.UUID, now time.Time) error {
	if inc.UserID == nil {
		return errIncidentNoGuest
	}
	uid := *inc.UserID
	// Ж2 (Ж-Р3): минуты компенсации — к зоне ПК из записи, годны только там
	// (проблема была на этой машине — и время возвращаем на ней же); записи
	// без ПК или ПК без зоны — в старый пул без зоны, как раньше.
	var compZone *models.Zone
	if inc.ComputerID != nil {
		compZone = zoneByID(zoneOfComputer(*inc.ComputerID))
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if cp.Minutes > 0 {
			if compZone != nil {
				ref := inc.ID
				grant := grantFor(uid, compZone, cp.Minutes, models.MinuteGrantCompensation, &ref, &adminID)
				if err := tx.Create(&grant).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&models.User{}).Where("id = ?", uid).
				UpdateColumn("coin_minutes", gorm.Expr("coin_minutes + ?", cp.Minutes)).Error; err != nil {
				return err
			}
		}
		if cp.Grosz > 0 {
			ref := inc.ID
			if _, err := walletApply(tx, walletOp{
				UserID: uid, Kind: models.WalletTxAdjust, Amount: cp.Grosz,
				RefType: "incident", RefID: &ref, CreatedBy: &adminID,
				Note: "компенсация: " + incidentKindName(inc.Kind),
			}); err != nil {
				return err
			}
		}
		res := tx.Model(&models.Incident{}).
			Where("id = ? AND compensated_at IS NULL", inc.ID).
			Updates(map[string]any{
				"comp_minutes": cp.Minutes, "comp_grosz": cp.Grosz, "compensated_at": now,
				"resolved_at": now, "resolved_by": adminID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errIncidentCompensated // две кассы одновременно — компенсирует первая
		}
		inc.CompMinutes, inc.CompGrosz = cp.Minutes, cp.Grosz
		inc.CompensatedAt, inc.ResolvedAt, inc.ResolvedBy = &now, &now, &adminID
		return nil
	})
}

type incidentError string

func (e incidentError) Error() string { return string(e) }

const (
	errIncidentNoGuest     incidentError = "incident_no_guest"
	errIncidentCompensated incidentError = "incident_compensated"
)

// afterCompensation — то, что происходит после успешной компенсации:
// оживить сессию (свежие минуты/деньги — как депозит, К0-и2), уведомить
// гостя (оба клиента), аудит, live-лента админки.
func afterCompensation(c *gin.Context, inc *models.Incident, cp compensation) {
	uid := *inc.UserID
	reviveActiveSession(uid, nil)
	payload := map[string]any{
		"kind": inc.Kind, "minutes": cp.Minutes, "wallet_pln": models.PLNFromGrosz(cp.Grosz), "note": inc.Note,
	}
	if cp.Minutes > 0 && inc.ComputerID != nil { // Ж2: гостю — в какой зоне годны минуты
		if z := zoneByID(zoneOfComputer(*inc.ComputerID)); z != nil {
			payload["zone"] = z.Name
		}
	}
	notifyUser(uid, "incident_compensation", payload)
	logAdminAction(c, "incident_compensate", &uid, fmt.Sprintf("%s: %s%s", incidentKindName(inc.Kind), cp.text(), incidentNote(inc.Note)))
	hub.AdminBroadcast("incident", map[string]any{"id": inc.ID, "kind": inc.Kind, "open": false, "compensation": cp.text()})
}

func incidentNote(note string) string {
	if strings.TrimSpace(note) == "" {
		return ""
	}
	return " — " + strings.TrimSpace(note)
}

// ── Ж0-и1: инвариант цели (Ж-Р1) ─────────────────────────────────────────
//
// Решение №4 трека Б — «цель действия персонала — только гость и не сам
// исполнитель» — до ревью 14.09 на инциденты не распространялось: user_id
// принимался любой, и админ мог записать компенсацию себе или владельцу
// (проверено на стенде: 100 zł + 120 мин себе, 9999 zł владельцу). Теперь
// гость записи проходит ту же проверку, что депозит и грант.

// incidentTargetOK — гость записи по правилу canTargetUser; пишет 403 сам.
// Вернул nil — обработчику выходить.
func incidentTargetOK(c *gin.Context, uid uuid.UUID) *models.User {
	var u models.User
	if err := db.Select("id, role, nickname").First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость не найден"})
		return nil
	}
	if ok, code := canTargetUser(u.Role, u.ID.String(), c.GetString("user_id")); !ok {
		msg := map[string]string{
			"cannot_touch_self":  "Нельзя записать компенсацию самому себе",
			"cannot_touch_staff": "Компенсация — только гостю, персоналу нельзя",
		}[code]
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": msg})
		return nil
	}
	return &u
}

// sessionGuestForIncident — гость и сессия с ПК, если там идёт сессия ГОСТЯ.
// Персонал за ПК гостем записи не становится (иначе компенсация «по ПК» стала
// бы обходом инварианта) — запись создаётся без гостя.
func sessionGuestForIncident(pcID uuid.UUID) (userID, sessionID *uuid.UUID) {
	var rows []models.Session
	db.Preload("User").Where("computer_id = ? AND status = ?", pcID, models.SessionStatusActive).Limit(1).Find(&rows)
	if len(rows) == 0 || rows[0].User == nil || rows[0].User.Role != models.UserRolePlayer {
		return nil, nil
	}
	return &rows[0].UserID, &rows[0].ID
}

// ── Ж0-и2: дневной потолок компенсаций админа (Ж-Р2) ──────────────────────
//
// Потолок депозитов (Б0-и4) существует, чтобы админ не раздавал деньги; без
// потолка у компенсаций он обходился одной кнопкой (5000 zł при лимите
// депозитов 50). Две настройки владельца — деньги и минуты, 0 = без лимита,
// владелец не ограничен. Окно — сутки от полуночи, как у остальных потолков;
// Ж3 переведёт всё разом на клубные сутки.

// compCapVerdict — чистое правило (тест): что именно упёрлось в потолок.
func compCapVerdict(usedPLN, addPLN float64, capPLN int64, usedMin, addMin int, capMin int64) string {
	if adminDayCapExceeded(usedPLN, addPLN, float64(capPLN)) {
		return "cap_pln"
	}
	if adminDayCapExceeded(float64(usedMin), float64(addMin), float64(capMin)) {
		return "cap_min"
	}
	return ""
}

// compCapOK — проверка потолка для роли admin; пишет 403 day_cap сам.
func compCapOK(c *gin.Context, adminID uuid.UUID, cp compensation) bool {
	if cp.empty() || c.GetString("user_role") != string(models.UserRoleAdmin) {
		return true
	}
	capPLN := settingInt64("admin_day_comp_cap_pln", 0)
	capMin := settingInt64("admin_day_comp_cap_min", 0)
	if capPLN <= 0 && capMin <= 0 {
		return true
	}
	var used struct {
		Grosz   int64
		Minutes int64
	}
	db.Model(&models.Incident{}).
		Select("COALESCE(SUM(comp_grosz),0) AS grosz, COALESCE(SUM(comp_minutes),0) AS minutes").
		Where("resolved_by = ? AND compensated_at >= ?", adminID, startOfToday()).Scan(&used)
	usedPLN := models.PLNFromGrosz(used.Grosz)
	switch compCapVerdict(usedPLN, models.PLNFromGrosz(cp.Grosz), capPLN, int(used.Minutes), cp.Minutes, capMin) {
	case "cap_pln":
		c.JSON(http.StatusForbidden, gin.H{"code": "day_cap", "message": fmt.Sprintf(
			"Дневной лимит компенсаций админа: %d zł (уже выдано %.0f zł) — больше может только владелец", capPLN, usedPLN)})
		return false
	case "cap_min":
		c.JSON(http.StatusForbidden, gin.H{"code": "day_cap", "message": fmt.Sprintf(
			"Дневной лимит компенсаций админа: %d мин (уже выдано %d мин) — больше может только владелец", capMin, used.Minutes)})
		return false
	}
	return true
}

// ── выдача списка ──

type incidentRow struct {
	models.Incident
	Computer  string `json:"computer"`
	Nickname  string `json:"nickname"`
	AdminNick string `json:"admin"`
}

func incidentQuery() *gorm.DB {
	return db.Table("incidents AS i").
		Select("i.*, COALESCE(c.name, '') AS computer, COALESCE(u.nickname, '') AS nickname, COALESCE(a.nickname, '') AS admin_nick").
		Joins("LEFT JOIN computers c ON c.id = i.computer_id").
		Joins("LEFT JOIN users u ON u.id = i.user_id").
		Joins("LEFT JOIN users a ON a.id = COALESCE(i.resolved_by, i.created_by)")
}

func incidentJSON(r incidentRow) gin.H {
	return gin.H{
		"id": r.ID, "kind": r.Kind, "kind_name": incidentKindName(r.Kind), "auto": r.Auto,
		"computer_id": r.ComputerID, "computer": r.Computer, "user_id": r.UserID, "nickname": r.Nickname,
		"session_id": r.SessionID, "started_at": r.StartedAt, "ended_at": r.EndedAt, "minutes": r.Minutes,
		"note": r.Note, "comp_minutes": r.CompMinutes, "comp_pln": models.PLNFromGrosz(r.CompGrosz),
		"compensated_at": r.CompensatedAt, "resolved_at": r.ResolvedAt, "open": r.ResolvedAt == nil,
		"admin": r.AdminNick, "created_at": r.CreatedAt,
	}
}

// GET /admin/incidents?open=1 — журнал проблем: открытые всегда, закрытые —
// за сегодня (владелец — за всё время, до 200).
func handleAdminIncidents(c *gin.Context) {
	ownerView := c.GetString("user_role") == string(models.UserRoleOwner)
	q := incidentQuery().Order("i.created_at DESC").Limit(200)
	if c.Query("open") == "1" {
		q = q.Where("i.resolved_at IS NULL")
	} else if !ownerView {
		q = q.Where("i.resolved_at IS NULL OR i.created_at >= ?", startOfToday())
	}
	var rows []incidentRow
	if err := q.Scan(&rows).Error; err != nil {
		respondDBError(c, err)
		return
	}
	var open int64
	db.Model(&models.Incident{}).Where("resolved_at IS NULL").Count(&open)
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, incidentJSON(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "open": open})
}

type incidentCreateRequest struct {
	ComputerID  string  `json:"computer_id"`
	UserID      string  `json:"user_id"`
	SessionID   string  `json:"session_id"`
	Kind        string  `json:"kind" binding:"required"`
	Minutes     int     `json:"minutes"`
	Note        string  `json:"note"`
	CompMinutes int     `json:"comp_minutes"`
	CompPLN     float64 `json:"comp_pln"`
}

// POST /admin/incidents — записать проблему руками; с компенсацией — сразу
// закрытая, без — открытая. Гость и сессия подставляются с ПК, если там идёт
// сессия и их не указали.
func handleAdminIncidentCreate(c *gin.Context) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var req incidentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if _, ok := incidentKindNames[req.Kind]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "incident_kind_invalid", "message": "Вид проблемы: pc_hang, agent_lost, power или other"})
		return
	}
	if req.Minutes < 0 || req.CompMinutes < 0 || req.CompPLN < 0 || req.Minutes > 24*60 || req.CompMinutes > 24*60 || req.CompPLN > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "incident_amount_invalid", "message": "Минуты и сумма — от нуля до разумного"})
		return
	}
	now := time.Now()
	inc := models.Incident{Kind: req.Kind, Minutes: req.Minutes, Note: strings.TrimSpace(req.Note),
		StartedAt: now.Add(-time.Duration(req.Minutes) * time.Minute), EndedAt: &now, CreatedBy: &adminID}
	if req.ComputerID != "" {
		pcID, err := uuid.Parse(req.ComputerID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "computer_id_invalid", "message": "Некорректный идентификатор компьютера"})
			return
		}
		var pc models.Computer
		if err := db.First(&pc, "id = ?", pcID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "Компьютер не найден"})
			return
		}
		inc.ComputerID, inc.ClubID = &pc.ID, pc.ClubID
		// гость и сессия — с ПК, если там сидит ГОСТЬ и их не указали (Ж0-и1:
		// персонал за ПК гостем записи не становится)
		if req.UserID == "" {
			inc.UserID, inc.SessionID = sessionGuestForIncident(pcID)
		}
	}
	if req.UserID != "" {
		uid, err := uuid.Parse(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "user_id_invalid", "message": "Некорректный идентификатор гостя"})
			return
		}
		u := incidentTargetOK(c, uid) // Ж-Р1: только гость и не себе — 403 пишет сам
		if u == nil {
			return
		}
		inc.UserID = &u.ID
	}
	if req.SessionID != "" {
		if sid, err := uuid.Parse(req.SessionID); err == nil {
			inc.SessionID = &sid
		}
	}
	if inc.ComputerID == nil && inc.UserID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "incident_target_required", "message": "Укажи ПК или гостя"})
		return
	}
	if inc.ClubID == uuid.Nil { // без ПК — клуб по умолчанию (пилот — один клуб)
		var club models.Club
		if err := db.Order("created_at").First(&club).Error; err == nil {
			inc.ClubID = club.ID
		}
	}
	cp := compensation{Minutes: req.CompMinutes, Grosz: models.GroszFromPLN(req.CompPLN)}
	if !cp.empty() && inc.UserID == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "incident_no_guest", "message": "Компенсировать некому: за ПК никто не сидит — укажи гостя"})
		return
	}
	if !compCapOK(c, adminID, cp) { // Ж-Р2: потолок — ДО записи, иначе останется открытая запись без компенсации
		return
	}
	if err := db.Create(&inc).Error; err != nil {
		respondDBError(c, err)
		return
	}
	target := inc.UserID
	logAdminAction(c, "incident_create", target, fmt.Sprintf("%s%s, %d мин%s", incidentKindName(inc.Kind), incidentPlace(&inc), inc.Minutes, incidentNote(inc.Note)))
	if !cp.empty() {
		if err := applyCompensation(&inc, cp, adminID, now); err != nil {
			respondDBError(c, err)
			return
		}
		afterCompensation(c, &inc, cp)
	} else {
		hub.AdminBroadcast("incident", map[string]any{"id": inc.ID, "kind": inc.Kind, "open": true})
	}
	var row incidentRow
	incidentQuery().Where("i.id = ?", inc.ID).Scan(&row)
	c.JSON(http.StatusCreated, incidentJSON(row))
}

func incidentPlace(inc *models.Incident) string {
	if inc.ComputerID == nil {
		return ""
	}
	var pc models.Computer
	if db.Select("name").First(&pc, "id = ?", *inc.ComputerID).Error != nil {
		return ""
	}
	return " на " + pc.Name
}

type incidentCompensateRequest struct {
	CompMinutes int     `json:"comp_minutes"`
	CompPLN     float64 `json:"comp_pln"`
	Note        string  `json:"note"`
}

// POST /admin/incidents/:id/compensate — компенсация по открытой (или
// закрытой без компенсации) записи; второй раз — 409.
func handleAdminIncidentCompensate(c *gin.Context) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var inc models.Incident
	if err := db.First(&inc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "incident_not_found", "message": "Запись не найдена"})
		return
	}
	var req incidentCompensateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	cp := compensation{Minutes: req.CompMinutes, Grosz: models.GroszFromPLN(req.CompPLN)}
	if cp.empty() || req.CompMinutes < 0 || req.CompPLN < 0 || req.CompMinutes > 24*60 || req.CompPLN > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "incident_amount_invalid", "message": "Укажи минуты и/или сумму компенсации"})
		return
	}
	if inc.CompensatedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "incident_compensated", "message": "По этой записи компенсация уже была"})
		return
	}
	if inc.UserID == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "incident_no_guest", "message": "У записи нет гостя — компенсировать некому"})
		return
	}
	if incidentTargetOK(c, *inc.UserID) == nil { // Ж-Р1: гость записи — не персонал и не сам исполнитель
		return
	}
	if !compCapOK(c, adminID, cp) { // Ж-Р2
		return
	}
	if n := strings.TrimSpace(req.Note); n != "" {
		inc.Note = n
		db.Model(&inc).Update("note", n)
	}
	now := time.Now()
	if err := applyCompensation(&inc, cp, adminID, now); err != nil {
		if err == errIncidentCompensated {
			c.JSON(http.StatusConflict, gin.H{"code": "incident_compensated", "message": "По этой записи компенсация уже была"})
			return
		}
		respondDBError(c, err)
		return
	}
	afterCompensation(c, &inc, cp)
	var row incidentRow
	incidentQuery().Where("i.id = ?", inc.ID).Scan(&row)
	c.JSON(http.StatusOK, incidentJSON(row))
}

// POST /admin/incidents/:id/resolve — закрыть без компенсации.
func handleAdminIncidentResolve(c *gin.Context) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var inc models.Incident
	if err := db.First(&inc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "incident_not_found", "message": "Запись не найдена"})
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	if inc.ResolvedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "incident_resolved", "message": "Запись уже закрыта"})
		return
	}
	now := time.Now()
	upd := map[string]any{"resolved_at": now, "resolved_by": adminID}
	if n := strings.TrimSpace(req.Note); n != "" {
		upd["note"] = n
		inc.Note = n
	}
	if inc.EndedAt == nil { // открытая автозапись: агент так и не вернулся — длительность до сейчас
		upd["ended_at"] = now
		upd["minutes"] = incidentDurationMin(inc.StartedAt, now)
	}
	if err := db.Model(&inc).Updates(upd).Error; err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "incident_resolve", inc.UserID, fmt.Sprintf("%s%s — без компенсации%s", incidentKindName(inc.Kind), incidentPlace(&inc), incidentNote(inc.Note)))
	hub.AdminBroadcast("incident", map[string]any{"id": inc.ID, "kind": inc.Kind, "open": false})
	var row incidentRow
	incidentQuery().Where("i.id = ?", inc.ID).Scan(&row)
	c.JSON(http.StatusOK, incidentJSON(row))
}

// incidentsReport — компенсации за период для отчёта «Деньги» (Р7: рост
// обязательства, отдельной строкой) + сколько записей открыто сейчас.
func incidentsReport(from, to time.Time) gin.H {
	var agg struct {
		Cnt     int64
		Minutes int64
		Grosz   int64
	}
	db.Model(&models.Incident{}).
		Select("COUNT(*) AS cnt, COALESCE(SUM(comp_minutes),0) AS minutes, COALESCE(SUM(comp_grosz),0) AS grosz").
		Where("compensated_at >= ? AND compensated_at < ?", from, to).Scan(&agg)
	var total, open int64
	db.Model(&models.Incident{}).Where("created_at >= ? AND created_at < ?", from, to).Count(&total)
	db.Model(&models.Incident{}).Where("resolved_at IS NULL").Count(&open)
	return gin.H{
		"period_count": total, "compensated": agg.Cnt,
		"comp_minutes": agg.Minutes, "comp_pln": models.PLNFromGrosz(agg.Grosz),
		"open": open,
	}
}
