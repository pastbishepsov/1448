package models

// З13-и6 «Профили учёта» (OWNER.md, трек З; №76; решения основателя 04.10, З-Р37 — бухгалтерия под любого бухгалтера,
// только Польша). Справочники польских форм учёта: ставки ryczałtu и колонки KPiR. Закон поменяет ставку или образец
// книги — меняется строка здесь, экран берёт списки с сервера. Какая ставка или колонка у клуба — решает владелец со
// своим бухгалтером: здесь только список и самое частое значение по умолчанию.

import "strconv"

// RyczaltRates — ставки ryczałtu od przychodów ewidencjonowanych в десятых долях процента: 170 — 17 %, 125 — 12,5 %,
// 85 — 8,5 %, 30 — 3 %. От большей к меньшей — так идут колонки ewidencji.
var RyczaltRates = []int{170, 150, 140, 125, 120, 100, 85, 55, 30, 20}

// RyczaltGoodsDefault — ставка новой позиции: 3 % — торговля и гастрономия без напитков крепче 1,5 % (бар клуба);
// пиво и другое спиртное — обычно 8,5 %, это ставка позиции.
const RyczaltGoodsDefault = 30

// RyczaltTimeDefault — ставка времени за ПК и пакетов по умолчанию: 15 % — услуги, связанные с развлечениями и
// отдыхом (PKWiU, раздел 93). Иначе классифицирует бухгалтер — настройка «Ставка ryczałtu времени».
const RyczaltTimeDefault = 150

// ValidRyczalt — ставка из списка RyczaltRates (чистая, тест).
func ValidRyczalt(r int) bool {
	for _, v := range RyczaltRates {
		if v == r {
			return true
		}
	}
	return false
}

// RyczaltLabel — ставка для людей: «15 %», «8,5 %» (чистая, тест).
func RyczaltLabel(r int) string {
	s := strconv.Itoa(r / 10)
	if r%10 != 0 {
		s += "," + strconv.Itoa(r%10)
	}
	return s + " %"
}

// Колонки KPiR — куда идёт расход категории по образцу книги с 2026 года (rozporządzenie MF z 6.09.2025: 19 колонок
// вместо 17, номера сдвинулись на два). KpirNone — расход в книгу не идёт (например, налог с дохода).
const (
	KpirNone  = 0
	KpirGoods = 12 // zakup towarów handlowych i materiałów według cen zakupu
	KpirSide  = 13 // koszty uboczne zakupu
	KpirWages = 14 // wynagrodzenia w gotówce i w naturze
	KpirOther = 15 // pozostałe wydatki
)

// KpirCols — колонки, которые можно выбрать у категории, по порядку книги; «не в книгу» — последней.
var KpirCols = []int{KpirGoods, KpirSide, KpirWages, KpirOther, KpirNone}

// ValidKpir — колонка из списка KpirCols (чистая, тест).
func ValidKpir(c int) bool {
	for _, v := range KpirCols {
		if v == c {
			return true
		}
	}
	return false
}

// KpirDefault — колонка новой категории: закупка товаров или кухни — 12, остальное — 15 (чистая, тест).
func KpirDefault(purchase bool) int {
	if purchase {
		return KpirGoods
	}
	return KpirOther
}
