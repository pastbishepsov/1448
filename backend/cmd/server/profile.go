package main

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxAvatarID = 12 // число доступных аватаров (тюнинг-параметр)

// validateProfilePatch — чистая проверка полей PATCH /me (тестируется отдельно).
func validateProfilePatch(nickname *string, avatarID *int) (ok bool, code string) {
	if nickname == nil && avatarID == nil {
		return false, "empty"
	}
	if nickname != nil {
		n := utf8.RuneCountInString(strings.TrimSpace(*nickname))
		if n < 3 {
			return false, "nickname_short"
		}
		if n > 32 {
			return false, "nickname_long"
		}
	}
	if avatarID != nil {
		if *avatarID < 1 || *avatarID > maxAvatarID {
			return false, "avatar_invalid"
		}
	}
	return true, ""
}

type patchMeRequest struct {
	Nickname *string `json:"nickname"`
	AvatarID *int    `json:"avatar_id"`
}

// PATCH /me — смена ника и/или аватара.
func handlePatchMe(c *gin.Context) {
	userID := c.GetString("user_id")

	var req patchMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	if ok, code := validateProfilePatch(req.Nickname, req.AvatarID); !ok {
		msg := map[string]string{
			"empty":          "Нечего обновлять",
			"nickname_short": "Никнейм слишком короткий (мин. 3)",
			"nickname_long":  "Никнейм слишком длинный (макс. 32)",
			"avatar_invalid": "Недопустимый аватар",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	updates := map[string]any{}
	if req.Nickname != nil {
		updates["nickname"] = strings.TrimSpace(*req.Nickname)
	}
	if req.AvatarID != nil {
		updates["avatar_id"] = *req.AvatarID
	}

	if err := db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "nickname_taken", "message": "Никнейм уже занят"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, user)
}
