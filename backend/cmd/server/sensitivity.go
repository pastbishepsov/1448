package main

// Профиль сенсы игрока (S3): DPI + сенсы по играм, синхронизируются с аккаунтом
// и подставляются на любом клубном ПК. GET/PUT /me/sensitivity.
//
// С2 (SENS.md, Р2/Т4/Т5): инвариант профиля — cm/360, а не пара dpi+sens.
// DPI — свойство клубной мыши (настройка mouse_dpi), поэтому на ПК сенса
// уезжает пересчитанной под мышь клуба: sens × dpi_профиля / mouse_dpi (yaw
// для этого не нужен: внутри одной игры sens × dpi = const при одном cm/360).
// «Применить сейчас» идёт через сервер: PUT при активной сессии шлёт на ПК
// гостя WS-команду sens_apply, агент отвечает sens_result → уведомление
// sens_applied гостю (киоск и PWA показывают тост).

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

const (
	minDPI        = 100
	maxDPI        = 32000
	maxSensVal    = 1000.0
	maxSensGame   = 64 // ключей в профиле
	mouseDPIDef   = 0  // С2-и1: 0 = DPI мыши клуба не задан — пересчёта нет
	mousepadCmDef = 0  // С4-и3: 0 = ширина коврика клуба не задана — студия предложит лист A4
)

// validateSensitivity — чистая проверка профиля (тестируется отдельно).
func validateSensitivity(dpi int, games map[string]float64) (ok bool, code string) {
	if dpi < minDPI || dpi > maxDPI {
		return false, "bad_dpi"
	}
	if len(games) > maxSensGame {
		return false, "too_many"
	}
	for id, s := range games {
		if id == "" || len(id) > 32 {
			return false, "bad_game"
		}
		if s <= 0 || s > maxSensVal {
			return false, "bad_sens"
		}
	}
	return true, ""
}

// rescaleSens — та же сенса при другом DPI (cm/360 не меняется):
// sens' = sens × fromDPI / toDPI. Чистая функция (тестируется).
func rescaleSens(games map[string]float64, fromDPI, toDPI int) map[string]float64 {
	out := make(map[string]float64, len(games))
	k := float64(fromDPI) / float64(toDPI)
	for g, s := range games {
		out[g] = roundSens(s * k)
	}
	return out
}

// roundSens — до 4 знаков: столько же пишет агент в конфиг (formatSens).
func roundSens(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}

// sensPayload — что уходит на ПК: {dpi, games} (+ rescaled_from, если сенса
// пересчитана под мышь клуба). mouseDPI 0 или равный DPI профиля — как есть.
func sensPayload(dpi int, games map[string]float64, mouseDPI int) gin.H {
	if mouseDPI > 0 && dpi > 0 && dpi != mouseDPI {
		return gin.H{"dpi": mouseDPI, "games": rescaleSens(games, dpi, mouseDPI), "rescaled_from": dpi}
	}
	return gin.H{"dpi": dpi, "games": games}
}

// clubMouseDPI — DPI мыши клуба из настроек (0 = не задан).
func clubMouseDPI() int { return int(settingInt64("mouse_dpi", mouseDPIDef)) }

// GET /me/sensitivity — профиль сенсы игрока (дефолт, если ещё не сохранял).
func handleGetSensitivity(c *gin.Context) {
	userID := c.GetString("user_id")

	var p models.SensitivityProfile
	if err := db.First(&p, "user_id = ?", userID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"dpi": 800, "games": gin.H{}, "saved": false})
		return
	}
	games := map[string]float64{}
	_ = json.Unmarshal([]byte(p.Games), &games)
	c.JSON(http.StatusOK, gin.H{
		"dpi": p.DPI, "games": games, "saved": true, "updated_at": p.UpdatedAt,
	})
}

// sensPayloadFor — профиль сенсы гостя для агента в session_start (К7) и при
// пересадке, пересчитанный под мышь клуба (С2-и1); nil — гость ничего не
// сохранял.
func sensPayloadFor(userID uuid.UUID) gin.H {
	var rows []models.SensitivityProfile
	db.Where("user_id = ?", userID).Limit(1).Find(&rows)
	if len(rows) == 0 {
		return nil
	}
	games := map[string]float64{}
	_ = json.Unmarshal([]byte(rows[0].Games), &games)
	if len(games) == 0 {
		return nil
	}
	return sensPayload(rows[0].DPI, games, clubMouseDPI())
}

type sensitivityRequest struct {
	DPI      int                `json:"dpi" binding:"required"`
	Games    map[string]float64 `json:"games"`
	OwnMouse bool               `json:"own_mouse"` // С2-и5: гость со своей мышью — DPI как прислал, без пересчёта
}

// PUT /me/sensitivity — сохранить/обновить профиль сенсы (upsert по user_id).
// С2-и4 (Т5): гость за клубным ПК с активной сессией — сенса тут же уходит на
// его ПК командой sens_apply (ответ pushed: true); агент применяет и отвечает
// sens_result → уведомление sens_applied.
func handlePutSensitivity(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var req sensitivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if req.Games == nil {
		req.Games = map[string]float64{}
	}
	if ok, code := validateSensitivity(req.DPI, req.Games); !ok {
		msg := map[string]string{
			"bad_dpi":  "DPI должен быть от 100 до 32000",
			"too_many": "Слишком много игр в профиле",
			"bad_game": "Некорректный идентификатор игры",
			"bad_sens": "Сенса должна быть больше 0",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	gamesJSON, _ := json.Marshal(req.Games)
	profile := models.SensitivityProfile{
		UserID: userID, DPI: req.DPI, Games: string(gamesJSON),
	}
	// upsert: создать или обновить dpi/games/updated_at
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"dpi", "games", "updated_at"}),
	}).Create(&profile).Error; err != nil {
		respondDBError(c, err)
		return
	}

	pushed := false
	if len(req.Games) > 0 {
		pushed = pushSensToActivePC(userID, req.DPI, req.Games, req.OwnMouse)
	}
	c.JSON(http.StatusOK, gin.H{"dpi": req.DPI, "games": req.Games, "saved": true, "pushed": pushed})
}

// pushSensToActivePC — активная сессия гостя + ПК на связи → sens_apply на ПК.
// ownMouse: гость сказал «со своей мышью» — DPI и сенсы как прислал.
func pushSensToActivePC(userID uuid.UUID, dpi int, games map[string]float64, ownMouse bool) bool {
	var s models.Session
	if err := db.Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Order("started_at DESC").First(&s).Error; err != nil {
		return false
	}
	computerID := s.ComputerID.String()
	if !hub.IsConnected(computerID) {
		return false
	}
	mouseDPI := 0
	if !ownMouse {
		mouseDPI = clubMouseDPI()
	}
	notifyShell(computerID, websocket.MsgSensApply, sensPayload(dpi, games, mouseDPI))
	return true
}

// sensResultFromShell — агент применил сенсу (ответ на sens_apply): гостю за
// этим ПК — уведомление sens_applied {applied, skipped, dry_run, running}.
// Хук хаба, вызывается из горутины чтения WS.
func sensResultFromShell(computerID string, payload json.RawMessage) {
	var r struct {
		Applied []string          `json:"applied"`
		Skipped map[string]string `json:"skipped"`
		DryRun  bool              `json:"dry_run"`
		Running []string          `json:"running"`
	}
	if err := json.Unmarshal(payload, &r); err != nil {
		log.Printf("sens_result от %s не разобрался: %v", computerID, err)
		return
	}
	var s models.Session
	if err := db.Where("computer_id = ? AND status = ?", computerID, models.SessionStatusActive).
		Order("started_at DESC").First(&s).Error; err != nil {
		return // сессии уже нет — тост некому
	}
	if r.Applied == nil {
		r.Applied = []string{}
	}
	if r.Skipped == nil {
		r.Skipped = map[string]string{}
	}
	if r.Running == nil {
		r.Running = []string{}
	}
	notifyUser(s.UserID, "sens_applied", map[string]any{
		"applied": r.Applied, "skipped": r.Skipped, "dry_run": r.DryRun, "running": r.Running,
	})
}
