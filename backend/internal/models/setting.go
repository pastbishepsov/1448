package models

import "time"

// Setting — настройка клуба (миграция 017): key-value. Движок читает
// значения через settingInt64 (cmd/server/settings.go) с дефолтом из кода.
type Setting struct {
	Key       string    `json:"key"   gorm:"primaryKey;size:64"`
	Value     string    `json:"value" gorm:"not null"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Setting) TableName() string { return "settings" }
