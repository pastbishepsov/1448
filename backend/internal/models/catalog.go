package models

import "time"

// CatalogApp — приложение гостевого экрана (миграция 011).
// target/args выполняет shell-agent строго по этому allowlist.
//
// Обложка (миграция 054): либо ссылка CoverURL, либо файл, загруженный в БД
// (cover BYTEA — в модель НЕ входит, читается/пишется raw-SQL в catalog.go,
// как фото позиции кухни). Что показывать витрине — считает catalogCoverURL
// и кладёт в поле Cover.
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
	CoverURL  *string   `json:"cover_url,omitempty" gorm:"type:text"`
	Sort      int       `json:"sort"      gorm:"default:100"`
	Enabled   bool      `json:"enabled"   gorm:"default:true"`
	UpdatedAt time.Time `json:"updated_at"`

	// Загруженная обложка: сами байты в модели не держим (могут быть сотни КБ
	// на запись, а каталог читается целиком на каждый экран).
	CoverType *string    `json:"-" gorm:"size:32"`
	CoverAt   *time.Time `json:"-"`

	// Вычисляемые поля ответа API, в БД не хранятся:
	//   Cover     — итоговый URL картинки для витрины (загрузка > ссылка);
	//   TargetRaw — исходный target с {GAMES}, когда сервер уже подставил
	//               корень игр (нужен агенту, если он переопределит путь).
	Cover     string `json:"cover,omitempty"      gorm:"-"`
	TargetRaw string `json:"target_raw,omitempty" gorm:"-"`
}

func (CatalogApp) TableName() string { return "catalog_apps" }
