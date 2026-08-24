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
	"gorm.io/gorm"

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

// ── Е0-и3: поиск гостя по нику, телефону и имени ──────────────────────────
//
// До этого спринта админка искала ТОЛЬКО по нику (`nickname ILIKE`). У стойки
// это тупик ровно в тех случаях, ради которых поиск и нужен: гость звонит
// забронировать и называет телефон, а не ник; гость пришёл и говорит «я
// Ковальский»; ник он вообще забыл — за этим и шёл. Телефон и имя лежат в
// базе с миграций 001 и 043 — оставалось только начать по ним искать.

// escapeLike — экранирует спецсимволы LIKE/ILIKE (чистая, тест). Без этого
// поиск «50%» превращается в «покажи всех», а «_» матчит любой символ.
// Экранирующий символ задаём явно через ESCAPE — дефолт в Postgres тот же,
// но полагаться на него молча не стоит.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// phoneDigits — цифры из запроса (чистая, тест). Телефон у стойки называют
// как придётся: «+48 123-456-789», «123 456 789», «...789». В базе он лежит
// в E.164, поэтому сравниваем цифра к цифре. Меньше четырёх цифр не считаем
// телефоном: «77» в нике «Гость-77» иначе тянуло бы за собой пол-клуба.
const minPhoneQueryDigits = 4

func phoneDigits(s string) string {
	d := phoneDigitsRaw(s)
	if len(d) < minPhoneQueryDigits {
		return ""
	}
	return d
}

// guestMatchReason — по какому полю гость нашёлся (чистая, тест). У стойки
// это половина смысла ответа: три Ковальских в списке бесполезны, а «нашёлся
// по телефону» — уже ответ. Порядок проверок = порядок точности.
func guestMatchReason(u *models.User, query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(u.Nickname), q) {
		return "ник"
	}
	if d := phoneDigits(query); d != "" && u.Phone != nil &&
		strings.Contains(phoneDigitsRaw(*u.Phone), d) {
		return "телефон"
	}
	first, last := "", ""
	if u.FirstName != nil {
		first = strings.ToLower(*u.FirstName)
	}
	if u.LastName != nil {
		last = strings.ToLower(*u.LastName)
	}
	switch {
	case first != "" && strings.Contains(first, q):
		return "имя"
	case last != "" && strings.Contains(last, q):
		return "фамилия"
	case strings.Contains(strings.TrimSpace(first+" "+last), q):
		return "имя и фамилия"
	}
	return ""
}

// phoneDigitsRaw — цифры без порога длины (для значения из базы, а не запроса).
func phoneDigitsRaw(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── Е0-и4: один способ опознать гостя для поиска, посадки и брони ──────────
//
// До этого посадка и walk-in бронь требовали ТОЧНЫЙ ник. У стойки это тот же
// тупик, что и в поиске: гость звонит забронировать и называет телефон,
// приходит и называет фамилию. Резолвер делит правила поиска (и3) с
// `/admin/users` — чтобы «нашёлся в поиске» и «сажается по этому же запросу»
// никогда не разъезжались.
//
// Главное правило — НИКОГДА не брать первого попавшегося. Посадить не того
// гостя значит списать деньги с чужого кошелька; на неоднозначность отвечаем
// 409 со списком кандидатов, и выбор делает человек.

// guestSearchCondition — общее условие поиска гостя (и3): ник, телефон
// цифра-к-цифре, имя, фамилия, «имя фамилия».
func guestSearchCondition(s string) *gorm.DB {
	like := "%" + escapeLike(s) + "%"
	cond := db.Where(`nickname ILIKE ? ESCAPE '\'`, like).
		Or(`first_name ILIKE ? ESCAPE '\'`, like).
		Or(`last_name ILIKE ? ESCAPE '\'`, like).
		Or(`TRIM(COALESCE(first_name,'') || ' ' || COALESCE(last_name,'')) ILIKE ? ESCAPE '\'`, like)
	if d := phoneDigits(s); d != "" {
		cond = cond.Or(`regexp_replace(COALESCE(phone,''), '\D', '', 'g') LIKE ?`, "%"+d+"%")
	}
	return cond
}

// maxGuestCandidates — сколько кандидатов показываем при неоднозначности.
// Больше пяти у стойки не читают: это уже не «выбери», а «ищи заново».
const maxGuestCandidates = 5

// resolveGuest — опознать ГОСТЯ (role=player) по нику, телефону или имени.
// Возвращает либо ровно одного, либо код ошибки и список кандидатов.
//
// Точное совпадение бьёт частичное: гость с ником «Ян» садится по запросу
// «Ян», даже когда в клубе есть «Янек» и «Яна». Без этого правила частые
// короткие ники стали бы неразрешимо неоднозначными.
func resolveGuest(query string) (*models.User, string, []models.User) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, "guest_empty", nil
	}
	player := func() *gorm.DB {
		return db.Model(&models.User{}).Where("role = ?", models.UserRolePlayer)
	}

	// 1. Точный ник (регистр не важен: уникальный индекс регистрозависим,
	//    поэтому теоретически «Egor» и «egor» сосуществуют — тогда падаем
	//    в общий разбор ниже, а не выбираем произвольного).
	var exact []models.User
	player().Where("LOWER(nickname) = LOWER(?)", q).Limit(2).Find(&exact)
	if len(exact) == 1 {
		return &exact[0], "", nil
	}

	// 2. Точный телефон — цифра к цифре: «+48 123-456-789» и «123456789»
	//    это один и тот же номер.
	if d := phoneDigits(q); d != "" {
		var byPhone []models.User
		player().Where(`regexp_replace(COALESCE(phone,''), '\D', '', 'g') = ?`, d).
			Limit(2).Find(&byPhone)
		if len(byPhone) == 1 {
			return &byPhone[0], "", nil
		}
	}

	// 3. Частичное совпадение по всем ключам сразу.
	var found []models.User
	player().Where(guestSearchCondition(q)).
		Order("last_active_at DESC").Limit(maxGuestCandidates + 1).Find(&found)
	switch len(found) {
	case 0:
		return nil, "guest_not_found", nil
	case 1:
		return &found[0], "", nil
	default:
		if len(found) > maxGuestCandidates {
			found = found[:maxGuestCandidates]
		}
		return nil, "guest_ambiguous", found
	}
}

// guestCandidates — кандидаты для тела 409: столько, чтобы человек узнал
// своего гостя, и ни поля больше.
func guestCandidates(list []models.User, query string) []gin.H {
	out := make([]gin.H, 0, len(list))
	for i := range list {
		u := &list[i]
		name := strings.TrimSpace(strDeref(u.FirstName) + " " + strDeref(u.LastName))
		item := gin.H{"id": u.ID, "nickname": u.Nickname, "matched_by": guestMatchReason(u, query)}
		if name != "" {
			item["name"] = name
		}
		if u.Phone != nil {
			item["phone"] = *u.Phone
		}
		out = append(out, item)
	}
	return out
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lookupGuestForAction — опознание гостя для посадки и брони.
//
// Поле `guest` — новое и умное (ник, телефон, имя). Поле `nickname` остаётся
// СТРОГИМ: так его понимала админка до Е0, и менять смысл живого поля молча
// нельзя — «посадить Гостя» не должно вдруг начать спрашивать «которого из
// Гость-1, Гость-77». Пишет ответ сам; вернул nil — обработчику выходить.
func lookupGuestForAction(c *gin.Context, guest, nickname string) *models.User {
	if strings.TrimSpace(guest) == "" {
		if strings.TrimSpace(nickname) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "guest_empty",
				"message": "Нужен ник, телефон или имя гостя"})
			return nil
		}
		var user models.User
		if err := db.First(&user, "nickname = ? AND role = ?",
			nickname, models.UserRolePlayer).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found",
				"message": "Гость с таким ником не найден"})
			return nil
		}
		return &user
	}

	user, code, cands := resolveGuest(guest)
	switch code {
	case "":
		return user
	case "guest_ambiguous":
		c.JSON(http.StatusConflict, gin.H{"code": code,
			"message":    "Под запрос подходит несколько гостей — выбери нужного",
			"candidates": guestCandidates(cands, guest)})
	case "guest_empty":
		c.JSON(http.StatusBadRequest, gin.H{"code": code,
			"message": "Нужен ник, телефон или имя гостя"})
	default:
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found",
			"message": "Гость не найден: ни по нику, ни по телефону, ни по имени"})
	}
	return nil
}
