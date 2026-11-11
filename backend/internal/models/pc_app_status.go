package models

import (
	"time"

	"github.com/google/uuid"
)

// PCAppStatus — последний снимок доступности приложения каталога на ПК (миграция 096, З20-и1): агент шлёт apps_status
// по WS, сервер хранит строку на ПК × приложение. Reason — почему недоступно (target_missing, path_unreachable,
// no_target) или checking — агент ещё не проверял (available тогда оптимистично true).
type PCAppStatus struct {
	ComputerID uuid.UUID `json:"computer_id" gorm:"type:uuid;primaryKey"`
	AppID      string    `json:"app_id"      gorm:"size:32;primaryKey"`
	Available  bool      `json:"available"   gorm:"not null"`
	Reason     string    `json:"reason"      gorm:"size:32;not null"`
	ReportedAt time.Time `json:"reported_at" gorm:"not null"`
}

func (PCAppStatus) TableName() string { return "pc_app_status" }
