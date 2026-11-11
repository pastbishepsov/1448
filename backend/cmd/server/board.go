package main

// Ж8-и3 «Табло зала» (ADMIN.md, трек Ж; решение Ж-Р12, ревью 14.09 №37).
//
// Отдельная страница под ТВ у входа: зал по зонам (свободно / занято — БЕЗ
// ников: кто за какой машиной — не дело прохожего), очередь по никам (тут
// ник и нужен — «Guest10, ваш ПК свободен»), ближайшие брони по ПК. Своего
// логина у табло нет — как у агента: токен `board_token` в настройках
// владельца, страница ходит с ним в `GET /board/state?token=…` раз в 10 с.
// Пустой токен = табло выключено (401 на всё): случайно открытое табло не
// должно светить занятость зала в интернет.
//
// Токен генерирует сервер (POST /admin/board/token — 24 случайных байта), а не
// владелец руками: «1234» у входа в клуб — не секрет. Владелец видит токен и
// готовую ссылку в «Экономике», может перевыпустить (старая ссылка умирает).

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const boardTokenKey = "board_token"

// boardTokenOK — совпал ли токен запроса с настройкой (чистая, тест):
// пустая настройка — табло выключено, ничего не подходит.
func boardTokenOK(setting, presented string) bool {
	setting, presented = strings.TrimSpace(setting), strings.TrimSpace(presented)
	if setting == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(setting), []byte(presented)) == 1
}

func newBoardToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GET /admin/board/token — токен табло и ссылка (owner).
func handleAdminBoardTokenGet(c *gin.Context) {
	tok := settingString(boardTokenKey, "")
	c.JSON(http.StatusOK, gin.H{"token": tok, "enabled": tok != "", "path": boardPath(tok)})
}

// POST /admin/board/token — выпустить новый токен (owner); старый перестаёт
// действовать сразу — табло у входа надо переоткрыть по новой ссылке.
func handleAdminBoardTokenRotate(c *gin.Context) {
	tok, err := newBoardToken()
	if err != nil {
		respondDBError(c, err)
		return
	}
	if err := settingSetString(boardTokenKey, tok); err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "board_token", nil, "табло: выпущен новый токен")
	c.JSON(http.StatusOK, gin.H{"token": tok, "enabled": true, "path": boardPath(tok)})
}

// DELETE /admin/board/token — выключить табло (owner).
func handleAdminBoardTokenDelete(c *gin.Context) {
	if err := settingSetString(boardTokenKey, ""); err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "board_token", nil, "табло выключено")
	c.JSON(http.StatusOK, gin.H{"token": "", "enabled": false, "path": ""})
}

func boardPath(tok string) string {
	if tok == "" {
		return ""
	}
	return "/board?token=" + tok
}

// boardZone — зона на табло: машины без ников.
type boardZone struct {
	Name     string   `json:"name"`
	RatePLN  float64  `json:"rate_pln"`
	Free     int      `json:"free"`
	Total    int      `json:"total"`
	FreeList []string `json:"free_list"` // имена свободных ПК — гость с очереди ищет свою машину
	Busy     []string `json:"busy_list"` // занятые — без ников
}

// boardTokenOf — токен запроса табло (З19-и3, №40): заголовок X-Board-Token; в адресе (?token=…) — только у вкладок
// прошлой версии, открытых на ТВ до обновления. Заголовок не оседает в журнале запросов и в истории прокси.
func boardTokenOf(c *gin.Context) string {
	if t := c.GetHeader("X-Board-Token"); t != "" {
		return t
	}
	return c.Query("token")
}

// GET /board/state — состояние зала для табло (без логина, по токену в заголовке X-Board-Token).
func handleBoardState(c *gin.Context) {
	if !boardTokenOK(settingString(boardTokenKey, ""), boardTokenOf(c)) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "board_token", "message": "Табло выключено или токен не подходит"})
		return
	}
	now := time.Now()
	club, _ := defaultClub()

	var computers []models.Computer
	db.Order("name").Find(&computers)
	var zones []models.Zone
	db.Order("sort, name").Find(&zones)
	zoneName := map[string]*boardZone{}
	order := []string{}
	get := func(key, name string, rate float64) *boardZone {
		if z, ok := zoneName[key]; ok {
			return z
		}
		z := &boardZone{Name: name, RatePLN: rate, FreeList: []string{}, Busy: []string{}}
		zoneName[key] = z
		order = append(order, key)
		return z
	}
	for _, z := range zones {
		get(z.ID.String(), z.Name, z.RatePLN)
	}
	clubRate := 0.0
	if club != nil {
		clubRate = club.BaseRatePLN
	}
	for _, pc := range computers {
		if pc.Status == models.ComputerStatusMaintenance {
			continue // в ремонте — не «занят» и не «свободен», табло его не считает
		}
		key, name := "", "Зал"
		if pc.ZoneID != nil {
			key, name = pc.ZoneID.String(), pc.Zone
		}
		z := get(key, name, clubRate)
		if key != "" {
			z.RatePLN = zoneName[key].RatePLN
		}
		z.Total++
		if pc.Status == models.ComputerStatusAvailable && !computerHeld(&pc, now) {
			z.Free++
			z.FreeList = append(z.FreeList, pc.Name)
		} else {
			z.Busy = append(z.Busy, pc.Name)
		}
	}
	zonesOut := make([]boardZone, 0, len(order))
	freeTotal, total := 0, 0
	for _, k := range order {
		z := zoneName[k]
		if z.Total == 0 {
			continue
		}
		zonesOut = append(zonesOut, *z)
		freeTotal += z.Free
		total += z.Total
	}

	// очередь — по никам (Ж-Р12), в порядке постановки
	var entries []models.WaitlistEntry
	db.Preload("User").Where("status = ?", models.WaitlistStatusWaiting).Order("created_at").Limit(20).Find(&entries)
	queue := make([]gin.H, 0, len(entries))
	for i, e := range entries {
		queue = append(queue, gin.H{"position": i + 1, "nickname": nicknameOf(e.User),
			"waited_min": int(now.Sub(e.CreatedAt).Minutes()), "called": e.NotifiedAt != nil})
	}

	// ближайшие живые брони на ближайшие 3 часа — по ПК и времени, без ников
	var bookings []models.Booking
	db.Preload("Computer").Where("status IN ? AND start_time >= ? AND start_time < ?",
		liveBookingStatuses, now.Add(-10*time.Minute), now.Add(3*time.Hour)).
		Order("start_time").Limit(12).Find(&bookings)
	bkOut := make([]gin.H, 0, len(bookings))
	for _, b := range bookings {
		row := gin.H{"start_time": b.StartTime, "duration_min": b.DurationMin}
		if b.Computer != nil {
			row["computer"] = b.Computer.Name
		}
		bkOut = append(bkOut, row)
	}
	sort.SliceStable(bkOut, func(i, j int) bool {
		return bkOut[i]["start_time"].(time.Time).Before(bkOut[j]["start_time"].(time.Time))
	})

	resp := gin.H{
		"now": now, "zones": zonesOut, "free": freeTotal, "total": total,
		"queue": queue, "bookings": bkOut, "refresh_sec": 10,
	}
	if club != nil {
		resp["club"] = club.Name
	}
	c.JSON(http.StatusOK, resp)
}
