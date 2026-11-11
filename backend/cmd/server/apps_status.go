package main

// З20-и1 (OWNER.md, трек З; №57 «Где не установлено»): агент ПК шлёт по WS снимок доступности приложений каталога —
// тип apps_status, payload как у его GET /apps-status: {"apps": {"cs2": {"available": true}, "gta5": {"available":
// false, "reason": "path_unreachable"}}}; остальное (running, kind, checked_at) серверу не нужно. Сервер хранит
// последний снимок по каждому ПК (pc_app_status, миграция 096): новый заменяет прежний целиком, приложения не из
// каталога пропускаются. Битый снимок — строка в лог, прежний остаётся. Показывать — З20-и3 (ниже): каталог — где
// приложение не запустится, шторка ПК — что не запустится на нём.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	appsStatusMaxApps   = 1000 // больше в одном снимке — не каталог клуба, а мусор
	appsStatusMaxID     = 32   // как id записи каталога
	appsStatusMaxReason = 32
)

// appState — доступность одного приложения в снимке агента.
type appState struct {
	Available bool
	Reason    string
}

// parseAppsStatus — снимок из payload apps_status. Ошибка — снимок целиком не годится (нет apps, нет available,
// пустой или длинный id, слишком много приложений): лучше оставить прежний, чем записать половину.
func parseAppsStatus(raw json.RawMessage) (map[string]appState, error) {
	var p struct {
		Apps map[string]*struct {
			Available *bool  `json:"available"`
			Reason    string `json:"reason"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("не JSON: %v", err)
	}
	if p.Apps == nil {
		return nil, errors.New("нет apps")
	}
	if len(p.Apps) > appsStatusMaxApps {
		return nil, fmt.Errorf("приложений %d — больше %d", len(p.Apps), appsStatusMaxApps)
	}
	out := make(map[string]appState, len(p.Apps))
	for id, st := range p.Apps {
		if id == "" || strings.TrimSpace(id) != id || utf8.RuneCountInString(id) > appsStatusMaxID {
			return nil, fmt.Errorf("плохой id приложения %q", id)
		}
		if st == nil || st.Available == nil {
			return nil, fmt.Errorf("у %q нет available", id)
		}
		reason := strings.TrimSpace(st.Reason)
		if r := []rune(reason); len(r) > appsStatusMaxReason {
			reason = string(r[:appsStatusMaxReason])
		}
		out[id] = appState{Available: *st.Available, Reason: reason}
	}
	return out, nil
}

// saveAppsStatus — снимок ПК в pc_app_status одной транзакцией: строки приложений, которых в снимке нет, удаляются,
// остальные — вставка или обновление. Приложения не из каталога пропускаются (skipped): агент мог прислать запись,
// которую уже удалили. Возвращает, сколько строк записано и сколько пропущено.
func saveAppsStatus(gdb *gorm.DB, computerID uuid.UUID, snap map[string]appState, at time.Time) (kept, skipped int, err error) {
	ids := make([]string, 0, len(snap))
	for id := range snap {
		ids = append(ids, id)
	}
	known := []string{}
	if len(ids) > 0 {
		if err := gdb.Model(&models.CatalogApp{}).Where("id IN ?", ids).Pluck("id", &known).Error; err != nil {
			return 0, 0, err
		}
	}
	sort.Strings(known)
	rows := make([]models.PCAppStatus, 0, len(known))
	for _, id := range known {
		st := snap[id]
		rows = append(rows, models.PCAppStatus{ComputerID: computerID, AppID: id, Available: st.Available, Reason: st.Reason,
			ReportedAt: at})
	}
	err = gdb.Transaction(func(tx *gorm.DB) error {
		del := tx.Where("computer_id = ?", computerID)
		if len(known) > 0 {
			del = del.Where("app_id NOT IN ?", known)
		}
		if err := del.Delete(&models.PCAppStatus{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "computer_id"}, {Name: "app_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"available", "reason", "reported_at"}),
		}).CreateInBatches(rows, 200).Error
	})
	if err != nil {
		return 0, 0, err
	}
	return len(rows), len(ids) - len(known), nil
}

// appsStatusFromShell — apps_status от агента (hub.OnAppsStatus, З20-и1). Хук хаба, вызывается из горутины чтения WS
// этого ПК: цикл хаба не ждёт базу.
func appsStatusFromShell(computerID string, payload json.RawMessage) {
	pc, err := uuid.Parse(computerID)
	if err != nil {
		log.Printf("apps_status: computer_id %q — не uuid, снимок не записан", computerID)
		return
	}
	snap, err := parseAppsStatus(payload)
	if err != nil {
		log.Printf("apps_status от computer=%s отклонён: %v — остаётся прежний снимок", computerID, err)
		return
	}
	var n int64
	if err := db.Model(&models.Computer{}).Where("id = ?", pc).Count(&n).Error; err != nil || n == 0 {
		log.Printf("apps_status: ПК %s нет в базе, снимок не записан", computerID)
		return
	}
	if _, skipped, err := saveAppsStatus(db, pc, snap, time.Now()); err != nil {
		log.Printf("apps_status от computer=%s не записан: %v", computerID, err)
	} else if skipped > 0 {
		log.Printf("apps_status от computer=%s: не из каталога — %d, пропущены", computerID, skipped)
	}
}

// ── З20-и3 (№57): «где не установлено» — каталогу и шторке ПК, по последним снимкам агентов ──────────────────────────

// appInstall — где приложение каталога не запустится. Reported — сколько ПК прислали проверенный ответ («checking» —
// агент ещё не проверял — не в счёт); Missing — ПК, где файла нет (target_missing, no_target); Unreachable — где путь
// недостижим (path_unreachable: шара или том не отвечают). ПК — по имени, по порядку.
type appInstall struct {
	Reported    int      `json:"reported"`
	Missing     []string `json:"missing"`
	Unreachable []string `json:"unreachable"`
}

// appsInstallByApp — appInstall по id записи каталога; записей без снимков в карте нет.
func appsInstallByApp(gdb *gorm.DB) (map[string]*appInstall, error) {
	var rows []struct {
		AppID     string
		Name      string
		Available bool
		Reason    string
	}
	if err := gdb.Raw(`SELECT s.app_id, c.name, s.available, s.reason FROM pc_app_status s
		JOIN computers c ON c.id = s.computer_id ORDER BY c.name`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]*appInstall{}
	for _, r := range rows {
		if r.Available && r.Reason == "checking" {
			continue
		}
		in := out[r.AppID]
		if in == nil {
			in = &appInstall{Missing: []string{}, Unreachable: []string{}}
			out[r.AppID] = in
		}
		in.Reported++
		switch {
		case r.Available:
		case r.Reason == "path_unreachable":
			in.Unreachable = append(in.Unreachable, r.Name)
		default:
			in.Missing = append(in.Missing, r.Name)
		}
	}
	return out, nil
}

// appOff — приложение, которое на этом ПК не запустится (шторка ПК).
type appOff struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// appsOffByPC — по ПК: что у него не запустится и когда пришёл его последний снимок (apps_at). ПК без снимков в картах
// нет.
func appsOffByPC(gdb *gorm.DB) (off map[string][]appOff, at map[string]time.Time, err error) {
	var rows []struct {
		ComputerID string
		AppID      string
		Name       string
		Available  bool
		Reason     string
		ReportedAt time.Time
	}
	if err := gdb.Raw(`SELECT s.computer_id::text AS computer_id, s.app_id, a.name, s.available, s.reason, s.reported_at
		FROM pc_app_status s JOIN catalog_apps a ON a.id = s.app_id ORDER BY a.name, s.app_id`).Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	off, at = map[string][]appOff{}, map[string]time.Time{}
	for _, r := range rows {
		if r.ReportedAt.After(at[r.ComputerID]) {
			at[r.ComputerID] = r.ReportedAt
		}
		if !r.Available {
			off[r.ComputerID] = append(off[r.ComputerID], appOff{ID: r.AppID, Name: r.Name, Reason: r.Reason})
		}
	}
	return off, at, nil
}

// catalogInstall — appsInstallByApp для GET /admin/catalog; база не ответила — строка в лог и пустая карта.
func catalogInstall() map[string]*appInstall {
	m, err := appsInstallByApp(db)
	if err != nil {
		log.Printf("каталог: снимки агентов не прочитались: %v", err)
		return map[string]*appInstall{}
	}
	return m
}

// pcAppsOff — appsOffByPC для GET /admin/computers; база не ответила — строка в лог и пустые карты.
func pcAppsOff() (map[string][]appOff, map[string]time.Time) {
	off, at, err := appsOffByPC(db)
	if err != nil {
		log.Printf("ПК: снимки агентов не прочитались: %v", err)
		return map[string][]appOff{}, map[string]time.Time{}
	}
	return off, at
}
