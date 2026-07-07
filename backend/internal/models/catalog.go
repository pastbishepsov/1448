package models

import "time"

// CatalogApp — приложение гостевого экрана (миграция 011).
// target/args выполняет shell-agent строго по этому allowlist.
type CatalogApp struct {
	ID        string    `json:"id"        gorm:"primaryKey;size:32"`
	Name      string    `json:"name"      gorm:"size:64;not null"`
	Subtitle  *string   `json:"subtitle,omitempty" gorm:"size:64"`
	Category  string    `json:"category"  gorm:"size:16;not null"` // game | app | system | platform
	Tag       *string   `json:"tag,omitempty"   gorm:"size:12"`
	Emoji     *string   `json:"emoji,omitempty" gorm:"size:8"`
	ColorA    *string   `json:"color_a,omitempty" gorm:"size:9"`
	ColorB    *string   `json:"color_b,omitempty" gorm:"size:9"`
	Target    *string   `json:"target,omitempty"`
	Args      *string   `json:"args,omitempty" gorm:"type:jsonb"` // JSON-массив строк
	Sort      int       `json:"sort"      gorm:"default:100"`
	Enabled   bool      `json:"enabled"   gorm:"default:true"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CatalogApp) TableName() string { return "catalog_apps" }
