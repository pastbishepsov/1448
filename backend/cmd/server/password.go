package main

// Е0-и2 (OPERATOR.md, этап II.2) — сброс пароля гостя у стойки и обязательная
// смена временного пароля.
//
// Сценарий клуба: гость забыл пароль. Админ жмёт «Сбросить», диктует
// временный, гость входит и на первом же экране задаёт свой. До замены
// аккаунт наполовину чужой — временный пароль знает и админ, — поэтому играть
// с ним гость сам не начнёт (409 на старте сессии). Посадку админом это не
// блокирует: у стойки человека опознали лично.
//
// Пароль не попадает ни в аудит, ни в live-ленту, ни в лог — он существует
// ровно в одном ответе и дальше живёт только в голове у гостя.

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// tempPassAlphabet — без «похожих» символов: 0/O, 1/l/I, 5/S. Пароль диктуют
// вслух через стойку, и «ноль или буква о» — это второй заход и очередь.
const tempPassAlphabet = "abcdefghjkmnpqrtuvwxyz2346789"

const (
	tempPassGroups   = 3 // групп по 4 символа: «hkr7-mp3q-xv2t» — читается голосом
	tempPassGroupLen = 4
	minPasswordLen   = 6 // как в registerRequest (binding min=6)
	maxPasswordLen   = 72
)

// generateTempPassword — временный пароль из crypto/rand, группами по 4.
// Энтропия: 12 символов алфавита из 29 ≈ 58 бит — для пароля, который живёт
// минуты, с запасом.
func generateTempPassword() (string, error) {
	groups := make([]string, 0, tempPassGroups)
	for g := 0; g < tempPassGroups; g++ {
		var b strings.Builder
		for i := 0; i < tempPassGroupLen; i++ {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tempPassAlphabet))))
			if err != nil {
				return "", err
			}
			b.WriteByte(tempPassAlphabet[n.Int64()])
		}
		groups = append(groups, b.String())
	}
	return strings.Join(groups, "-"), nil
}

// validatePasswordLen — те же границы, что у регистрации: bcrypt режет вход
// на 72 байтах, поэтому длиннее не принимаем молча (чистая, тест).
func validatePasswordLen(p string) (code string, ok bool) {
	switch {
	case len(p) < minPasswordLen:
		return "password_short", false
	case len(p) > maxPasswordLen:
		return "password_long", false
	}
	return "", true
}

// passwordCutoff — момент отсечки токенов (чистая, тест).
//
// История двух заходов, оба поймал живой e2e. Сначала округляли ВНИЗ до
// секунды — refresh, выданный в ту же секунду, что и сброс, отсечку
// переживал. Тогда стали округлять ВВЕРХ — и убили противоположное: гость,
// вошедший временным паролем в ту же секунду, получал refresh, мёртвый с
// рождения («войди заново» сразу после входа).
//
// Причина обоих — гранулярность: стандартный iat меряет секунды, а сброс и
// вход умещаются в одну. Поэтому токен теперь носит миллисекунды выдачи
// (claim iat_ms), и отсечка берётся БЕЗ округления: всё, выданное раньше
// неё, мертво; всё, что позже, живёт. Округлять больше нечего.
func passwordCutoff(now time.Time) time.Time {
	return now
}

// setPassword — общий путь смены пароля: новый хеш, отсечка старых токенов и
// снятие/установка флага обязательной смены. Всё одним апдейтом, чтобы не
// оставалось состояния «пароль сменили, а токены ещё живы». Возвращает
// момент отсечки — им подписывается свежая пара.
func setPassword(userID uuid.UUID, plain string, mustChange bool) (time.Time, error) {
	cutoff := passwordCutoff(time.Now())
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return cutoff, err
	}
	return cutoff, db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":        string(hash),
		"must_change_password": mustChange,
		"tokens_valid_from":    cutoff,
	}).Error
}

// tokenIssuedBefore — выдан ли токен раньше отсечки (чистая, тест).
//
// Есть миллисекунды — сравниваем точно; равенство считаем «не раньше», на нём
// держится пара, подписанная моментом отсечки. Токены, выданные до Е0-и5в,
// миллисекунд не носят: для них сравниваем по секундам и решаем в пользу
// БЕЗОПАСНОСТИ (секунда сброса считается «раньше») — такой токен старый по
// определению, и лишний перелогин здесь дешевле пережившего сброс доступа.
func tokenIssuedBefore(iat, iatMs int64, validFrom *time.Time) bool {
	if validFrom == nil {
		return false
	}
	if iatMs > 0 {
		return iatMs < validFrom.UnixMilli()
	}
	if iat == 0 {
		return false // ни iat, ни iat_ms — судить не по чему
	}
	return iat <= validFrom.Unix()
}

// POST /admin/users/:id/password/reset — выдать гостю временный пароль (Е0-и2).
// Отдаётся ОДИН раз в этом ответе: в базе только хеш, в аудите — факт.
func handleAdminPasswordReset(c *gin.Context) {
	user := targetPlayer(c) // 404 + 403 «не персонал и не себе» (Б0)
	if user == nil {
		return
	}
	temp, err := generateTempPassword()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "rand_error", "message": "Не удалось создать пароль"})
		return
	}
	if _, err := setPassword(user.ID, temp, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "password_reset", &target, "выдан временный пароль") // без значения
	notifyUser(user.ID, "password_reset", map[string]any{"by": "admin"})

	c.JSON(http.StatusOK, gin.H{
		"user_id":       user.ID,
		"nickname":      user.Nickname,
		"temp_password": temp,
		"hint":          "Продиктуй гостю: при входе система попросит задать свой пароль",
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password"     binding:"required"`
}

// POST /me/password — гость меняет свой пароль (Е0-и2).
//
// Текущий пароль спрашиваем всегда, в том числе сразу после сброса: гость его
// только что ввёл при входе, зато чужой человек за незапертым экраном сменить
// пароль не сможет. В ответ — свежая пара токенов, потому что своей же
// отсечкой мы только что убили старую.
func handleChangeMyPassword(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны current_password и new_password"})
		return
	}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_password", "message": "Текущий пароль не подходит"})
		return
	}
	if code, ok := validatePasswordLen(req.NewPassword); !ok {
		msg := map[string]string{
			"password_short": "Пароль: минимум 6 символов",
			"password_long":  "Пароль: максимум 72 символа",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}
	if req.NewPassword == req.CurrentPassword {
		c.JSON(http.StatusBadRequest, gin.H{"code": "password_same", "message": "Новый пароль совпадает со старым"})
		return
	}
	cutoff, err := setPassword(user.ID, req.NewPassword, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	// Перечитать: в ответ уходит пользователь уже без флага смены.
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}
	// Подписываем моментом отсечки: пара переживает её по построению.
	writeAuthAt(c, http.StatusOK, &user, cutoff)
}
