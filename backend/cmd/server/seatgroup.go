package main

// Ж8-и2 «Посадка компании» (ADMIN.md, трек Ж; решение Ж-Р13, ревью 14.09 №35).
//
// Компания из 2–6 гостей хочет сесть рядом. До этого спринта админ сажал их
// по одному, каждый раз выбирая машину глазами по схеме. Теперь: гости через
// общий резолвер, сервер предлагает соседние свободные ПК по координатам
// схемы зала (pos_x/pos_y, минимум суммарного попарного расстояния), админ
// правит выбор, «Посадить всех» — одним запросом. Каждая посадка — обычный
// startSessionFor: окно [Готов!], WoL и ожидание шелла (Ж4), порог старта,
// бронь на ПК — всё как у одиночной. Платит каждый со своего кошелька (Р1
// трека Г). Частичный успех честно перечислен: кто сел, кто нет и почему —
// «всё или ничего» здесь вредно: у троих деньги есть, у четвёртого нет, трое
// не должны ждать.

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	groupMin = 2
	groupMax = 6
)

// pcPoint — свободный ПК с координатой на схеме (или без неё).
type pcPoint struct {
	ID   string
	Name string
	X, Y *int
}

// pickAdjacent — выбрать n машин из свободных так, чтобы суммарное попарное
// расстояние между ними было минимальным (чистая, тест). Машины без координат
// идут последними — их «рядом» никто не проверит. Кандидатов у стойки мало
// (десятки), полный перебор сочетаний при n ≤ 6 и ≤ 20 машин укладывается в
// тысячи вариантов; больше 20 — берём 20 ближайших к первому свободному.
func pickAdjacent(free []pcPoint, n int) []pcPoint {
	if n <= 0 || len(free) == 0 {
		return nil
	}
	pos := make([]pcPoint, 0, len(free))
	nopos := make([]pcPoint, 0)
	for _, p := range free {
		if p.X != nil && p.Y != nil {
			pos = append(pos, p)
		} else {
			nopos = append(nopos, p)
		}
	}
	if len(pos) > 20 {
		pos = pos[:20]
	}
	best := []pcPoint(nil)
	bestCost := math.MaxFloat64
	if len(pos) >= n {
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		for {
			cost := 0.0
			for i := 0; i < n; i++ {
				for j := i + 1; j < n; j++ {
					a, b := pos[idx[i]], pos[idx[j]]
					dx, dy := float64(*a.X-*b.X), float64(*a.Y-*b.Y)
					cost += math.Hypot(dx, dy)
				}
			}
			if cost < bestCost {
				bestCost = cost
				best = make([]pcPoint, n)
				for i, k := range idx {
					best[i] = pos[k]
				}
			}
			// следующее сочетание
			i := n - 1
			for i >= 0 && idx[i] == len(pos)-n+i {
				i--
			}
			if i < 0 {
				break
			}
			idx[i]++
			for j := i + 1; j < n; j++ {
				idx[j] = idx[j-1] + 1
			}
		}
		return best
	}
	// координат не хватает — что есть, плюс машины без координат
	out := append([]pcPoint{}, pos...)
	for _, p := range nopos {
		if len(out) >= n {
			break
		}
		out = append(out, p)
	}
	return out
}

// freePCsForGroup — свободные ПК (без придержанных и без брони в ближайшие
// planned минут), при заданной зоне — только её.
func freePCsForGroup(zoneID *uuid.UUID, plannedMin int, now time.Time) []pcPoint {
	var pcs []models.Computer
	q := db.Where("status = ?", models.ComputerStatusAvailable).Order("name")
	if zoneID != nil {
		q = q.Where("zone_id = ?", *zoneID)
	}
	q.Find(&pcs)
	out := make([]pcPoint, 0, len(pcs))
	for i := range pcs {
		pc := &pcs[i]
		if computerHeld(pc, now) {
			continue
		}
		if !hub.IsConnected(pc.ID.String()) && (pc.MAC == nil || *pc.MAC == "") {
			// без связи и без MAC машину не разбудить — компании такую не предлагаем
			continue
		}
		if nb := nextForeignBooking(pc.ID, uuid.Nil, now); nb != nil {
			lock := time.Duration(settingInt64("booking_lock_min", bookingLockMinDef)) * time.Minute
			if nb.StartTime.Add(-lock).Before(now.Add(time.Duration(plannedMin) * time.Minute)) {
				continue // чужая бронь отрежет раньше, чем компания доиграет
			}
		}
		out = append(out, pcPoint{ID: pc.ID.String(), Name: pc.Name, X: pc.PosX, Y: pc.PosY})
	}
	return out
}

// GET /admin/seat-group/suggest?n=3&zone_id=&planned_min=60 — какие ПК предложить.
func handleAdminSeatGroupSuggest(c *gin.Context) {
	n := 0
	fmt.Sscanf(c.Query("n"), "%d", &n)
	if n < groupMin || n > groupMax {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_size", "message": fmt.Sprintf("Компания — от %d до %d гостей", groupMin, groupMax)})
		return
	}
	planned := 60
	fmt.Sscanf(c.Query("planned_min"), "%d", &planned)
	if planned <= 0 {
		planned = 60
	}
	var zoneID *uuid.UUID
	if z := strings.TrimSpace(c.Query("zone_id")); z != "" {
		id, err := uuid.Parse(z)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_zone", "message": "zone_id — UUID зоны"})
			return
		}
		zoneID = &id
	}
	free := freePCsForGroup(zoneID, planned, time.Now())
	pick := pickAdjacent(free, n)
	names := func(list []pcPoint) []gin.H {
		out := make([]gin.H, 0, len(list))
		for _, p := range list {
			row := gin.H{"id": p.ID, "name": p.Name}
			if p.X != nil && p.Y != nil {
				row["pos_x"], row["pos_y"] = *p.X, *p.Y
			}
			out = append(out, row)
		}
		return out
	}
	c.JSON(http.StatusOK, gin.H{"n": n, "suggested": names(pick), "free": names(free), "enough": len(pick) >= n})
}

// POST /admin/seat-group {seats:[{guest_id|guest|nickname, computer_id}], planned_min}
// — посадить компанию. Ответ 200 всегда, когда запрос разобран: по каждому
// месту либо сессия, либо код ошибки — стойка показывает, кого пересадить.
func handleAdminSeatGroup(c *gin.Context) {
	var req struct {
		Seats []struct {
			GuestID    string `json:"guest_id"`
			Guest      string `json:"guest"`
			Nickname   string `json:"nickname"`
			ComputerID string `json:"computer_id"`
		} `json:"seats"`
		PlannedMin *int `json:"planned_min"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен список мест: гость + ПК"})
		return
	}
	if len(req.Seats) < groupMin || len(req.Seats) > groupMax {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_size", "message": fmt.Sprintf("Компания — от %d до %d гостей", groupMin, groupMax)})
		return
	}
	// один ПК на одно место и один гость на одно место — проверяем до посадок
	seenPC, seenGuest := map[string]bool{}, map[string]bool{}
	for _, s := range req.Seats {
		if strings.TrimSpace(s.ComputerID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "need_computer", "message": "У каждого гостя должен быть свой ПК"})
			return
		}
		if seenPC[s.ComputerID] {
			c.JSON(http.StatusBadRequest, gin.H{"code": "dup_computer", "message": "Один ПК указан дважды"})
			return
		}
		seenPC[s.ComputerID] = true
	}
	actor := c.GetString("user_id")
	results := make([]gin.H, 0, len(req.Seats))
	seated := 0
	for i, s := range req.Seats {
		row := gin.H{"index": i, "computer_id": s.ComputerID}
		var pc models.Computer
		if err := db.First(&pc, "id = ?", s.ComputerID).Error; err != nil {
			row["ok"], row["code"], row["message"] = false, "computer_not_found", "ПК не найден"
			results = append(results, row)
			continue
		}
		row["computer"] = pc.Name
		// гостя резолвим без записи в ответ — коды кладём в строку места
		user, code, msg := resolveSeatGuest(s.GuestID, s.Guest, s.Nickname)
		if user == nil {
			row["ok"], row["code"], row["message"] = false, code, msg
			results = append(results, row)
			continue
		}
		row["nickname"], row["user_id"] = user.Nickname, user.ID
		if seenGuest[user.ID.String()] {
			row["ok"], row["code"], row["message"] = false, "dup_guest", "Гость указан дважды"
			results = append(results, row)
			continue
		}
		seenGuest[user.ID.String()] = true
		if ok, code := canTargetUser(user.Role, user.ID.String(), actor); !ok {
			row["ok"], row["code"], row["message"] = false, code, "Посадить можно только гостя"
			results = append(results, row)
			continue
		}
		if user.Status == models.UserStatusBanned {
			row["ok"], row["code"], row["message"] = false, "banned", "Аккаунт заблокирован — сначала разбань"
			results = append(results, row)
			continue
		}
		cid := pc.ID.String()
		status, resp := startSessionFor(user.ID, &cid, req.PlannedMin, false) // как одиночная посадка: окно [Готов!], WoL (Ж4)
		if status >= 300 {
			row["ok"] = false
			row["code"], row["message"] = resp["code"], resp["message"]
			results = append(results, row)
			continue
		}
		seated++
		row["ok"] = true
		row["session_id"] = resp["session_id"]
		row["shell_wait_until"], row["wol"], row["shell_note"] = resp["shell_wait_until"], resp["wol"], resp["shell_note"]
		row["effective_rate_pln"] = resp["effective_rate_pln"]
		target := user.ID
		details := user.Nickname + " за " + pc.Name + " · компания"
		if _, waiting := resp["shell_wait_until"]; waiting {
			details += " · ПК не на связи, WoL: " + fmt.Sprint(resp["wol"])
		}
		logAdminAction(c, "session_start", &target, details)
		results = append(results, row)
	}
	if seated > 0 {
		hub.AdminBroadcast("seat_group", map[string]any{"seated": seated, "total": len(req.Seats)})
		logAdminAction(c, "seat_group", nil, fmt.Sprintf("компания: посажено %d из %d", seated, len(req.Seats)))
	}
	c.JSON(http.StatusOK, gin.H{"seated": seated, "total": len(req.Seats), "results": results})
}

// resolveSeatGuest — резолвер гостя без записи HTTP-ответа (для пакетной
// посадки): те же три ключа, что у lookupGuestForAction.
func resolveSeatGuest(guestID, guest, nickname string) (*models.User, string, string) {
	if id := strings.TrimSpace(guestID); id != "" {
		var user models.User
		if err := db.First(&user, "id = ? AND role = ?", id, models.UserRolePlayer).Error; err != nil {
			return nil, "user_not_found", "Гость не найден"
		}
		return &user, "", ""
	}
	if strings.TrimSpace(guest) == "" {
		if strings.TrimSpace(nickname) == "" {
			return nil, "guest_empty", "Нужен ник, телефон или имя гостя"
		}
		var user models.User
		if err := db.First(&user, "nickname = ? AND role = ?", nickname, models.UserRolePlayer).Error; err != nil {
			return nil, "user_not_found", "Гость с таким ником не найден"
		}
		return &user, "", ""
	}
	user, code, cands := resolveGuest(guest)
	switch code {
	case "":
		return user, "", ""
	case "guest_ambiguous":
		names := make([]string, 0, len(cands))
		for _, u := range cands {
			names = append(names, u.Nickname)
		}
		return nil, code, "Подходит несколько: " + strings.Join(names, ", ") + " — выбери в форме"
	case "guest_empty":
		return nil, code, "Нужен ник, телефон или имя гостя"
	}
	return nil, "user_not_found", "Гость не найден: ни по нику, ни по телефону, ни по имени"
}
