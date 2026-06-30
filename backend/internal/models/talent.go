package models

import (
	"time"

	"github.com/google/uuid"
)

type TalentBranch string

const (
	TalentBranchStrength  TalentBranch = "strength"
	TalentBranchAgility   TalentBranch = "agility"
	TalentBranchIntellect TalentBranch = "intellect"
)

// TalentDefinition — конфигурация таланта (таблица talent_definitions, сидится миграцией 006).
type TalentDefinition struct {
	ID             string       `json:"id"               gorm:"primaryKey;size:64"`
	Branch         TalentBranch `json:"branch"           gorm:"type:talent_branch;not null"`
	Name           string       `json:"name"             gorm:"size:128;not null"`
	Description    string       `json:"description"      gorm:"not null"`
	MaxLevel       int          `json:"max_level"        gorm:"not null;default:5"`
	EffectType     string       `json:"effect_type"      gorm:"size:64;not null"`
	EffectPerLevel float64      `json:"effect_per_level" gorm:"type:decimal(8,4);not null"`
	MinUserLevel   int          `json:"min_user_level"   gorm:"not null;default:1"`
	IsActive       bool         `json:"is_active"        gorm:"not null;default:true"`
}

func (TalentDefinition) TableName() string { return "talent_definitions" }

// SkillTalent — вложенные игроком уровни таланта (таблица skill_talents).
type SkillTalent struct {
	ID           uuid.UUID    `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID    `json:"user_id"       gorm:"type:uuid;not null;index"`
	Branch       TalentBranch `json:"branch"        gorm:"type:talent_branch;not null"`
	TalentID     string       `json:"talent_id"     gorm:"size:64;not null"`
	CurrentLevel int          `json:"current_level" gorm:"not null;default:0"`
	MaxLevel     int          `json:"max_level"     gorm:"not null"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

func (SkillTalent) TableName() string { return "skill_talents" }
