package main

// К3-и3 (KIOSK.md, №18): ошибки — человеческим языком и со стабильным кодом.
//
// Раньше «message» у невалидного запроса был текстом валидатора
// («Key: 'registerRequest.Password' Error:Field validation for 'Password'
// failed on the 'min' tag»), а у сбоя БД — сырым err.Error() с SQL внутри.
// Гость на киоске и в PWA читал это как есть. Теперь:
//   · валидатор → код «<поле>_<что не так>» и фраза («Пароль — минимум 6
//     символов», «Ник — максимум 32 символа»);
//   · сбой БД → одна фраза наружу, полный текст — в лог сервера.
// Клиенты показывают message как показывали — правка только серверная.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// dbErrorMessage — то, что видит гость вместо текста ошибки Postgres.
const dbErrorMessage = "Не получилось сохранить — попробуй ещё раз или позови админа"

// fieldLabels — как поле запроса называется по-русски (ключ — snake_case
// имени поля структуры: NewPassword → new_password).
var fieldLabels = map[string]string{
	"nickname":         "Ник",
	"password":         "Пароль",
	"new_password":     "Новый пароль",
	"current_password": "Текущий пароль",
	"login":            "Логин",
	"email":            "Email",
	"phone":            "Телефон",
	"refresh_token":    "Токен",
	"planned_min":      "Минуты",
	"computer_id":      "ПК",
	"club_id":          "Клуб",
	"start_time":       "Время брони",
	"duration_min":     "Длительность",
	"minutes":          "Минуты",
	"amount":           "Сумма",
	"amount_pln":       "Сумма",
	"text":             "Текст",
	"talent_id":        "Талант",
	"good_id":          "Товар",
	"qty":              "Количество",
}

// respondBindError — ответ 400 на ошибку ShouldBindJSON.
func respondBindError(c *gin.Context, err error) {
	code, msg := bindError(err)
	c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
}

// respondDBError — ответ 500 на ошибку БД: гостю фраза, в лог — причина и ручка.
func respondDBError(c *gin.Context, err error) {
	log.Printf("db_error %s %s: %v", c.Request.Method, c.FullPath(), err)
	c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": dbErrorMessage})
}

// bindError — код и текст для ошибки привязки JSON (чистая функция, тест).
// Берём первую ошибку валидатора: гость чинит по одной, как в любой форме.
func bindError(err error) (code, message string) {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		return fieldError(ve[0].Field(), ve[0].Tag(), ve[0].Param(), ve[0].Kind().String())
	}
	return "invalid_request", "Не удалось прочитать запрос — проверь данные и попробуй снова"
}

// fieldError — одна ошибка валидатора → код и фраза. kind — вид поля
// («string» считает символы, остальные — значение).
func fieldError(field, tag, param, kind string) (string, string) {
	key := snakeCase(field)
	label := fieldLabels[key]
	if label == "" {
		label = key
	}
	n, _ := strconv.Atoi(param)
	switch tag {
	case "required":
		return key + "_required", fmt.Sprintf("Не хватает поля «%s»", strings.ToLower(label))
	case "email":
		return key + "_invalid", "Email выглядит неправильно — проверь адрес"
	case "min", "gte":
		if kind == "string" {
			return key + "_short", fmt.Sprintf("%s — минимум %d %s", label, n, plural(n, "символ", "символа", "символов"))
		}
		return key + "_min", fmt.Sprintf("%s — минимум %s", label, param)
	case "max", "lte":
		if kind == "string" {
			return key + "_long", fmt.Sprintf("%s — максимум %d %s", label, n, plural(n, "символ", "символа", "символов"))
		}
		return key + "_max", fmt.Sprintf("%s — максимум %s", label, param)
	case "oneof":
		return key + "_invalid", fmt.Sprintf("%s — допустимо: %s", label, strings.ReplaceAll(param, " ", ", "))
	}
	return key + "_invalid", fmt.Sprintf("%s — неверное значение", label)
}

// snakeCase — имя поля структуры Go → snake_case (PlannedMin → planned_min).
func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// plural — русская форма по числу: 1 символ, 3 символа, 6 символов.
func plural(n int, one, few, many string) string {
	n10, n100 := n%10, n%100
	switch {
	case n10 == 1 && n100 != 11:
		return one
	case n10 >= 2 && n10 <= 4 && (n100 < 10 || n100 >= 20):
		return few
	}
	return many
}

// plnRU — сумма в злотых с запятой, как на ценнике: 5.75 → «5,75».
func plnRU(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}
