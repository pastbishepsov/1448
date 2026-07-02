package models

import (
	"time"

	"github.com/google/uuid"
)

// RevokedToken — отозванный refresh-токен (миграция 010).
type RevokedToken struct {
	JTI       uuid.UUID `json:"jti"        gorm:"type:uuid;primaryKey;column:jti"`
	UserID    uuid.UUID `json:"user_id"    gorm:"type:uuid;not null"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	RevokedAt time.Time `json:"revoked_at" gorm:"autoCreateTime"`
}

func (RevokedToken) TableName() string { return "revoked_tokens" }
