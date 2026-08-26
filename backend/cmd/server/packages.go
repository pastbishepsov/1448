package main

// Пакеты времени: каталог (спринт Е2-и1, OPERATOR.md, этап III).
//
// Пакет — купленный запас минут в конкретной зоне, а не деньги в кошельке
// (Р2). Смысл покупки в том, что час внутри пакета дешевле обычного: «3 часа
// STANDARD за 45 zł» при цене часа 20 zł. Кошелёк такой скидки дать не может,
// не смешав предоплаченные злотые со скидкой, — поэтому отдельная сущность,
// устроенная как ценник товаров В2.
//
// Зона обязательна (Р10), срок годности задаётся у каждого пакета (Р11).
// Каталог правит владелец; стойка и гость его видят — продавать и покупать
// вслепую нельзя.

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxPackagePrice   = 5000 // zł за пакет: выше — почти наверняка опечатка
	maxPackageMinutes = 6000 // 100 часов; больше не пакет, а абонемент на год
	maxPackageDays    = 3650 // 10 лет ≈ «бессрочно», но с явной границей
)

var packageErrors = map[string]string{
	"invalid_request": "Не разобрал запрос",
	"bad_name":        "Название: 1–64 символа",
	"bad_minutes":     fmt.Sprintf("Минуты — от 1 до %d", maxPackageMinutes),
	"bad_price":       fmt.Sprintf("Цена — от 0.01 до %d zł", maxPackagePrice),
	"bad_days":        fmt.Sprintf("Срок — от 0 (бессрочно) до %d дней", maxPackageDays),
	"zone_required":   "У пакета должна быть зона: час VIP и час STANDARD стоят по-разному",
	"zone_not_found":  "Зона не найдена",
	"no_zones":        "В клубе ещё нет зон — заведи зону, потом пакет",
	"no_club":         "Клуб не настроен",
	"pack_not_found":  "Пакет не найден",
	"has_sales":       "Пакет уже продавали — его нельзя удалить, только выключить",
	"db_error":        "Не сохранилось — попробуй ещё раз",
	// Е2-и2: выдача и покупка
	"bad_method":      "Оплата: наличные, карта, BLIK или кошелёк",
	"pack_required":   "Не выбран пакет",
	"pack_inactive":   "Пакет выключен — включи его в каталоге",
	"not_enough":      "На кошельке не хватает — сначала пополни",
	"banned":          "Аккаунт заблокирован — сначала разбань",
	"issue_not_found": "Выдача не найдена",
	"already_void":    "Выдача уже отменена",
	"user_not_found":  "Гость не найден",
}

func packFail(c *gin.Context, status int, code string) {
	msg := packageErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

// validatePackage — границы полей. Чистая функция (тест): цена и минуты
// определяют, сколько клуб должен гостю, поэтому опечатка тут дороже обычной.
func validatePackage(name string, minutes int, price float64, days int) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" || len([]rune(n)) > 64 {
		return false, "bad_name"
	}
	if minutes <= 0 || minutes > maxPackageMinutes {
		return false, "bad_minutes"
	}
	if price <= 0 || price > maxPackagePrice {
		return false, "bad_price"
	}
	if days < 0 || days > maxPackageDays {
		return false, "bad_days"
	}
	return true, ""
}

// packageHourPLN — во сколько обходится час внутри пакета. Чистая функция
// (тест). Ради этой цифры пакет и покупают, поэтому её считает сервер, а не
// каждый клиент по-своему.
func packageHourPLN(minutes int, price float64) float64 {
	if minutes <= 0 {
		return 0
	}
	return round2(price / float64(minutes) * 60)
}

func packOut(p *models.TimePackage, zoneName string, zoneRate float64) gin.H {
	out := gin.H{
		"id": p.ID, "name": p.Name, "zone_id": p.ZoneID, "zone": zoneName,
		"minutes": p.Minutes, "price_pln": p.PricePLN, "days_valid": p.DaysValid,
		"sort": p.Sort, "active": p.Active,
		"hour_pln": packageHourPLN(p.Minutes, p.PricePLN),
	}
	// Выгода считается от цены часа ЗОНЫ: без неё «45 zł за 3 часа» ни о чём
	// не говорит ни гостю, ни админу у стойки.
	if zoneRate > 0 {
		full := round2(zoneRate * float64(p.Minutes) / 60)
		out["full_pln"] = full
		out["save_pln"] = round2(full - p.PricePLN)
	}
	return out
}

// zoneIndex — зоны разом: имя и цена часа для каждой. Пакетов немного, но
// ходить в базу за зоной на каждую строку — плохая привычка.
func zoneIndex() map[uuid.UUID]models.Zone {
	var zones []models.Zone
	db.Find(&zones)
	idx := make(map[uuid.UUID]models.Zone, len(zones))
	for i := range zones {
		idx[zones[i].ID] = zones[i]
	}
	return idx
}

// GET /admin/packages — каталог пакетов (staff: продавать нужно обеим ролям).
// Выключенные отдаём с флагом — UI решает, показывать их или нет.
func handleAdminPackages(c *gin.Context) {
	var packs []models.TimePackage
	db.Order("sort, minutes").Find(&packs)
	zones := zoneIndex()
	out := make([]gin.H, 0, len(packs))
	for i := range packs {
		z := zones[packs[i].ZoneID]
		out = append(out, packOut(&packs[i], z.Name, z.RatePLN))
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "packages": out, "zones_count": len(zones)})
}

// GET /me/packages/catalog — тот же каталог гостю (Р12: гость покупает сам).
// Только включённые: выключенный пакет для гостя не существует.
func handleMyPackageCatalog(c *gin.Context) {
	var packs []models.TimePackage
	db.Where("active = TRUE").Order("sort, minutes").Find(&packs)
	zones := zoneIndex()
	out := make([]gin.H, 0, len(packs))
	for i := range packs {
		z := zones[packs[i].ZoneID]
		out = append(out, packOut(&packs[i], z.Name, z.RatePLN))
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "packages": out})
}

type packageRequest struct {
	Name      string   `json:"name"`
	ZoneID    string   `json:"zone_id"`
	Minutes   *int     `json:"minutes"`
	PricePLN  *float64 `json:"price_pln"`
	DaysValid *int     `json:"days_valid"`
	Sort      *int     `json:"sort"`
	Active    *bool    `json:"active"`
}

// POST /admin/packages — новый пакет (owner).
func handleAdminPackageCreate(c *gin.Context) {
	var req packageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		packFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	minutes, price, days := 0, 0.0, 0
	if req.Minutes != nil {
		minutes = *req.Minutes
	}
	if req.PricePLN != nil {
		price = *req.PricePLN
	}
	if req.DaysValid != nil {
		days = *req.DaysValid
	}
	if ok, code := validatePackage(req.Name, minutes, price, days); !ok {
		packFail(c, http.StatusBadRequest, code)
		return
	}
	zone, code := packageZone(req.ZoneID)
	if code != "" {
		status := http.StatusBadRequest
		if code == "no_zones" {
			status = http.StatusConflict
		}
		packFail(c, status, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		packFail(c, http.StatusConflict, "no_club")
		return
	}
	p := models.TimePackage{
		ClubID: club.ID, ZoneID: zone.ID, Name: strings.TrimSpace(req.Name),
		Minutes: minutes, PricePLN: price, DaysValid: days, Active: true,
	}
	if req.Sort != nil {
		p.Sort = *req.Sort
	}
	if req.Active != nil {
		p.Active = *req.Active
	}
	if err := db.Create(&p).Error; err != nil {
		packFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "package_create", nil,
		fmt.Sprintf("%s · %s · %d мин · %.2f zł%s", p.Name, zone.Name, p.Minutes, p.PricePLN, daysSuffix(p.DaysValid)))
	c.JSON(http.StatusCreated, gin.H{"package": packOut(&p, zone.Name, zone.RatePLN)})
}

// PATCH /admin/packages/:id — правка пакета (owner). Правка каталога НЕ
// трогает уже выданные минуты: они живут своей строкой у гостя и своим
// сроком — иначе смена цены задним числом переписала бы проданное.
func handleAdminPackageUpdate(c *gin.Context) {
	var p models.TimePackage
	if err := db.First(&p, "id = ?", c.Param("id")).Error; err != nil {
		packFail(c, http.StatusNotFound, "pack_not_found")
		return
	}
	var req packageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		packFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name, minutes, price, days := p.Name, p.Minutes, p.PricePLN, p.DaysValid
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
	}
	if req.Minutes != nil {
		minutes = *req.Minutes
	}
	if req.PricePLN != nil {
		price = *req.PricePLN
	}
	if req.DaysValid != nil {
		days = *req.DaysValid
	}
	if ok, code := validatePackage(name, minutes, price, days); !ok {
		packFail(c, http.StatusBadRequest, code)
		return
	}
	zones := zoneIndex()
	zone := zones[p.ZoneID]
	if strings.TrimSpace(req.ZoneID) != "" {
		z, code := packageZone(req.ZoneID)
		if code != "" {
			packFail(c, http.StatusBadRequest, code)
			return
		}
		zone = *z
	}

	changes := []string{}
	if name != p.Name {
		changes = append(changes, p.Name+" → "+name)
	}
	if minutes != p.Minutes {
		changes = append(changes, fmt.Sprintf("минуты %d → %d", p.Minutes, minutes))
	}
	if price != p.PricePLN {
		changes = append(changes, fmt.Sprintf("цена %.2f → %.2f zł", p.PricePLN, price))
	}
	if days != p.DaysValid {
		changes = append(changes, fmt.Sprintf("срок %d → %d дн", p.DaysValid, days))
	}
	if zone.ID != p.ZoneID {
		changes = append(changes, "зона: "+zone.Name)
	}
	p.Name, p.Minutes, p.PricePLN, p.DaysValid, p.ZoneID = name, minutes, price, days, zone.ID
	if req.Sort != nil {
		p.Sort = *req.Sort
	}
	if req.Active != nil && *req.Active != p.Active {
		p.Active = *req.Active
		changes = append(changes, map[bool]string{true: "включён", false: "выключен"}[p.Active])
	}
	if err := db.Save(&p).Error; err != nil {
		packFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if len(changes) > 0 {
		logAdminAction(c, "package_update", nil, p.Name+": "+strings.Join(changes, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"package": packOut(&p, zone.Name, zone.RatePLN)})
}

// DELETE /admin/packages/:id — удалить пакет (owner). Если его уже продавали,
// удалять нельзя: история выручки потеряла бы имя проданного — то же правило,
// что у товаров В2. Выключение остаётся всегда.
func handleAdminPackageDelete(c *gin.Context) {
	var p models.TimePackage
	if err := db.First(&p, "id = ?", c.Param("id")).Error; err != nil {
		packFail(c, http.StatusNotFound, "pack_not_found")
		return
	}
	if packageSold(p.ID) {
		packFail(c, http.StatusConflict, "has_sales")
		return
	}
	if err := db.Delete(&p).Error; err != nil {
		packFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "package_delete", nil, p.Name)
	c.JSON(http.StatusOK, gin.H{"deleted": p.ID})
}

// packageZone — разбор и проверка зоны из запроса (Р10: зона обязательна).
// Отдельно различаем «зону не указали» и «зон в клубе нет вообще»: второе —
// не ошибка ввода, а незаконченная настройка клуба, и говорить о ней надо
// иначе, иначе владелец будет искать опечатку там, где её нет.
func packageZone(raw string) (*models.Zone, string) {
	id := strings.TrimSpace(raw)
	if id == "" {
		var n int64
		db.Model(&models.Zone{}).Count(&n)
		if n == 0 {
			return nil, "no_zones"
		}
		return nil, "zone_required"
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", id).Error; err != nil {
		return nil, "zone_not_found"
	}
	return &z, ""
}

// packageSold — продавали ли этот пакет хоть раз. Отменённые выдачи тоже
// считаются: их строки остаются в истории и ссылаются на каталог.
func packageSold(id uuid.UUID) bool {
	var n int64
	db.Model(&models.UserPackage{}).Where("package_id = ?", id).Count(&n)
	return n > 0
}

func daysSuffix(days int) string {
	if days <= 0 {
		return " · бессрочно"
	}
	return fmt.Sprintf(" · %d дн", days)
}

// ── Выдача и покупка (Е2-и2) ──────────────────────────────────────────
//
// Два канала, один результат (Р12): у стойки админ продаёт за нал/карту/BLIK
// или списывает с кошелька, из приложения гость покупает сам — только с
// кошелька, живых денег в приложении пока нет.
//
// Деньги. Нал/карта/BLIK — это выручка: гость платит клубу здесь и сейчас.
// Кошелёк — НЕ выручка (Г-Р7): те злотые клуб уже получил при пополнении и
// уже записал в выручку тогда; списание с кошелька лишь переводит одно
// обязательство клуба в другое — из «должен деньгами» в «должен временем».
// Посчитать его выручкой второй раз значило бы задвоить кассу.

type issueRequest struct {
	PackageID string `json:"package_id"`
	Method    string `json:"method"` // cash | card | blik | wallet
}

// packageExpiry — до какого момента живут выданные минуты. Чистая функция
// (тест): срок считается от ВЫДАЧИ, а не от создания пакета в каталоге —
// гость покупает свой срок, а не остаток чужого.
func packageExpiry(days int, now time.Time) *time.Time {
	if days <= 0 {
		return nil // бессрочно (Р11)
	}
	t := now.AddDate(0, 0, days)
	return &t
}

// packageLive — годен ли выданный пакет прямо сейчас. Чистая функция (тест).
// Отменённый и просроченный не годятся, пустой — тоже: он уже отыгран.
func packageLive(p *models.UserPackage, now time.Time) bool {
	if p.VoidedAt != nil || p.MinutesLeft <= 0 {
		return false
	}
	return p.ExpiresAt == nil || now.Before(*p.ExpiresAt)
}

func userPackOut(p *models.UserPackage, now time.Time) gin.H {
	out := gin.H{
		"id": p.ID, "name": p.Name, "zone_id": p.ZoneID, "zone": p.ZoneName,
		"minutes_total": p.MinutesTotal, "minutes_left": p.MinutesLeft,
		"price_pln": p.PricePLN, "method": p.Method,
		"created_at": p.CreatedAt, "live": packageLive(p, now),
	}
	if p.ExpiresAt != nil {
		out["expires_at"] = p.ExpiresAt
		out["days_left"] = daysLeft(*p.ExpiresAt, now)
	}
	if p.VoidedAt != nil {
		out["voided_at"] = p.VoidedAt
	}
	return out
}

// daysLeft — сколько ПОЛНЫХ суток осталось, вверх: «сгорит через 0 дней»
// звучит как «уже сгорел», а пакет ещё жив до конца дня.
func daysLeft(exp, now time.Time) int {
	d := int(math.Ceil(exp.Sub(now).Hours() / 24))
	if d < 0 {
		return 0
	}
	return d
}

// issuePackage — общая часть обоих каналов: снимок каталога → строка выдачи,
// при оплате кошельком ещё и списание. Одна транзакция: пакет без денег и
// деньги без пакета одинаково плохи.
func issuePackage(user *models.User, pack *models.TimePackage, zone models.Zone,
	method string, by *uuid.UUID) (*models.UserPackage, int64, string) {

	now := time.Now()
	up := models.UserPackage{
		ClubID: pack.ClubID, UserID: user.ID, PackageID: &pack.ID, ZoneID: pack.ZoneID,
		Name: pack.Name, ZoneName: zone.Name,
		MinutesTotal: pack.Minutes, MinutesLeft: pack.Minutes,
		PricePLN: pack.PricePLN, Method: method,
		ExpiresAt: packageExpiry(pack.DaysValid, now), CreatedBy: by,
	}
	price := models.GroszFromPLN(pack.PricePLN)
	walletAfter := user.WalletGrosz

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&up).Error; err != nil {
			return err
		}
		if method != "wallet" {
			return nil
		}
		// Кошелёк проверяем ВНУТРИ транзакции: между чтением баланса и
		// списанием гость мог начать сессию, и биллинг уже забрал бы своё.
		var fresh models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&fresh, "id = ?", user.ID).Error; err != nil {
			return err
		}
		if fresh.WalletGrosz < price {
			return errNotEnough
		}
		var werr error
		walletAfter, werr = walletApply(tx, walletOp{
			UserID: user.ID, Kind: models.WalletTxPackageSpend, Amount: -price,
			RefType: "package", RefID: &up.ID, CreatedBy: by,
		})
		return werr
	})
	if err != nil {
		if errors.Is(err, errNotEnough) {
			return nil, 0, "not_enough"
		}
		return nil, 0, "db_error"
	}
	notifyUser(user.ID, "package_added", map[string]any{
		"name": up.Name, "zone": up.ZoneName, "minutes": up.MinutesTotal,
		"expires_at": up.ExpiresAt,
	})
	return &up, walletAfter, ""
}

var errNotEnough = errors.New("not_enough")

// POST /admin/users/:id/packages — выдать гостю пакет у стойки (staff).
// Цель — только player и не сам себе: тот же инвариант, что у депозитов.
func handleAdminPackageIssue(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
		return
	}
	if user.Status == models.UserStatusBanned {
		packFail(c, http.StatusConflict, "banned")
		return
	}
	var req issueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		packFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = "cash"
	}
	if method != "cash" && method != "card" && method != "blik" && method != "wallet" {
		packFail(c, http.StatusBadRequest, "bad_method")
		return
	}
	pack, zone, code := livePackage(req.PackageID)
	if code != "" {
		packFail(c, http.StatusBadRequest, code)
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	up, walletAfter, code := issuePackage(user, pack, *zone, method, &adminID)
	if code != "" {
		status := http.StatusInternalServerError
		if code == "not_enough" {
			status = http.StatusConflict
		}
		packFail(c, status, code)
		return
	}
	target := user.ID
	logAdminAction(c, "package_issue", &target,
		fmt.Sprintf("%s · %s · %d мин · %.2f zł · %s", up.Name, up.ZoneName,
			up.MinutesTotal, up.PricePLN, method))
	hub.AdminBroadcast("package", map[string]any{"kind": "issue", "user_id": user.ID.String()})
	c.JSON(http.StatusCreated, gin.H{"package": userPackOut(up, time.Now()),
		"wallet_pln": models.PLNFromGrosz(walletAfter)})
}

// POST /me/packages — гость покупает пакет сам (Р12), только с кошелька.
// Живых денег в приложении нет, поэтому method здесь не спрашиваем: любой
// другой способ означал бы, что клуб получил наличные, которых никто не видел.
func handleMyPackageBuy(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.GetString("user_id")).Error; err != nil {
		packFail(c, http.StatusNotFound, "user_not_found")
		return
	}
	if user.Status == models.UserStatusBanned {
		packFail(c, http.StatusConflict, "banned")
		return
	}
	var req issueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		packFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	pack, zone, code := livePackage(req.PackageID)
	if code != "" {
		packFail(c, http.StatusBadRequest, code)
		return
	}
	up, walletAfter, code := issuePackage(&user, pack, *zone, "wallet", &user.ID)
	if code != "" {
		status := http.StatusInternalServerError
		if code == "not_enough" {
			status = http.StatusConflict
		}
		packFail(c, status, code)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"package": userPackOut(up, time.Now()),
		"wallet_pln": models.PLNFromGrosz(walletAfter)})
}

// livePackage — пакет каталога, годный к продаже прямо сейчас.
func livePackage(id string) (*models.TimePackage, *models.Zone, string) {
	if strings.TrimSpace(id) == "" {
		return nil, nil, "pack_required"
	}
	var p models.TimePackage
	if err := db.First(&p, "id = ?", id).Error; err != nil {
		return nil, nil, "pack_not_found"
	}
	if !p.Active {
		return nil, nil, "pack_inactive"
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", p.ZoneID).Error; err != nil {
		return nil, nil, "zone_not_found"
	}
	return &p, &z, ""
}

// GET /admin/users/:id/packages — пакеты гостя у стойки (staff).
func handleAdminUserPackages(c *gin.Context) {
	list, live := userPackages(c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"packages": list, "minutes_live": live})
}

// GET /me/packages — свои пакеты (шелл и PWA).
func handleMyPackages(c *gin.Context) {
	list, live := userPackages(c.GetString("user_id"))
	c.JSON(http.StatusOK, gin.H{"packages": list, "minutes_live": live})
}

// userPackages — история пакетов гостя и сумма ЖИВЫХ минут. Отменённые и
// сгоревшие остаются в списке: «куда делись мои три часа» — вопрос, на
// который должен отвечать экран, а не админ по памяти.
func userPackages(userID string) ([]gin.H, int) {
	var packs []models.UserPackage
	db.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&packs)
	now := time.Now()
	out := make([]gin.H, 0, len(packs))
	live := 0
	for i := range packs {
		out = append(out, userPackOut(&packs[i], now))
		if packageLive(&packs[i], now) {
			live += packs[i].MinutesLeft
		}
	}
	return out, live
}

// POST /admin/user-packages/:id/void — отменить ошибочную выдачу (staff).
// Правила В4-4: свою и в пределах текущих клубных суток отменяет админ,
// чужую и вчерашнюю — владелец. Иначе «отмена» становится тихим способом
// вынуть деньги из сданной смены.
//
// Забираем НЕОТЫГРАННЫЙ остаток: если часть времени гость уже отсидел,
// вернуть можно только хвост — отмена существует для свежих ошибок, а не для
// того, чтобы отнять сыгранное.
func handleAdminPackageVoid(c *gin.Context) {
	var up models.UserPackage
	if err := db.First(&up, "id = ?", c.Param("id")).Error; err != nil {
		packFail(c, http.StatusNotFound, "issue_not_found")
		return
	}
	if up.VoidedAt != nil {
		packFail(c, http.StatusConflict, "already_void")
		return
	}
	reportHour := int(settingInt64("report_hour", 8))
	from, to, _, _ := shiftWindow("", reportHour, time.Now())
	createdBy := ""
	if up.CreatedBy != nil {
		createdBy = up.CreatedBy.String()
	}
	if ok, code := canVoidSale(c.GetString("user_role"), createdBy,
		c.GetString("user_id"), up.CreatedAt, from, to); !ok {
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": goodErrors[code]})
		return
	}

	adminID, _ := uuid.Parse(c.GetString("user_id"))
	now := time.Now()
	// Деньги возвращаются только тем же путём, каким пришли: с кошелька —
	// на кошелёк. Наличные из ящика возвращает человек, и сервер не может
	// сделать вид, что он это сделал.
	refund := int64(0)
	if up.Method == "wallet" {
		refund = refundForVoid(models.GroszFromPLN(up.PricePLN), up.MinutesTotal, up.MinutesLeft)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.UserPackage{}).
			Where("id = ? AND voided_at IS NULL", up.ID).
			Updates(map[string]any{"voided_at": now, "voided_by": adminID, "minutes_left": 0})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errAlreadyVoid
		}
		if refund > 0 {
			id := up.ID
			_, werr := walletApply(tx, walletOp{
				UserID: up.UserID, Kind: models.WalletTxRefund, Amount: refund,
				RefType: "package", RefID: &id, CreatedBy: &adminID,
				Note: "отмена выдачи пакета: " + up.Name,
			})
			return werr
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errAlreadyVoid) {
			packFail(c, http.StatusConflict, "already_void")
			return
		}
		packFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	target := up.UserID
	logAdminAction(c, "package_void", &target,
		fmt.Sprintf("отмена: %s · %s · забрано %d мин из %d%s", up.Name, up.ZoneName,
			up.MinutesLeft, up.MinutesTotal, refundSuffix(refund)))
	notifyUser(up.UserID, "package_void", map[string]any{
		"name": up.Name, "minutes": up.MinutesLeft,
		"refund_pln": models.PLNFromGrosz(refund),
	})
	c.JSON(http.StatusOK, gin.H{"voided": up.ID, "minutes_taken": up.MinutesLeft,
		"refund_pln": models.PLNFromGrosz(refund)})
}

var errAlreadyVoid = errors.New("already_void")

// refundForVoid — сколько вернуть на кошелёк при отмене: доля цены за
// НЕОТЫГРАННЫЕ минуты, вниз до гроша. Чистая функция (тест). Вниз — потому
// что отыгранное время клуб уже отдал, и округление в пользу гостя тут
// означало бы платить дважды.
func refundForVoid(priceGrosz int64, total, left int) int64 {
	if total <= 0 || left <= 0 || priceGrosz <= 0 {
		return 0
	}
	if left > total {
		left = total
	}
	return priceGrosz * int64(left) / int64(total)
}

func refundSuffix(refund int64) string {
	if refund <= 0 {
		return ""
	}
	return fmt.Sprintf(", возврат %.2f zł", models.PLNFromGrosz(refund))
}

// ── Расход в биллинге (Е2-и3) ─────────────────────────────────────────
//
// Порядок расхода — «что скорее пропадёт, то первым»: монеты (тают
// еженедельно, В4-3) → минуты пакета (могут сгореть по сроку, Р11) →
// кошелёк (не тает никогда, Г-Р1). Внутри пакетов — та же логика: сперва
// тот, что сгорит раньше; бессрочные в конце.
//
// Пакет привязан к зоне (Р10), поэтому платит он только за время в СВОЕЙ
// зоне. Гостя пересадили в другую (Г2-и3) — с этой минуты пакет молчит и
// платит кошелёк: иначе часы VIP уходили бы по цене STANDARD.

// zoneOfComputer — зона машины сессии. nil — машина вне зон: пакеты к ней
// неприменимы по определению, платит кошелёк по клубному тарифу.
func zoneOfComputer(pcID uuid.UUID) *uuid.UUID {
	var pc models.Computer
	if err := db.Select("zone_id").First(&pc, "id = ?", pcID).Error; err != nil {
		return nil
	}
	return pc.ZoneID
}

// livePackagesFor — живые пакеты гостя в этой зоне, ближайший к сгоранию
// первым. Просроченные и отменённые не попадают: срок отпускает пакет сам,
// сравнением, без фонового джоба — как придержание ПК в Е1-и5.
func livePackagesFor(tx *gorm.DB, userID uuid.UUID, zoneID *uuid.UUID, now time.Time) []models.UserPackage {
	if zoneID == nil {
		return nil
	}
	var packs []models.UserPackage
	tx.Where(`user_id = ? AND zone_id = ? AND voided_at IS NULL AND minutes_left > 0
	          AND (expires_at IS NULL OR expires_at > ?)`, userID, *zoneID, now).
		Order("expires_at ASC NULLS LAST, created_at ASC").
		Find(&packs)
	return packs
}

// takePackageMinutes — снять need минут с пакетов по порядку. Возвращает,
// сколько снять удалось: пакетов могло не хватить, и остаток доплатит
// кошелёк. UPDATE идёт под условием остатка — параллельная отмена выдачи или
// вторая сессия того же гостя не уведут остаток в минус.
func takePackageMinutes(tx *gorm.DB, packs []models.UserPackage, need int) (int, error) {
	used := 0
	for i := range packs {
		if need <= 0 {
			break
		}
		take := packs[i].MinutesLeft
		if take > need {
			take = need
		}
		res := tx.Model(&models.UserPackage{}).
			Where("id = ? AND minutes_left >= ?", packs[i].ID, take).
			UpdateColumn("minutes_left", gorm.Expr("minutes_left - ?", take))
		if res.Error != nil {
			return used, res.Error
		}
		if res.RowsAffected == 0 {
			continue // остаток увели параллельно — просто идём дальше
		}
		used += take
		need -= take
	}
	return used, nil
}

// livePackMinutes — сколько минут пакетов гость может потратить в этой зоне
// прямо сейчас. Нужно и прогнозу «сколько осталось», и порогу старта: гость
// с пятичасовым пакетом и пустым кошельком обязан садиться.
func livePackMinutes(userID uuid.UUID, zoneID *uuid.UUID, now time.Time) int64 {
	if zoneID == nil {
		return 0
	}
	var sum int64
	db.Model(&models.UserPackage{}).
		Select("COALESCE(SUM(minutes_left),0)").
		Where(`user_id = ? AND zone_id = ? AND voided_at IS NULL AND minutes_left > 0
		       AND (expires_at IS NULL OR expires_at > ?)`, userID, *zoneID, now).
		Scan(&sum)
	return sum
}

// ── Предупреждение о сгорании (Е2-и5) ─────────────────────────────────
//
// Пакет со сроком (Р11) сгорает молча: гость купил три часа, забыл про них на
// две недели и обнаружил ноль. Формально честно — срок был написан при
// покупке; по-человечески это способ поссориться с гостем на ровном месте.
// Монеты в такой ситуации предупреждают (В4-3), пакет ведёт себя так же.

const packWarnDaysDef int64 = 3 // за сколько дней сказать (настройка pack_warn_days)

// packWarnDue — пора ли предупреждать. Чистая функция (тест): срок ещё не
// вышел, но до него меньше warnDays. warnDays = 0 выключает предупреждения.
func packWarnDue(expires time.Time, now time.Time, warnDays int64) bool {
	if warnDays <= 0 || !now.Before(expires) {
		return false
	}
	return expires.Sub(now) <= time.Duration(warnDays)*24*time.Hour
}

// warnPackagesExpiring — один заход: сказать тем, у кого пакет вот-вот сгорит.
// Каждому пакету — ровно одно предупреждение (`warned_at`), поэтому повторные
// прогоны и перезапуск сервера безопасны.
func warnPackagesExpiring(now time.Time) int {
	warnDays := settingInt64("pack_warn_days", packWarnDaysDef)
	if warnDays <= 0 {
		return 0
	}
	var packs []models.UserPackage
	db.Where(`voided_at IS NULL AND warned_at IS NULL AND minutes_left > 0
	          AND expires_at IS NOT NULL AND expires_at > ?`, now).Find(&packs)

	sent := 0
	for i := range packs {
		p := packs[i]
		if !packWarnDue(*p.ExpiresAt, now, warnDays) {
			continue
		}
		// Метку ставим ПЕРВОЙ и под условием: два прогона подряд (или два
		// сервера) не должны прислать гостю одно и то же дважды.
		res := db.Model(&models.UserPackage{}).
			Where("id = ? AND warned_at IS NULL", p.ID).
			UpdateColumn("warned_at", now)
		if res.Error != nil || res.RowsAffected == 0 {
			continue
		}
		notifyUser(p.UserID, "package_expiring", map[string]any{
			"name": p.Name, "zone": p.ZoneName, "minutes": p.MinutesLeft,
			"days_left": daysLeft(*p.ExpiresAt, now), "expires_at": p.ExpiresAt,
		})
		sent++
	}
	return sent
}

// GET /admin/packages/expiring — кого предупредит ближайший прогон (owner).
// Тот же вопрос, что у монет («кто потеряет сейчас», В4-3): владелец должен
// видеть, что случится, до того как оно случится.
func handleAdminPackagesExpiring(c *gin.Context) {
	now := time.Now()
	warnDays := settingInt64("pack_warn_days", packWarnDaysDef)
	var packs []models.UserPackage
	db.Where(`voided_at IS NULL AND minutes_left > 0
	          AND expires_at IS NOT NULL AND expires_at > ?`, now).
		Order("expires_at ASC").Find(&packs)
	out := make([]gin.H, 0, len(packs))
	for i := range packs {
		if !packWarnDue(*packs[i].ExpiresAt, now, warnDays) {
			continue
		}
		row := userPackOut(&packs[i], now)
		row["user_id"] = packs[i].UserID
		row["warned"] = packs[i].WarnedAt != nil
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"warn_days": warnDays, "count": len(out), "packages": out})
}

// POST /admin/packages/warn — предупредить сейчас, не дожидаясь фонового
// прогона (owner). Ровно то же, что делает джоб: одна логика, две двери —
// иначе кнопка и фон однажды разъедутся.
func handleAdminPackagesWarnRun(c *gin.Context) {
	n := warnPackagesExpiring(time.Now())
	if n > 0 {
		logAdminAction(c, "package_warn", nil, fmt.Sprintf("предупреждено о сгорании: %d", n))
	}
	c.JSON(http.StatusOK, gin.H{"warned": n})
}

// startPackageWarnJob — фоновый прогон раз в 6 часов, первый через минуту
// после старта. Чаще незачем: речь о днях, а не о минутах.
func startPackageWarnJob() {
	go func() {
		time.Sleep(time.Minute)
		for {
			if n := warnPackagesExpiring(time.Now()); n > 0 {
				log.Printf("пакеты: предупреждено о сгорании — %d штук", n)
			}
			time.Sleep(6 * time.Hour)
		}
	}()
}
