package main

// З13-и1 «Ставки VAT: схема и снимки» (OWNER.md, трек З; №72 «а»; решения основателя 04.10 — бухгалтерия под любого
// бухгалтера: каждый вопрос бухгалтеру — настройка владельца; только Польша; VAT у пополнения — настройкой, по
// умолчанию «при трате»).
//
// Ставка VAT — у зоны, пакета и позиции; ручки зон, пакетов и позиций принимают и отдают `vat_pct`. Снимок ставки в
// операции пишет модель (models/vat.go). Здесь — список ставок для экрана и «ставка всей категории» одной кнопкой.
// Всё — только владельцу.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// vatBadMessage — ответ на ставку не из списка: «Ставка VAT — одна из: 23 %, 8 %, 5 %, 0 %, zw».
var vatBadMessage = "Ставка VAT — одна из: " + vatRatesText()

func vatRatesText() string {
	parts := make([]string, 0, len(models.VatRates))
	for _, v := range models.VatRates {
		parts = append(parts, models.VatLabel(v))
	}
	return strings.Join(parts, ", ")
}

// vatFromRequest — ставка из запроса: не прислана — def; не из списка — false (чистая, тест).
func vatFromRequest(v *int, def int) (int, bool) {
	if v == nil {
		return def, true
	}
	return *v, models.ValidVat(*v)
}

// GET /admin/vat/rates — ставки, которые можно выбрать, с подписями (owner): экран строит выбор по ним, и смена
// ставок законом — одна строка на сервере.
func handleVatRates(c *gin.Context) {
	out := make([]gin.H, 0, len(models.VatRates))
	for _, v := range models.VatRates {
		out = append(out, gin.H{"vat_pct": v, "label": models.VatLabel(v)})
	}
	c.JSON(http.StatusOK, gin.H{"rates": out, "default": models.VatDefault})
}

type goodsVatRequest struct {
	Category *string `json:"category"` // "" — позиции без категории
	VatPct   *int    `json:"vat_pct"`
}

// PUT /admin/vat/goods — ставка VAT всей категории позиций одной кнопкой (owner): напитки — 23 %, кухня — 8 %.
// Прошлые продажи не меняются — у них свой снимок.
func handleGoodsVat(c *gin.Context) {
	var req goodsVatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Category == nil || req.VatPct == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны категория и ставка"})
		return
	}
	if !models.ValidVat(*req.VatPct) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_vat", "message": vatBadMessage})
		return
	}
	cat := strings.TrimSpace(*req.Category)
	res := db.Model(&models.Good{}).Where("category = ? AND vat_pct <> ?", cat, *req.VatPct).Update("vat_pct", *req.VatPct)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "Не сохранилось — попробуй ещё раз"})
		return
	}
	var total int64
	db.Model(&models.Good{}).Where("category = ?", cat).Count(&total)
	if res.RowsAffected > 0 {
		name := cat
		if name == "" {
			name = "без категории"
		}
		n := int(res.RowsAffected)
		logAdminAction(c, "goods_vat", nil, fmt.Sprintf("категория «%s»: VAT → %s (%d %s)",
			name, models.VatLabel(*req.VatPct), n, plural(n, "позиция", "позиции", "позиций")))
	}
	c.JSON(http.StatusOK, gin.H{"category": cat, "vat_pct": *req.VatPct, "changed": res.RowsAffected, "total": total})
}

// ── З13-и3 «Продажи по ставкам VAT» (OWNER.md, трек З; №72 «а»; решения основателя 04.10, З-Р37) ──
//
// Облагаемое за период — по настройкам «Экономика → Бухгалтерия»: когда VAT с пополнения (acc_vat_moment),
// плательщик VAT (acc_vat_payer), подарки (acc_gift_vat). Каждая операция — строка: вид, клубные сутки, ставка, способ,
// брутто со знаком.
//
//	«при трате» (по умолчанию; bon różnego przeznaczenia): время с кошелька — по моменту списания и ставке зоны сессии;
//	пакеты — при выдаче, по ставке пакета (и с кошелька); товары и кухня — по моменту оплаты, по ставке позиции (и с
//	кошелька): так их пробивает фискальная касса; пополнения — без VAT;
//	«при пополнении» (bon jednego przeznaczenia): пополнения — по ставке пополнения на их момент; пакеты, товары и кухня
//	за живые деньги — по своей ставке; с кошелька — без VAT: его взяли с пополнения;
//	возвраты — минусом в день отмены, по ставке, с которой VAT брали: живыми деньгами, а «при трате» — и на кошелёк
//	(«при пополнении» оплаченное с кошелька и возвращённое деньгами — по ставке пополнения); «не возвращали» и
//	неоплаченный долг — ничего: деньги остались у клуба или их не было;
//	подарки товаров — по настройке «Подарки в VAT»: по себестоимости партий, по ставке позиции, в день подарка;
//	освобождённый от VAT — всё «zw», подарков нет.
//
// Ставка операции — её снимок (З13-и1); NULL (операция до ставок) — нынешняя ставка позиции, пакета, зоны или
// пополнения; взять неоткуда — 23 %. VAT — из брутто дня по ставке, как в дневном отчёте фискальной кассы: брутто ×
// r / (100 + r), до гроша; нетто = брутто − VAT; за период — сумма дней. «При пополнении» брутто — это «Доходы» минус
// «Возвраты» «Прибыли» (З12-и3) за тот же период: те же правила. GET /admin/reports/vat (владелец); экран — «Обзор».

// vatConfig — настройки «Бухгалтерии», по которым считается VAT.
type vatConfig struct {
	Payer    bool // плательщик VAT; нет — всё «zw»
	Topup    bool // VAT при пополнении; нет — при трате
	Gifts    bool // подарки товаров — в VAT по себестоимости
	TopupVat int  // ставка пополнения
	RyTime   int  // З13-и6: ставка ryczałtu времени и пакетов, десятые доли процента (строки vatRow.Ry)
}

func vatConfigNow() vatConfig {
	set := func(key string) int64 { return settingInt64(key, settingDefaults[key]) }
	return vatConfig{Payer: set("acc_vat_payer") == 1, Topup: set("acc_vat_moment") == 1, Gifts: set("acc_gift_vat") == 1,
		TopupVat: int(set("acc_topup_vat")), RyTime: ryczaltTimeNow()}
}

// vatKinds — виды операций по порядку для экрана.
var vatKinds = []string{"time", "packs", "sales", "deposits", "gifts", "returns"}

// vatRow — облагаемые операции одного вида, дня, ставки и способа.
type vatRow struct {
	Kind   string // time | packs | sales | deposits | gifts | returns
	D      string // клубные сутки, ГГГГ-ММ-ДД
	Vat    int    // ставка, % (−1 — zw)
	Ry     int    // З13-и6: ставка ryczałtu, десятые доли процента: время, пакеты и пополнения — настройка, товары — позиции; у подарка — 0
	Method string // как платили; у возврата — как вернули, у подарка — comp
	G      int64  // брутто со знаком, гроши
	N      int64  // операций (у подарков — штук)
}

// vatSalesSQL — облагаемые операции за [@from, @to) по правилам выше; @spv — «при пополнении», @topup — ставка
// пополнения сейчас, @def — ставка, когда взять её неоткуда. %[1]s…%[7]s — клубные сутки момента операции. З13-и6:
// ry — ставка ryczałtu строки: время, пакеты и пополнения — @rtime (настройка), товары и кухня — нынешняя ставка
// позиции (снимка нет: ставку ryczałtu задаёт вид деятельности, а не дата), позиции уже нет — @rgoods; возврат
// деньгами оплаченного с кошелька «при пополнении» — @rtime, как его пополнение. CAST(@rtime AS int), а не «::int»:
// без типа параметр первой ветки UNION — text, а «@rtime::int» GORM за именованный параметр не считает.
const vatSalesSQL = `SELECT kind, d, vat, ry, method, SUM(g)::bigint AS g, COUNT(*) AS n FROM (
  SELECT 'deposits' AS kind, %[1]s AS d, COALESCE(dp.vat_pct, @topup) AS vat, CAST(@rtime AS int) AS ry, dp.method, ROUND(dp.amount_pln * 100)::bigint AS g
    FROM deposits dp WHERE @spv AND dp.created_at >= @from AND dp.created_at < @to
  UNION ALL
  SELECT 'time', %[2]s, COALESCE(se.vat_pct, z.vat_pct, @def), CAST(@rtime AS int), 'wallet', -w.amount_grosz
    FROM wallet_transactions w LEFT JOIN sessions se ON se.id = w.ref_id
    LEFT JOIN computers pc ON pc.id = se.computer_id LEFT JOIN zones z ON z.id = pc.zone_id
   WHERE NOT @spv AND w.kind = 'session_spend' AND w.amount_grosz < 0 AND w.created_at >= @from AND w.created_at < @to
  UNION ALL
  SELECT 'packs', %[3]s, COALESCE(p.vat_pct, tp.vat_pct, z.vat_pct, @def), CAST(@rtime AS int), p.method, ROUND(p.price_pln * 100)::bigint
    FROM user_packages p LEFT JOIN time_packages tp ON tp.id = p.package_id LEFT JOIN zones z ON z.id = p.zone_id
   WHERE p.method <> 'comp' AND NOT (@spv AND p.method = 'wallet') AND p.created_at >= @from AND p.created_at < @to
  UNION ALL
  SELECT 'sales', %[4]s, COALESCE(s.vat_pct, g.vat_pct, @def), COALESCE(g.ryczalt_rate, @rgoods)::int, s.method, ROUND(s.total_pln * 100)::bigint
    FROM sales s LEFT JOIN goods g ON g.id = s.good_id
   WHERE s.method <> 'comp' AND NOT (@spv AND s.method = 'wallet') AND s.paid_at >= @from AND s.paid_at < @to
  UNION ALL
  SELECT 'returns', %[5]s, COALESCE(dp.vat_pct, @topup), CAST(@rtime AS int), COALESCE(dp.refund_method, dp.method), -dp.refund_grosz
    FROM deposits dp
   WHERE @spv AND dp.voided_at >= @from AND dp.voided_at < @to AND dp.refund_grosz <> 0
     AND COALESCE(dp.refund_method, dp.method) IN ('cash', 'card', 'blik')
  UNION ALL
  SELECT 'returns', %[6]s, CASE WHEN @spv AND p.method = 'wallet' THEN @topup ELSE COALESCE(p.vat_pct, tp.vat_pct, z.vat_pct, @def) END,
         CAST(@rtime AS int), COALESCE(p.refund_method, p.method), -p.refund_grosz
    FROM user_packages p LEFT JOIN time_packages tp ON tp.id = p.package_id LEFT JOIN zones z ON z.id = p.zone_id
   WHERE p.method <> 'comp' AND p.voided_at >= @from AND p.voided_at < @to AND p.refund_grosz <> 0
     AND (COALESCE(p.refund_method, p.method) IN ('cash', 'card', 'blik') OR (NOT @spv AND COALESCE(p.refund_method, p.method) = 'wallet'))
  UNION ALL
  SELECT 'returns', %[7]s, CASE WHEN @spv AND s.method = 'wallet' THEN @topup ELSE COALESCE(s.vat_pct, g.vat_pct, @def) END,
         CASE WHEN @spv AND s.method = 'wallet' THEN CAST(@rtime AS int) ELSE COALESCE(g.ryczalt_rate, @rgoods)::int END,
         COALESCE(s.refund_method, s.method), -ROUND(s.total_pln * 100)::bigint
    FROM sales s LEFT JOIN goods g ON g.id = s.good_id
   WHERE s.paid_at IS NOT NULL AND s.method <> 'comp' AND s.voided_at >= @from AND s.voided_at < @to
     AND (COALESCE(s.refund_method, s.method) IN ('cash', 'card', 'blik') OR (NOT @spv AND COALESCE(s.refund_method, s.method) = 'wallet'))
) x GROUP BY 1, 2, 3, 4, 5`

// vatRowsIn — облагаемые операции периода p по настройкам cfg.
func vatRowsIn(p period, reportHour int, cfg vatConfig) []vatRow {
	day := func(col string) string { return "to_char(" + dayExpr(col, reportHour) + ", 'YYYY-MM-DD')" }
	var rows []vatRow
	db.Raw(fmt.Sprintf(vatSalesSQL, day("dp.created_at"), day("w.created_at"), day("p.created_at"), day("s.paid_at"),
		day("dp.voided_at"), day("p.voided_at"), day("s.voided_at")),
		map[string]any{"from": p.From, "to": p.To, "spv": cfg.Topup, "topup": cfg.TopupVat, "def": models.VatDefault,
			"rtime": cfg.RyTime, "rgoods": models.RyczaltGoodsDefault}).Scan(&rows)
	if !cfg.Gifts || !cfg.Payer {
		return rows
	}
	// подарки — по себестоимости партий, тем же счётом, что «Подарено» и маржа (goodsEconSQL), по дню и ставке
	key := day("s.created_at") + " || '|' || COALESCE(s.vat_pct, g.vat_pct, " + strconv.Itoa(models.VatDefault) + ")"
	var gifts []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, key, "LEFT JOIN goods g ON g.id = s.good_id", "method = '"+models.SaleMethodComp+"'"),
		map[string]any{"from": p.From, "to": p.To}).Scan(&gifts)
	for _, x := range gifts {
		d, v, ok := strings.Cut(x.Key, "|")
		r, err := strconv.Atoi(v)
		if !ok || err != nil || x.Cost == 0 {
			continue // подарок без себестоимости (у партии нет цены) — в VAT не входит
		}
		rows = append(rows, vatRow{Kind: "gifts", D: d, Vat: r, Method: models.SaleMethodComp, G: x.Cost, N: x.Items})
	}
	return rows
}

// vatOf — VAT внутри брутто по ставке r (чистая, тест): брутто × r / (100 + r) до гроша, половина — от нуля; zw и
// 0 % — ноль. Так считает дневной отчёт фискальной кассы.
func vatOf(gross int64, r int) int64 {
	if r <= 0 {
		return 0
	}
	d, n := int64(100+r), 2*gross*int64(r)
	if n < 0 {
		return (n - d) / (2 * d)
	}
	return (n + d) / (2 * d)
}

// vatCell — брутто и VAT одной ставки (за день или за период), гроши, и сколько операций.
type vatCell struct {
	Gross, Vat, N int64
}

// vatAgg — продажи по ставкам за период.
type vatAgg struct {
	Days       map[string]map[int]*vatCell // день → ставка
	Rates      map[int]*vatCell            // ставка за период: суммы дней
	Kinds      map[string]*vatCell         // вид: брутто и операций (VAT по видам не считается)
	Methods    map[string]int64            // способ: брутто
	DayMethods map[string]map[string]int64 // З13-и4: день → способ: брутто (лист «Продажи по дням и ставкам»)
}

// vatSum — строки по дням и ставкам (чистая, тест): освобождённому от VAT всё — «zw»; VAT дня по ставке — из брутто
// дня (vatOf), за период — сумма дней.
func vatSum(rows []vatRow, payer bool) vatAgg {
	a := vatAgg{Days: map[string]map[int]*vatCell{}, Rates: map[int]*vatCell{}, Kinds: map[string]*vatCell{}, Methods: map[string]int64{},
		DayMethods: map[string]map[string]int64{}}
	cell := func(m map[int]*vatCell, r int) *vatCell {
		if m[r] == nil {
			m[r] = &vatCell{}
		}
		return m[r]
	}
	for _, x := range rows {
		r := x.Vat
		if !payer {
			r = models.VatZW
		}
		if a.Days[x.D] == nil {
			a.Days[x.D] = map[int]*vatCell{}
		}
		c := cell(a.Days[x.D], r)
		c.Gross += x.G
		c.N += x.N
		if a.Kinds[x.Kind] == nil {
			a.Kinds[x.Kind] = &vatCell{}
		}
		a.Kinds[x.Kind].Gross += x.G
		a.Kinds[x.Kind].N += x.N
		a.Methods[x.Method] += x.G
		if a.DayMethods[x.D] == nil {
			a.DayMethods[x.D] = map[string]int64{}
		}
		a.DayMethods[x.D][x.Method] += x.G
	}
	for _, day := range a.Days {
		for r, c := range day {
			c.Vat = vatOf(c.Gross, r)
			t := cell(a.Rates, r)
			t.Gross += c.Gross
			t.Vat += c.Vat
			t.N += c.N
		}
	}
	return a
}

// vatOrder — ставки по порядку списка VatRates (23, 8, 5, 0, zw), прочие — после, от большей (чистая, тест).
func vatOrder(m map[int]*vatCell) []int {
	pos := func(r int) int {
		for i, v := range models.VatRates {
			if v == r {
				return i
			}
		}
		return len(models.VatRates)
	}
	out := make([]int, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if pi, pj := pos(out[i]), pos(out[j]); pi != pj {
			return pi < pj
		}
		return out[i] > out[j]
	})
	return out
}

// vatCellOut — ставка для ответа: брутто, нетто, VAT.
func vatCellOut(r int, c *vatCell) gin.H {
	pln := models.PLNFromGrosz
	return gin.H{"vat_pct": r, "label": models.VatLabel(r), "gross_pln": pln(c.Gross), "net_pln": pln(c.Gross - c.Vat),
		"vat_pln": pln(c.Vat), "count": c.N}
}

// GET /admin/reports/vat — продажи по ставкам VAT за период: по ставкам, по дням (до 92 дней, как кассовая книга), из
// чего и какими способами; как считали — настройки «Бухгалтерии» (owner).
func handleReportVat(c *gin.Context) {
	p, _, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	cfg := vatConfigNow()
	a := vatSum(vatRowsIn(p, reportHour, cfg), cfg.Payer)
	pln := models.PLNFromGrosz
	rates := make([]gin.H, 0, len(a.Rates))
	var total vatCell
	for _, r := range vatOrder(a.Rates) {
		x := a.Rates[r]
		rates = append(rates, vatCellOut(r, x))
		total.Gross += x.Gross
		total.Vat += x.Vat
		total.N += x.N
	}
	kinds := gin.H{}
	for _, k := range vatKinds {
		x := a.Kinds[k]
		if x == nil {
			x = &vatCell{}
		}
		kinds[k] = gin.H{"pln": pln(x.Gross), "count": x.N}
	}
	methods := gin.H{}
	for m, g := range a.Methods {
		methods[m] = pln(g)
	}
	var days []gin.H
	if p.Days <= maxBookDays {
		days = make([]gin.H, 0, p.Days)
		for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
			k := d.Format("2006-01-02")
			var dt vatCell
			drs := make([]gin.H, 0, len(a.Days[k]))
			for _, r := range vatOrder(a.Days[k]) {
				x := a.Days[k][r]
				drs = append(drs, vatCellOut(r, x))
				dt.Gross += x.Gross
				dt.Vat += x.Vat
			}
			days = append(days, gin.H{"date": k, "gross_pln": pln(dt.Gross), "net_pln": pln(dt.Gross - dt.Vat), "vat_pln": pln(dt.Vat), "rates": drs})
		}
	}
	moment := "spend"
	if cfg.Topup {
		moment = "topup"
	}
	c.JSON(http.StatusOK, gin.H{
		"period":   p.out(),
		"settings": gin.H{"payer": cfg.Payer, "moment": moment, "gifts": cfg.Gifts, "topup_vat": cfg.TopupVat},
		"rates":    rates,
		"total":    gin.H{"gross_pln": pln(total.Gross), "net_pln": pln(total.Gross - total.Vat), "vat_pln": pln(total.Vat), "count": total.N},
		"kinds":    kinds,
		"methods":  methods,
		"days":     days, // до 92 дней; длиннее — без ряда (null)
	})
}
