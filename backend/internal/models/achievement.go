package models

import (
	"time"

	"github.com/google/uuid"
)

// Achievement — определение достижения (таблица achievements, сидится миграцией 005).
type Achievement struct {
	ID                string    `json:"id"                 gorm:"primaryKey;size:64"`
	Category          string    `json:"category"           gorm:"type:achievement_category;not null"`
	Title             string    `json:"title"              gorm:"size:128;not null"`
	Description       string    `json:"description"`
	ConditionType     string    `json:"condition_type"     gorm:"size:64;not null"`
	ConditionValue    string    `json:"-"                  gorm:"type:jsonb;not null"` // сырой JSON, напр. {"min":10}
	RewardSkillpoints int       `json:"reward_skillpoints" gorm:"not null;default:0"`
	RewardCaseTier    *CaseTier `json:"reward_case_tier,omitempty" gorm:"type:case_tier"`
	IsActive          bool      `json:"is_active"          gorm:"not null;default:true"`
	CreatedAt         time.Time `json:"created_at"`
}

func (Achievement) TableName() string { return "achievements" }

// UserAchievement — выданное игроку достижение (таблица user_achievements).
type UserAchievement struct {
	ID            uuid.UUID `json:"id"             gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID        uuid.UUID `json:"user_id"        gorm:"type:uuid;not null;index"`
	AchievementID string    `json:"achievement_id" gorm:"size:64;not null"`
	EarnedAt      time.Time `json:"earned_at"`
	PeriodKey     *string   `json:"period_key,omitempty" gorm:"size:16"`
}

func (UserAchievement) TableName() string { return "user_achievements" }
