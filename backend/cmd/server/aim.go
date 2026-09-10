package main

// С4 (SENS.md «После пилота»): история калибровок AI-калибратора, тёплый
// старт и аим-рейтинг клуба.
//
//   POST /me/aim-sessions — шелл кладёт сюда aim_result калибратора (и1);
//   GET  /me/aim-sessions — 10 последних + warm (откуда стартовать в следующий
//                           раз) + stats («реакция 210 → 185 мс»);
//   GET  /aim-rating      — топ гостей по лучшему throughput (бит/с, и4).
//
// Единицы: sens — CS2 при dpi прогона; cm/360 — инвариант, сервер считает его
// сам из sens×dpi (клиенту не доверяем). Синтетические прогоны стенда (sim)
// хранятся, но в warm/рейтинг/отчёт не попадают; частичные (partial>0) — в
// историю да, в warm и рейтинг нет: там оптимум не сошёлся.

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	cs2Yaw             = 0.022 // градусов на отсчёт мыши при sens 1 (CS2)
	aimKeepPerUser     = 200   // строк истории на гостя
	aimHistoryLimit    = 10    // по умолчанию в GET /me/aim-sessions
	aimRatingTopN      = 10
	aimRatingMinRounds = 6 // меньше — не калибровка, а проба
)

// aimCm360 — см на оборот при sens (CS2) и dpi. Чистая функция (тест).
func aimCm360(sens float64, dpi int) float64 {
	if sens <= 0 || dpi <= 0 {
		return 0
	}
	return math.Round(360*2.54/(cs2Yaw*sens*float64(dpi))*100) / 100
}

type aimSessionRequest struct {
	Sens     float64 `json:"sens"`
	DPI      int     `json:"dpi"`
	Cur      float64 `json:"cur"`
	Lo       float64 `json:"lo"`
	Hi       float64 `json:"hi"`
	Conf     string  `json:"conf"`
	Verdict  string  `json:"verdict"`
	GainPct  float64 `json:"gain_pct"`
	Acc1     *int    `json:"acc1"`
	RtMs     int     `json:"rt_ms"`
	MtMs     int     `json:"mt_ms"`
	Tp       float64 `json:"tp"`
	Mode     string  `json:"mode"`
	Rounds   int     `json:"rounds"`
	AbRounds int     `json:"ab_rounds"`
	Partial  int     `json:"partial"`
	Raw      *bool   `json:"raw"`
	Hz       int     `json:"hz"`
	Coach    string  `json:"coach"`
	Warm     bool    `json:"warm"`
	Lang     string  `json:"lang"`
	Sim      bool    `json:"sim"`
}

func oneOf(v string, set ...string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// validateAimSession — проверка и нормализация прогона: диапазоны чисел,
// коды conf/verdict/mode/coach, язык. Пустые coach/lang — дефолты. Чистая
// функция (тест): code == "" — ок.
func validateAimSession(r *aimSessionRequest) (code string) {
	if r.Sens < 0.01 || r.Sens > 100 || r.Cur < 0 || r.Cur > 100 {
		return "bad_sens"
	}
	if r.DPI < minDPI || r.DPI > maxDPI {
		return "bad_dpi"
	}
	if r.Lo < 0 || r.Hi < 0 || r.Lo > 100 || r.Hi > 100 || (r.Hi > 0 && r.Lo > r.Hi) {
		return "bad_range"
	}
	if !oneOf(r.Conf, "high", "mid", "low") {
		return "bad_conf"
	}
	if !oneOf(r.Verdict, "rec", "cur", "tie", "none") {
		return "bad_verdict"
	}
	if r.GainPct < -100 || r.GainPct > 1000 {
		return "bad_gain"
	}
	if r.Acc1 != nil && (*r.Acc1 < 0 || *r.Acc1 > 100) {
		return "bad_acc1"
	}
	if r.RtMs < 0 || r.RtMs > 10000 || r.MtMs < 0 || r.MtMs > 10000 {
		return "bad_ms"
	}
	if r.Tp < 0 || r.Tp > 50 {
		return "bad_tp"
	}
	if !oneOf(r.Mode, "quick", "precise", "custom") {
		return "bad_mode"
	}
	if r.Rounds < 0 || r.Rounds > 60 || r.AbRounds < 0 || r.AbRounds > 12 || r.Partial < 0 || r.Partial > 60 {
		return "bad_rounds"
	}
	if r.Hz < 0 || r.Hz > 10000 {
		return "bad_hz"
	}
	if r.Coach == "" {
		r.Coach = "none"
	}
	if !oneOf(r.Coach, "over", "under", "none") {
		return "bad_coach"
	}
	if r.Lang == "" || !oneOf(r.Lang, "ru", "en", "pl") {
		r.Lang = "ru"
	}
	return ""
}

// aimWarmPick — откуда стартовать следующую калибровку: последний настоящий
// полный прогон; если A/B подтвердил текущую (cur/tie) — её cm/360, иначе
// рекомендацию. rows — новые первыми. Чистая функция (тест).
func aimWarmPick(rows []models.AimSession) *models.AimSession {
	for i := range rows {
		r := &rows[i]
		if r.Sim || r.Partial > 0 {
			continue
		}
		return r
	}
	return nil
}

func aimWarmOut(r *models.AimSession) gin.H {
	if r == nil {
		return nil
	}
	cm, sens := r.Cm360, r.Sens
	if r.Verdict == "cur" || r.Verdict == "tie" {
		cm, sens = r.CurCm, r.Cur
	}
	if cm <= 0 {
		return nil
	}
	return gin.H{"cm360": cm, "sens": sens, "dpi": r.DPI, "verdict": r.Verdict, "at": r.CreatedAt}
}

// aimStats — сводка по настоящим полным прогонам: сколько, реакция первого и
// последнего («210 → 185 мс»), лучшие throughput и первый выстрел.
// rows — новые первыми. Чистая функция (тест).
func aimStats(rows []models.AimSession) gin.H {
	var real []models.AimSession
	for _, r := range rows {
		if !r.Sim && r.Partial == 0 {
			real = append(real, r)
		}
	}
	out := gin.H{"count": len(real)}
	if len(real) == 0 {
		return out
	}
	last, first := real[0], real[len(real)-1]
	out["last_rt"] = last.RtMs
	out["first_rt"] = first.RtMs
	bestTp, bestAcc := 0.0, -1
	for _, r := range real {
		if r.Tp > bestTp {
			bestTp = r.Tp
		}
		if r.Acc1 != nil && *r.Acc1 > bestAcc {
			bestAcc = *r.Acc1
		}
	}
	out["best_tp"] = bestTp
	if bestAcc >= 0 {
		out["best_acc1"] = bestAcc
	}
	return out
}

// POST /me/aim-sessions — записать прогон калибратора.
func handlePostAimSession(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var req aimSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if code := validateAimSession(&req); code != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": "Некорректный результат калибровки"})
		return
	}
	row := models.AimSession{
		UserID: userID, Sens: req.Sens, DPI: req.DPI, Cm360: aimCm360(req.Sens, req.DPI),
		Cur: req.Cur, CurCm: aimCm360(req.Cur, req.DPI), Lo: req.Lo, Hi: req.Hi,
		Conf: req.Conf, Verdict: req.Verdict, GainPct: req.GainPct, Acc1: req.Acc1,
		RtMs: req.RtMs, MtMs: req.MtMs, Tp: req.Tp, Mode: req.Mode, Rounds: req.Rounds,
		AbRounds: req.AbRounds, Partial: req.Partial, Raw: req.Raw, Hz: req.Hz,
		Coach: req.Coach, Warm: req.Warm, Lang: req.Lang, Sim: req.Sim,
	}
	if err := db.Create(&row).Error; err != nil {
		respondDBError(c, err)
		return
	}
	// история не растёт бесконечно: держим aimKeepPerUser последних
	var ids []uuid.UUID
	db.Model(&models.AimSession{}).Where("user_id = ?", userID).
		Order("created_at DESC").Offset(aimKeepPerUser).Pluck("id", &ids)
	if len(ids) > 0 {
		db.Where("id IN ?", ids).Delete(&models.AimSession{})
	}
	c.JSON(http.StatusOK, gin.H{"saved": true, "id": row.ID, "at": row.CreatedAt, "cm360": row.Cm360})
}

func aimHistoryFor(userID string, limit int) []models.AimSession {
	var rows []models.AimSession
	db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Find(&rows)
	return rows
}

// GET /me/aim-sessions?limit=10 — история + тёплый старт + сводка.
func handleGetAimSessions(c *gin.Context) {
	userID := c.GetString("user_id")
	limit := aimHistoryLimit
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 50 {
		limit = v
	}
	rows := aimHistoryFor(userID, limit)
	// warm и stats смотрят глубже лимита истории: последний полный прогон мог
	// уехать за 10 частичных/симовых
	deep := rows
	if len(rows) == limit {
		deep = aimHistoryFor(userID, aimKeepPerUser)
	}
	c.JSON(http.StatusOK, gin.H{
		"items": rows,
		"warm":  aimWarmOut(aimWarmPick(deep)),
		"stats": aimStats(deep),
	})
}

// ── аим-рейтинг клуба (и4) ────────────────────────────────────────────

type aimBest struct {
	UserID    string
	Tp        float64
	Acc1      *int
	RtMs      int
	CreatedAt time.Time
}

type aimRatingRow struct {
	Rank     int       `json:"rank"`
	UserID   string    `json:"user_id"`
	Nickname string    `json:"nickname"`
	AvatarID int       `json:"avatar_id"`
	Level    int       `json:"level"`
	Tp       float64   `json:"tp"`
	Acc1     *int      `json:"acc1"`
	RtMs     int       `json:"rt_ms"`
	At       time.Time `json:"at"`
	IsMe     bool      `json:"is_me"`
}

// rankAim — порядок рейтинга: throughput по убыванию, при равенстве — точнее
// первый выстрел, затем раньше. Возвращает топ-N и место me (0 — нет в
// рейтинге). Чистая функция (тест).
func rankAim(best []aimBest, me string, topN int) ([]aimRatingRow, int, float64) {
	sort.SliceStable(best, func(i, j int) bool {
		if best[i].Tp != best[j].Tp {
			return best[i].Tp > best[j].Tp
		}
		ai, aj := -1, -1
		if best[i].Acc1 != nil {
			ai = *best[i].Acc1
		}
		if best[j].Acc1 != nil {
			aj = *best[j].Acc1
		}
		if ai != aj {
			return ai > aj
		}
		return best[i].CreatedAt.Before(best[j].CreatedAt)
	})
	top := make([]aimRatingRow, 0, topN)
	myRank, myTp := 0, 0.0
	for i, b := range best {
		if b.UserID == me {
			myRank, myTp = i+1, b.Tp
		}
		if i < topN {
			top = append(top, aimRatingRow{Rank: i + 1, UserID: b.UserID, Tp: b.Tp, Acc1: b.Acc1, RtMs: b.RtMs, At: b.CreatedAt, IsMe: b.UserID == me})
		}
	}
	return top, myRank, myTp
}

// GET /aim-rating — топ гостей по лучшему настоящему полному прогону.
// Только активные игроки (как в /leaderboard): персонал и забаненные — нет.
func handleAimRating(c *gin.Context) {
	userID := c.GetString("user_id")
	var best []aimBest
	db.Raw(`SELECT DISTINCT ON (a.user_id) a.user_id::text AS user_id, a.tp, a.acc1, a.rt_ms, a.created_at
	          FROM aim_sessions a JOIN users u ON u.id = a.user_id
	         WHERE a.sim = FALSE AND a.partial = 0 AND a.rounds >= ? AND u.role = ? AND u.status = ?
	         ORDER BY a.user_id, a.tp DESC, a.created_at ASC`,
		aimRatingMinRounds, models.UserRolePlayer, models.UserStatusActive).Scan(&best)
	top, myRank, myTp := rankAim(best, userID, aimRatingTopN)
	if len(top) > 0 {
		ids := make([]string, 0, len(top))
		for _, r := range top {
			ids = append(ids, r.UserID)
		}
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		byID := map[string]models.User{}
		for _, u := range users {
			byID[u.ID.String()] = u
		}
		for i := range top {
			if u, ok := byID[top[i].UserID]; ok {
				top[i].Nickname, top[i].AvatarID, top[i].Level = u.Nickname, u.AvatarID, u.Level
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"top": top, "me": gin.H{"rank": myRank, "tp": myTp, "total": len(best)}})
}

// aimReportCounts — сколько настоящих калибровок и уникальных гостей за
// период (отчёт владельца «Гости», С4-и1).
func aimReportCounts(from, to time.Time) (sessions, guests int64) {
	var r struct {
		N int64
		U int64
	}
	db.Model(&models.AimSession{}).Select("COUNT(*) AS n, COUNT(DISTINCT user_id) AS u").
		Where("sim = FALSE AND created_at >= ? AND created_at < ?", from, to).Scan(&r)
	return r.N, r.U
}
