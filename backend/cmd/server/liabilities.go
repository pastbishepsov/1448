package main

// З12-и1 «Обязательства клуба и сгоревшее» (OWNER.md, трек З; №49; З-Р9; решение основателя 03.10 —
// сгоревшие монеты тоже в «Сгорело»).
//
// «Обязательства клуба» — снимок «сколько клуб должен гостям сейчас», одна цифра и её состав: кошельки
// (деньги на счету), пакеты (неотыгранный остаток по доле цены; у подаренного — минуты без злотых, З11-и5),
// монеты на руках (по курсу погашения), минуты за монеты и компенсации (по тарифу выдачи, пул без зоны — по
// клубному); минус дебиторка — что гости должны клубу (З11-и4). Каждая часть — тем же кодом, что «Кошельки
// гостей» и «Долг временем» в «Деньгах», «на руках» и «должны временем» в «Монетах», «Дебиторка сейчас»:
// цифры сходятся.
//
// «Сгорело за период» — обязательства, ушедшие без игры: просроченные пакеты — неотыгранный остаток по доле
// цены (у подаренного — минуты без злотых), по моменту срока; монеты, сгоревшие у неактивных (В4-3), — по
// нынешнему курсу погашения, по моменту сгорания. Список — последние maxBurnItems.
//
// Блок — в «Деньгах» и в «Бухгалтерии» → «Касса» (рядом с ящиком): GET /admin/reports/money и
// GET /admin/cash/book отдают `liabilities` и `burned`.

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// walletOwed — деньги на кошельках (Г0-и4: обязательство клуба, не выручка) и у скольких.
type walletOwed struct {
	Total  int64 // грошей
	Guests int64
}

// walletsNow — «Кошельки гостей»: снимок.
func walletsNow() walletOwed {
	var w walletOwed
	db.Model(&models.User{}).
		Select("COALESCE(SUM(wallet_grosz),0) AS total, COUNT(*) FILTER (WHERE wallet_grosz > 0) AS guests").
		Scan(&w)
	return w
}

// packOwed — неотыгранные минуты пакетов: в злотых по доле цены, минутами и у скольких.
type packOwed struct {
	Grosz  int64
	Guests int64
	Min    int64
	Count  int64
}

// packsOwedNow — «Долг временем» (Е2-и6): обязательство по НЕОТЫГРАННЫМ минутам пакетов. Считаем по цене,
// за которую пакет продали (доля за остаток), а не по нынешней цене зоны: клуб должен ровно то время, которое
// гость купил, и переоценка задним числом превратила бы долг в фантазию. Просроченные не считаются — клуб их
// уже не должен (они — в «Сгорело»). З11-и5: подаренный пакет — минуты клуб должен, злотых за них не брал.
func packsOwedNow() packOwed {
	var p packOwed
	db.Model(&models.UserPackage{}).
		Select(`COALESCE(SUM(ROUND(price_pln * 100 * minutes_left / minutes_total)) FILTER (WHERE method <> 'comp'),0) AS grosz,
		        COUNT(DISTINCT user_id) AS guests, COALESCE(SUM(minutes_left),0) AS min, COUNT(*) AS count`).
		Where(`voided_at IS NULL AND minutes_left > 0
		       AND (expires_at IS NULL OR expires_at > now())`).
		Scan(&p)
	return p
}

// liabSum — клуб должен гостям (кошельки + пакеты + монеты + минуты) и за вычетом дебиторки; всё в грошах
// (чистая, тест).
func liabSum(wallets, packs, coins, minutes, receivable int64) (owed, net int64) {
	owed = wallets + packs + coins + minutes
	return owed, owed - receivable
}

// liabilitiesNow — «Обязательства клуба» сейчас: одна цифра (net) и её состав.
func liabilitiesNow() gin.H {
	wal, pk := walletsNow(), packsOwedNow()
	sr := spendRate()
	coins, holders := coinsOnHandNow()
	coinsG := models.GroszFromPLN(coinValuePLN(coins, sr))
	mo := minutesOwedNow()
	minG := models.GroszFromPLN(mo.pln())
	rcv, rcvGuests := receivablesTotal()
	owed, net := liabSum(wal.Total, pk.Grosz, coinsG, minG, rcv)
	pln := models.PLNFromGrosz
	return gin.H{
		"owed_pln": pln(owed), "net_pln": pln(net),
		"wallets":  gin.H{"pln": pln(wal.Total), "guests": wal.Guests},
		"packages": gin.H{"pln": pln(pk.Grosz), "minutes": pk.Min, "guests": pk.Guests, "count": pk.Count},
		"coins":    gin.H{"pln": pln(coinsG), "coins": coins, "guests": holders, "rate": sr},
		"minutes":  gin.H{"pln": pln(minG), "minutes": mo.Pool + mo.Zoned},
		// З11-и4: что гости должны клубу — бар и кухня в долг; вычитается
		"receivable": gin.H{"pln": pln(rcv), "guests": rcvGuests},
	}
}

const maxBurnItems = 100

// burnedRow — одно сгорание периода: пакет (остаток на момент срока) или монеты.
type burnedRow struct {
	Kind   string // package | coins
	ID     uuid.UUID
	UserID uuid.UUID
	Name   string // у пакета — его название
	Qty    int64  // минуты пакета или монеты
	Grosz  int64  // пакет — по доле цены (подаренный — 0); монеты — по курсу погашения
	Gift   bool   // подаренный пакет (З11-и5)
	At     time.Time
}

// sortBurned — порядок списка «Сгорело» (чистая, тест): свежие сверху, при равном моменте — пакеты, потом
// монеты, дальше по id.
func sortBurned(rows []burnedRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		if a.Kind != b.Kind {
			return a.Kind == "package"
		}
		return a.ID.String() < b.ID.String()
	})
}

// burnedPacksSQL — пакеты, чей срок вышел в окне [from, to) и уже наступил, с неотыгранным остатком;
// отменённые не сгорели — их отменили. Остаток — по доле цены, у подаренного — 0 zł.
const burnedPacksSQL = `
SELECT 'package' AS kind, id, user_id, name, minutes_left AS qty,
       (CASE WHEN method = 'comp' THEN 0 ELSE ROUND(price_pln * 100 * minutes_left / minutes_total) END)::bigint AS grosz,
       method = 'comp' AS gift, expires_at AS at
  FROM user_packages
 WHERE voided_at IS NULL AND minutes_left > 0 AND expires_at >= ? AND expires_at < ? AND expires_at <= now()`

// burnedIn — «Сгорело за период»: пакеты и монеты, итоги и список.
func burnedIn(from, to time.Time) gin.H {
	var packs []burnedRow
	db.Raw(burnedPacksSQL, from, to).Scan(&packs)
	var burns []models.CoinBurn
	db.Where("created_at >= ? AND created_at < ?", from, to).Find(&burns)
	sr := spendRate()

	var packG, packMin, giftMin, coins int64
	packGuests, coinGuests := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	rows := make([]burnedRow, 0, len(packs)+len(burns))
	for _, p := range packs {
		packG += p.Grosz
		packMin += p.Qty
		if p.Gift {
			giftMin += p.Qty
		}
		packGuests[p.UserID] = true
		rows = append(rows, p)
	}
	for _, b := range burns {
		coins += b.Coins
		coinGuests[b.UserID] = true
		rows = append(rows, burnedRow{Kind: "coins", ID: b.ID, UserID: b.UserID, Qty: b.Coins,
			Grosz: models.GroszFromPLN(coinValuePLN(b.Coins, sr)), At: b.CreatedAt})
	}
	// монеты — по курсу на сумму, а не сложением округлённых строк: как «на руках» в «Монетах»
	coinG := models.GroszFromPLN(coinValuePLN(coins, sr))
	sortBurned(rows)
	shown := rows[:min(len(rows), maxBurnItems)]
	ids := make([]string, 0, len(shown))
	for _, r := range shown {
		ids = append(ids, r.UserID.String())
	}
	nick := nicknamesByID(ids)
	items := make([]gin.H, 0, len(shown))
	for _, r := range shown {
		items = append(items, gin.H{"kind": r.Kind, "id": r.ID, "user_id": r.UserID, "guest": nick[r.UserID.String()],
			"name": r.Name, "qty": r.Qty, "pln": models.PLNFromGrosz(r.Grosz), "gift": r.Gift, "at": r.At})
	}
	pln := models.PLNFromGrosz
	return gin.H{
		"pln": pln(packG + coinG),
		"packages": gin.H{"pln": pln(packG), "count": len(packs), "minutes": packMin, "gift_minutes": giftMin,
			"guests": len(packGuests)},
		"coins": gin.H{"pln": pln(coinG), "coins": coins, "count": len(burns), "guests": len(coinGuests), "rate": sr},
		"items": items, "items_total": len(rows),
	}
}
