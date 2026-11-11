package main

// З20-и1 (OWNER.md, трек З; №57 «Где не установлено»): агент ПК шлёт по WS снимок доступности приложений каталога —
// тип apps_status, payload как у его GET /apps-status: {"apps": {"cs2": {"available": true}, "gta5": {"available":
// false, "reason": "path_unreachable"}}}; остальное (running, kind, checked_at) серверу не нужно. Сервер хранит
// последний снимок по каждому ПК (pc_app_status, миграция 096): новый заменяет прежний целиком, приложения не из
// каталога пропускаются. Битый снимок — строка в лог, прежний остаётся. Показывать — З20-и3.

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
