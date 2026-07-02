package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusCompleted BookingStatus = "completed"
	BookingStatusNoShow    BookingStatus = "no_show"
)

// Booking — бронь ПК (миграция 007).
type Booking struct {
	ID         uuid.UUID     `json:"id"          gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID     uuid.UUID     `json:"user_id"     gorm:"type:uuid;not null;index"`
	ComputerID uuid.UUID     `json:"computer_id" gorm:"type:uuid;not null"`
	ClubID     uuid.UUID     `json:"club_id"     gorm:"type:uuid;not null"`
	Status     BookingStatus `json:"status"      gorm:"type:booking_status;default:pending"`

	StartTime   time.Time `json:"start_time"   gorm:"not null"`
	DurationMin int       `json:"duration_min" gorm:"not null;default:60"`
	Prepaid     bool      `json:"prepaid"      gorm:"not null;default:true"`
	Notes       *string   `json:"notes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Computer *Computer `json:"computer,omitempty" gorm:"foreignKey:ComputerID"`
	Club     *Club     `json:"club,omitempty"     gorm:"foreignKey:ClubID"`
}

func (b *Booking) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// EndTime — конец брони.
func (b *Booking) EndTime() time.Time {
	return b.StartTime.Add(time.Duration(b.DurationMin) * time.Minute)
}
