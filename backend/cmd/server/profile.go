package main

// Профиль гостя. Спринт Г8 добавил анкету (Р4/Р9 GUEST.md): дата рождения,
// любимые игры (до 3, из каталога), Discord/Telegram, «откуда узнал». Всё
// опционально и стираемо (GDPR, пустое значение очищает поле), значения в
// аудит не пишутся. Награды — lifetime-ачивки сида 040 (+25 XP за поле,
// «Анкета закрыта» — Light + 1 sp): выдаются один раз навсегда, стирание и
// повторное заполнение ничего не дублирует (have-map user_achievements).
// Контракт PATCH /me для страницы регистрации основателя — docs/api.md.

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxAvatarID    = 12  // число доступных аватаров (тюнинг-параметр)
	maxFavGames    = 3   // любимых игр в профиле
	minProfileAge  = 6   // возраст по дате рождения — рамки здравого смысла
	maxProfileAge  = 100
	maxHandleRunes = 64 // discord/telegram
	maxSourceRunes = 32 // «откуда узнал»
)

// validateProfilePatch — чистая проверка полей PATCH /me (тестируется отдельно).
// nicknameSafe — чистая проверка символов ника (тест в profile_test.go).
// Кавычки, угловые скобки, бэктик, слэши и управляющие запрещены: ники
// едут в разметку и inline-обработчики админки — это инвариант сервера,
// UI-экранирования недостаточно (QA-прогон Б9–Б11, 2026-07-22).
func nicknameSafe(s string) bool {
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(`'"<>&`+"`\\", r) {
			return false
		}
	}
	return true
}

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
		if !nicknameSafe(strings.TrimSpace(*nickname)) {
			return false, "nickname_charset"
		}
	}
	if avatarID != nil {
		if *avatarID < 1 || *avatarID > maxAvatarID {
			return false, "avatar_invalid"
		}
	}
	return true, ""
}

// validateBirthDate — "2006-01-02", возраст 6–100 (чистая, тест).
func validateBirthDate(s string, now time.Time) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	age := now.Year() - t.Year()
	if now.Month() < t.Month() || (now.Month() == t.Month() && now.Day() < t.Day()) {
		age--
	}
	return t, age >= minProfileAge && age <= maxProfileAge
}

// validateHandle — discord/telegram: ведущий @ срезается, 2..64 безопасных
// символов без пробелов (чистая, тест).
func validateHandle(s string) (string, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "@")
	n := utf8.RuneCountInString(h)
	if n < 2 || n > maxHandleRunes || !nicknameSafe(h) || strings.ContainsAny(h, " \t") {
		return "", false
	}
	return h, true
}

// validateFavorites — до 3 включённых игр каталога, без дублей (чистая, тест).
func validateFavorites(list []string, games map[string]bool) ([]string, bool) {
	if len(list) > maxFavGames {
		return nil, false
	}
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, id := range list {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || !games[id] {
			return nil, false
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, true
}

type patchMeRequest struct {
	Nickname      *string   `json:"nickname"`
	AvatarID      *int      `json:"avatar_id"`
	BirthDate     *string   `json:"birth_date"`     // "2006-01-02"; "" = стереть
	Discord       *string   `json:"discord"`        // "" = стереть
	Telegram      *string   `json:"telegram"`       // с @ или без; "" = стереть
	Source        *string   `json:"source"`         // «откуда узнал»; "" = стереть
	FavoriteGames *[]string `json:"favorite_games"` // до 3 id игр каталога; [] = стереть
	Language      *string   `json:"language"`       // Г9: ru|en|pl — пресет, едет за гостем
}

// profileStats — бинарные факты анкеты для условий ачивок Г8 (сид 040).
func profileStats(u *models.User) playerStats {
	s := playerStats{LoginCount: 1}
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	s.ProfileBirth = b(u.BirthDate != nil)
	s.ProfileGames = b(len(u.FavoriteGames) > 0)
	s.ProfileDiscord = b(u.Discord != nil)
	s.ProfileTelegram = b(u.Telegram != nil)
	s.ProfileSource = b(u.Source != nil)
	s.ProfileComplete = b(s.ProfileBirth+s.ProfileGames+s.ProfileDiscord+s.ProfileTelegram+s.ProfileSource == 5)
	return s
}

// PATCH /me — ник, аватар и анкета Г8. Пустая строка (или пустой список)
// очищает поле; награды не дублируются при повторном заполнении.
func handlePatchMe(c *gin.Context) {
	userID := c.GetString("user_id")

	var req patchMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}

	if req.Nickname == nil && req.AvatarID == nil && req.BirthDate == nil &&
		req.Discord == nil && req.Telegram == nil && req.Source == nil &&
		req.FavoriteGames == nil && req.Language == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "empty", "message": "Нечего обновлять"})
		return
	}
	if req.Nickname != nil || req.AvatarID != nil {
		if ok, code := validateProfilePatch(req.Nickname, req.AvatarID); !ok {
			msg := map[string]string{
				"nickname_short": "Никнейм слишком короткий (мин. 3)",
				"nickname_long":  "Никнейм слишком длинный (макс. 32)",
				"avatar_invalid": "Недопустимый аватар",
			}[code]
			c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
			return
		}
	}

	updates := map[string]any{}
	profileTouched := false
	if req.Nickname != nil {
		updates["nickname"] = strings.TrimSpace(*req.Nickname)
	}
	if req.Language != nil { // Г9: язык — пресет, не «данные» (без ачивок)
		l := strings.ToLower(strings.TrimSpace(*req.Language))
		if l != "ru" && l != "en" && l != "pl" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_language", "message": "Язык: ru, en или pl"})
			return
		}
		updates["language"] = l
	}
	if req.AvatarID != nil {
		updates["avatar_id"] = *req.AvatarID
	}
	if req.BirthDate != nil {
		profileTouched = true
		if s := strings.TrimSpace(*req.BirthDate); s == "" {
			updates["birth_date"] = nil
		} else {
			t, ok := validateBirthDate(s, time.Now().In(clubLocation))
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_birth_date",
					"message": "Дата рождения — ГГГГ-ММ-ДД, возраст от 6 до 100"})
				return
			}
			updates["birth_date"] = t
		}
	}
	for _, f := range []struct {
		val *string
		col string
	}{{req.Discord, "discord"}, {req.Telegram, "telegram"}} {
		if f.val == nil {
			continue
		}
		profileTouched = true
		if strings.TrimSpace(*f.val) == "" {
			updates[f.col] = nil
			continue
		}
		h, ok := validateHandle(*f.val)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_" + f.col,
				"message": "Ник в " + map[string]string{"discord": "Discord", "telegram": "Telegram"}[f.col] +
					": 2–64 символа, без пробелов и кавычек"})
			return
		}
		updates[f.col] = h
	}
	if req.Source != nil {
		profileTouched = true
		s := strings.TrimSpace(*req.Source)
		switch {
		case s == "":
			updates["source"] = nil
		case utf8.RuneCountInString(s) > maxSourceRunes || !nicknameSafe(s):
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_source",
				"message": "«Откуда узнал» — до 32 символов без кавычек"})
			return
		default:
			updates["source"] = s
		}
	}
	if req.FavoriteGames != nil {
		profileTouched = true
		games := map[string]bool{}
		var apps []models.CatalogApp
		db.Where("category = ?", "game").Find(&apps)
		for _, a := range apps {
			games[a.ID] = true
		}
		clean, ok := validateFavorites(*req.FavoriteGames, games)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_favorites",
				"message": "Любимые игры: до 3 игр из каталога, без повторов"})
			return
		}
		b, _ := json.Marshal(clean)
		updates["favorite_games"] = string(b)
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
	if profileTouched { // Г8: ачивки за данные — сразу, без ожидания сессии
		if uid, err := uuid.Parse(userID); err == nil {
			checkAchievements(uid, profileStats(&user))
			if req.BirthDate != nil {
				checkBirthdayGift(uid) // вдруг сегодня и есть день рождения
			}
		}
		db.First(&user, "id = ?", userID) // перечитать после наград (xp_total и т.п.)
	}
	c.JSON(http.StatusOK, user)
}

// ── ДР-подарок (Г8-и4): раз в год, по клубному времени, тир — настройка ──

func isLeapYear(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// checkBirthdayGift — выдать подарок, если у гостя сегодня день рождения.
// Вызывается на входе и старте сессии; CAS по birthday_gift_year — раз в год.
// Монеты как вариант подарка отложены до журнала монет (иначе разъедется
// отчёт эмиссии В4-2) — дарим кейс.
func checkBirthdayGift(userID uuid.UUID) {
	tierN := settingInt64("birthday_case_tier", 1)
	if tierN <= 0 {
		return
	}
	var u models.User
	if db.First(&u, "id = ?", userID).Error != nil || u.BirthDate == nil {
		return
	}
	now := time.Now().In(clubLocation)
	m, d := u.BirthDate.Month(), u.BirthDate.Day()
	if m == time.February && d == 29 && !isLeapYear(now.Year()) {
		d = 28 // именинникам 29 февраля дарим 28-го
	}
	if now.Month() != m || now.Day() != d {
		return
	}
	res := db.Model(&models.User{}).
		Where("id = ? AND birthday_gift_year < ?", userID, now.Year()).
		UpdateColumn("birthday_gift_year", now.Year())
	if res.Error != nil || res.RowsAffected == 0 {
		return // уже дарили в этом году (или гонка — подарок один)
	}
	tiers := []models.CaseTier{models.CaseTierLight, models.CaseTierMedium,
		models.CaseTierHeavy, models.CaseTierTitan, models.CaseTierGods}
	if tierN > int64(len(tiers)) {
		tierN = int64(len(tiers))
	}
	tier := tiers[tierN-1]
	if err := grantCase(db, userID, nil, tier, models.CaseSourceBirthday); err == nil {
		notifyUser(userID, "birthday_gift", map[string]any{"case_tier": string(tier)})
		log.Printf("ДР-подарок: гость %s получил кейс %s", userID, tier)
	}
}
