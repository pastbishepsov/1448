package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StaffMessage — сообщение рабочего канала персонала (миграция 052, Е6, Р7).
//
// Гостевой чат адресный (гость или машина), этот — общий канал клуба: у
// стойки вопрос почти всегда операционный, и ответ владельца полезен всей
// смене. Отдельная сущность, а не флаг в ChatMessage: смешать их значило бы
// однажды показать гостю то, что писали про него.
//
// З1-и2 (миграция 067): служебные сообщения пишет сервер — автора нет, Kind
// system; VisibleTo owner — строку видит и считает только владелец (деньги
// сдачи кассы, дальше — напоминания бухгалтерии и алерты).
type StaffMessage struct {
	ID       uuid.UUID  `json:"id"        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID   uuid.UUID  `json:"club_id"   gorm:"type:uuid;not null"`
	AuthorID *uuid.UUID `json:"author_id" gorm:"type:uuid"`
	// Снимок роли на момент письма: админ может стать владельцем и наоборот, а
	// «кто это писал тогда» из истории пропадать не должно. У служебных — system.
	Role      string    `json:"role"       gorm:"size:8;not null"`
	Kind      string    `json:"kind"       gorm:"size:8;not null;default:text"`
	VisibleTo string    `json:"visible_to" gorm:"size:8;not null;default:all"`
	Text      string    `json:"text"       gorm:"size:1000;not null"`
	CreatedAt time.Time `json:"created_at"`
}

func (m *StaffMessage) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

func (StaffMessage) TableName() string { return "staff_messages" }
