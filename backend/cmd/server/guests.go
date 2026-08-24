package main

// Е0 «Гость под рукой» (OPERATOR.md, этап II) — правка данных гостя у стойки.
//
// Лист основателя: «Смена пароля или любых данных». До этого спринта админ
// умел с гостем ровно четыре вещи — банить, пополнять, начислять и гасить
// монеты; опечатка в нике и забытый пароль лечились SQL.
//
// Роль: admin, а не owner. Это операционка у стойки (гость стоит и ждёт), и
// прятать её за владельцем значит звонить ему из-за каждой опечатки. Защита —
// не роль, а журнал: каждая правка попадает в аудит. Если владелец захочет
// строже, phone/email выносятся в own-группу одной строкой в main.go.
//
// GDPR-разрез (OPERATOR.md II.4, образец — кадровая карточка В3-2): в аудит
// уходит ФАКТ правки и СПИСОК полей, значения — никогда. Гостю летит
// уведомление шиной Б4: свои данные менял не он, он должен об этом узнать.

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// validateEmail — e-mail для правки у стойки: строгий разбор stdlib плюс
// инвариант nicknameSafe (значения едут в разметку админки — ровно тот
// вектор, что закрывали в QA Б9–Б11). Форма «Имя <a@b.c>» не принимается:
// в колонке должен лежать чистый адрес (чистая, тест).
func validateEmail(s string) (string, bool) {
	e := strings.TrimSpace(s)
	if e == "" || len(e) > 255 || !nicknameSafe(e) || strings.ContainsAny(e, " \t") {
		return "", false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Name != "" || a.Address != e {
		return "", false
	}
	return e, true
}

type adminGuestPatch struct {
	Nickname  *string `json:"nickname"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
}

// PATCH /admin/users/:id — правка данных гостя админом (Е0-и1).
//
// Пустая строка в необязательном поле ЧИСТИТ его (GDPR, как в PATCH /me
// анкеты Г8); ник пустым быть не может — это идентификатор входа.
func handleAdminGuestUpdate(c *gin.Context) {
	user := targetPlayer(c) // 404 «нет такого» + 403 «не персонал и не себе» (Б0)
	if user == nil {
		return
	}
	var req adminGuestPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Не разобрал запрос"})
		return
	}

	updates := map[string]any{}
	var fields []string // человеческие имена полей — для аудита, БЕЗ значений

	if req.Nickname != nil {
		if ok, code := validateProfilePatch(req.Nickname, nil); !ok {
			msg := map[string]string{
				"nickname_short":   "Никнейм: минимум 3 символа",
				"nickname_long":    "Никнейм: максимум 32 символа",
				"nickname_charset": "Никнейм: без кавычек, угловых скобок и слэшей",
			}[code]
			c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
			return
		}
		updates["nickname"] = strings.TrimSpace(*req.Nickname)
		fields = append(fields, "ник")
	}

	for _, f := range []struct {
		val   *string
		col   string
		label string
		human string
	}{
		{req.FirstName, "first_name", "имя", "Имя"},
		{req.LastName, "last_name", "фамилия", "Фамилия"},
	} {
		if f.val == nil {
			continue
		}
		if strings.TrimSpace(*f.val) == "" {
			updates[f.col] = nil // GDPR: пустое значение стирает поле
		} else {
			n, ok := validateName(*f.val)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_" + f.col,
					"message": f.human + ": до 64 букв, пробел, дефис и апостроф"})
				return
			}
			updates[f.col] = n
		}
		fields = append(fields, f.label)
	}

	if req.Phone != nil {
		if strings.TrimSpace(*req.Phone) == "" {
			updates["phone"] = nil
		} else {
			p, ok := validatePhone(*req.Phone)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_phone",
					"message": "Телефон в формате +48123456789"})
				return
			}
			updates["phone"] = p
		}
		fields = append(fields, "телефон")
	}

	if req.Email != nil {
		if strings.TrimSpace(*req.Email) == "" {
			updates["email"] = nil
		} else {
			e, ok := validateEmail(*req.Email)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_email", "message": "E-mail разобрать не смог"})
				return
			}
			updates["email"] = e
		}
		fields = append(fields, "e-mail")
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "empty", "message": "Нечего менять"})
		return
	}

	if err := db.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		if isDuplicate(err) {
			code, msg := guestTakenReason(updates)
			c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	target := user.ID
	logAdminAction(c, "guest_update", &target, "поля: "+strings.Join(fields, ", "))
	// Гостю — тост: свои данные менял не он (строки клиента — Е0-и5, до них
	// шелл покажет общий текст ветки default в notifText).
	notifyUser(user.ID, "profile_updated", map[string]any{"fields": fields})

	var fresh models.User
	if err := db.First(&fresh, "id = ?", user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": fresh, "changed": fields})
}

// guestTakenReason — какое из уникальных полей не пустило правку. Точный
// ответ возможен, только когда в патче ровно одно такое поле; иначе честно
// говорим «одно из» вместо угадывания (чистая, тест).
func guestTakenReason(updates map[string]any) (code, message string) {
	type uniq struct{ col, code, label string }
	var hit []uniq
	for _, u := range []uniq{
		{"nickname", "nickname_taken", "никнейм"},
		{"phone", "phone_taken", "телефон"},
		{"email", "email_taken", "e-mail"},
	} {
		if v, ok := updates[u.col]; ok && v != nil {
			hit = append(hit, u)
		}
	}
	switch len(hit) {
	case 0:
		return "taken", "Значение уже занято"
	case 1:
		return hit[0].code, "Этот " + hit[0].label + " уже занят другим аккаунтом"
	default:
		labels := make([]string, 0, len(hit))
		for _, u := range hit {
			labels = append(labels, u.label)
		}
		return "taken", "Уже занято одно из: " + strings.Join(labels, ", ")
	}
}
