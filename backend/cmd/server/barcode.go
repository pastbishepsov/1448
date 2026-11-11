package main

// З23-и5а «Штрихкоды — сервер» (OWNER.md, трек З; №54 «в», З-Р28; миграция 099). У позиции — штрихкод (goods.barcode):
// EAN-13 или EAN-8 с верной контрольной цифрой; UPC-A (12 цифр — американские коды) — тот же EAN-13 с ведущим нулём.
// Пробелы сканера (кое-какие ставят их между группами цифр) убираются. Код уникален в клубе: занят — 409 barcode_taken с
// именем позиции. Ставит и убирает владелец карточкой позиции (POST и PATCH /admin/goods, поле barcode; "" — убрать), в
// журнале — «штрихкод: … → …». В ценнике (GET /admin/goods) — у каждой позиции: по нему сканер в продаже, приёмке и
// инвентаризации находит позицию (экран — З23-и5б).

import (
	"strings"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func init() {
	goodErrors["bad_barcode"] = "Штрихкод — EAN-13 или EAN-8: 13 или 8 цифр, последняя — контрольная; проверь, что код считался целиком"
	goodErrors["barcode_taken"] = "Этот штрихкод уже у другой позиции"
}

// barcodeNorm — штрихкод из запроса (чистая, тест): пробелы убираются; пусто — "" и true (штрихкода нет); 12 цифр (UPC-A)
// — EAN-13 с ведущим нулём; 8 или 13 цифр с верной контрольной — код и true; иначе — "" и false.
func barcodeNorm(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), "")
	if s == "" {
		return "", true
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	if len(s) == 12 {
		s = "0" + s
	}
	if (len(s) != 8 && len(s) != 13) || !eanValid(s) {
		return "", false
	}
	return s, true
}

// eanValid — контрольная цифра EAN-8 и EAN-13 (чистая, тест): цифры перед контрольной справа налево с весами 3, 1, 3, 1 …;
// контрольная дополняет сумму до десятка.
func eanValid(s string) bool {
	sum := 0
	for i := len(s) - 2; i >= 0; i-- {
		d := int(s[i] - '0')
		if (len(s)-2-i)%2 == 0 {
			d *= 3
		}
		sum += d
	}
	return (10-sum%10)%10 == int(s[len(s)-1]-'0')
}

// barcodeOwner — какая позиция клуба уже носит код (кроме self); нет — "".
func barcodeOwner(clubID uuid.UUID, code string, self uuid.UUID) string {
	var g models.Good
	if db.Select("id, name").Where("club_id = ? AND barcode = ? AND id <> ?", clubID, code, self).First(&g).Error != nil {
		return ""
	}
	return g.Name
}

// barcodeChange — строка журнала о смене штрихкода (чистая, тест): «штрихкод: 590…», «штрихкод убран», «штрихкод 590… → 400…»;
// не менялся — "".
func barcodeChange(was *string, now string) string {
	w := ""
	if was != nil {
		w = *was
	}
	switch {
	case w == now:
		return ""
	case w == "":
		return "штрихкод: " + now
	case now == "":
		return "штрихкод убран (был " + w + ")"
	}
	return "штрихкод " + w + " → " + now
}

// barcodePtr — код для модели: "" — nil (в базе NULL: уникальный индекс пустые не сравнивает).
func barcodePtr(code string) *string {
	if code == "" {
		return nil
	}
	return &code
}
