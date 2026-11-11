package main

// З13-и6 «Профили учёта» (OWNER.md, трек З; №76; решения основателя 04.10, З-Р37 — вопросы бухгалтеру —
// настройки владельца). Здесь — справочники форм учёта для экрана (З13-и7) и реквизиты фирмы для «О выгрузке»:
//
//	GET /admin/ryczalt/rates — ставки ryczałtu с подписями и значения по умолчанию (позиция — 3 %, время — 15 %);
//	PUT /admin/ryczalt/goods — ставка ryczałtu всей категории позиций одной кнопкой, как у VAT;
//	GET /admin/kpir/columns — колонки KPiR, куда может идти расход категории (12, 13, 14, 15 или не в книгу);
//	GET/PUT /admin/export/requisites — название фирмы, NIP и адрес для бухгалтера; не задано — название и адрес клуба.
//
// Колонка KPiR и счёт категории — в её ручке (expense_refs.go), ставка ryczałtu позиции — в ручке позиции
// (goods.go), форма учёта и ставка времени — настройки acc_form и acc_ryczalt_time. Всё — только владельцу.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ryczaltBadMessage — ответ на ставку не из списка: «Ставка ryczałtu — одна из: 17 %, 15 %, …, 2 %».
var ryczaltBadMessage = "Ставка ryczałtu — одна из: " + ryczaltRatesText()

func ryczaltRatesText() string {
	parts := make([]string, 0, len(models.RyczaltRates))
	for _, r := range models.RyczaltRates {
		parts = append(parts, models.RyczaltLabel(r))
	}
	return strings.Join(parts, ", ")
}

// ryczaltFromRequest — ставка из запроса: не прислана — def; не из списка — false (чистая, тест).
func ryczaltFromRequest(v *int, def int) (int, bool) {
	if v == nil {
		return def, true
	}
	return *v, models.ValidRyczalt(*v)
}

// ryczaltTimeNow — ставка ryczałtu времени из настроек; строку в базе руками не из списка — как по умолчанию.
func ryczaltTimeNow() int {
	if v := int(exportSetting("acc_ryczalt_time")); models.ValidRyczalt(v) {
		return v
	}
	return models.RyczaltTimeDefault
}

// kpirNames — колонки KPiR для людей: [русский, польский].
var kpirNames = map[int][2]string{
	models.KpirGoods: {"закупка товаров и материалов", "zakup towarów handlowych i materiałów"},
	models.KpirSide:  {"побочные затраты закупки", "koszty uboczne zakupu"},
	models.KpirWages: {"зарплаты", "wynagrodzenia w gotówce i w naturze"},
	models.KpirOther: {"прочие расходы", "pozostałe wydatki"},
	models.KpirNone:  {"не в книгу", "poza księgą"},
}

// kpirColsText — колонки для ответа на чужую: «12, 13, 14, 15, 0 — не в книгу» (чистая, тест).
func kpirColsText() string {
	parts := make([]string, 0, len(models.KpirCols))
	for _, col := range models.KpirCols {
		if col == models.KpirNone {
			parts = append(parts, "0 — "+kpirNames[col][0])
		} else {
			parts = append(parts, strconv.Itoa(col))
		}
	}
	return strings.Join(parts, ", ")
}

// kpirText — колонка для журнала действий: «12», «не в книгу» (чистая, тест).
func kpirText(col int) string {
	if col == models.KpirNone {
		return kpirNames[models.KpirNone][0]
	}
	return strconv.Itoa(col)
}

// GET /admin/ryczalt/rates — ставки ryczałtu с подписями (owner): экран строит выбор по ним.
func handleRyczaltRates(c *gin.Context) {
	out := make([]gin.H, 0, len(models.RyczaltRates))
	for _, r := range models.RyczaltRates {
		out = append(out, gin.H{"rate": r, "label": models.RyczaltLabel(r)})
	}
	c.JSON(http.StatusOK, gin.H{"rates": out, "default_goods": models.RyczaltGoodsDefault, "default_time": models.RyczaltTimeDefault})
}

// GET /admin/kpir/columns — колонки KPiR для категории расхода (owner): номер по образцу книги с 2026 года и подписи.
func handleKpirColumns(c *gin.Context) {
	out := make([]gin.H, 0, len(models.KpirCols))
	for _, col := range models.KpirCols {
		out = append(out, gin.H{"col": col, "name_ru": kpirNames[col][0], "name_pl": kpirNames[col][1]})
	}
	c.JSON(http.StatusOK, gin.H{"columns": out, "default_purchase": models.KpirDefault(true), "default": models.KpirDefault(false)})
}

type goodsRyczaltRequest struct {
	Category    *string `json:"category"` // "" — позиции без категории
	RyczaltRate *int    `json:"ryczalt_rate"`
}

// PUT /admin/ryczalt/goods — ставка ryczałtu всей категории позиций одной кнопкой (owner): бар — 3 %, пиво — 8,5 %.
// Ставка позиции действует и на прошлые периоды выгрузки: снимка у ryczałtu нет — его задаёт вид деятельности.
func handleGoodsRyczalt(c *gin.Context) {
	var req goodsRyczaltRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Category == nil || req.RyczaltRate == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны категория и ставка"})
		return
	}
	if !models.ValidRyczalt(*req.RyczaltRate) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_ryczalt", "message": ryczaltBadMessage})
		return
	}
	cat := strings.TrimSpace(*req.Category)
	res := db.Model(&models.Good{}).Where("category = ? AND ryczalt_rate <> ?", cat, *req.RyczaltRate).Update("ryczalt_rate", *req.RyczaltRate)
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
		logAdminAction(c, "goods_ryczalt", nil, fmt.Sprintf("категория «%s»: ryczałt → %s (%d %s)",
			name, models.RyczaltLabel(*req.RyczaltRate), n, plural(n, "позиция", "позиции", "позиций")))
	}
	c.JSON(http.StatusOK, gin.H{"category": cat, "ryczalt_rate": *req.RyczaltRate, "changed": res.RowsAffected, "total": total})
}

// ── реквизиты фирмы ─────────────────────────────────────────────────────────────────────────────

// Реквизиты — строковые настройки: фирма может называться не так, как клуб, и быть зарегистрирована по другому
// адресу; пусто — в «О выгрузке» название и адрес клуба.
const (
	keyFirmName    = "acc_firm_name"
	keyFirmNIP     = "acc_firm_nip"
	keyFirmAddress = "acc_firm_address"
	maxFirmName    = 200
	maxFirmAddress = 300
)

type requisites struct {
	Name, NIP, Address string
}

func requisitesNow() requisites {
	return requisites{Name: settingString(keyFirmName, ""), NIP: settingString(keyFirmNIP, ""), Address: settingString(keyFirmAddress, "")}
}

// firmOf — что писать в «О выгрузке» (чистая, тест): реквизиты, а чего нет — название и адрес клуба.
func firmOf(r requisites, club models.Club) (name, address string) {
	name, address = r.Name, r.Address
	if name == "" {
		name = club.Name
	}
	if address == "" {
		address = club.Address
	}
	return name, address
}

func requisitesOut() gin.H {
	r := requisitesNow()
	var club models.Club
	db.Where("is_active = ?", true).Order("name").First(&club) // З19-и3: тот же «первый активный клуб», что defaultClub()
	return gin.H{"name": r.Name, "nip": r.NIP, "address": r.Address, "club_name": club.Name, "club_address": club.Address}
}

// GET /admin/export/requisites — реквизиты фирмы для выгрузки (owner); club_* — что пойдёт вместо пустых.
func handleRequisites(c *gin.Context) {
	c.JSON(http.StatusOK, requisitesOut())
}

// PUT /admin/export/requisites {name?, nip?, address?} — реквизиты фирмы (owner): название до 200 знаков, адрес до
// 300, NIP — 10 цифр с контрольной суммой (дефисы и пробелы — можно), "" — убрать.
func handleRequisitesPut(c *gin.Context) {
	var req struct {
		Name    *string `json:"name"`
		NIP     *string `json:"nip"`
		Address *string `json:"address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны название, NIP или адрес"})
		return
	}
	was := requisitesNow()
	now := was
	if req.Name != nil {
		now.Name = strings.TrimSpace(*req.Name)
		if len([]rune(now.Name)) > maxFirmName {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_firm_name", "message": fmt.Sprintf("Название фирмы — до %d символов", maxFirmName)})
			return
		}
	}
	if req.Address != nil {
		now.Address = strings.TrimSpace(*req.Address)
		if len([]rune(now.Address)) > maxFirmAddress {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_firm_address", "message": fmt.Sprintf("Адрес фирмы — до %d символов", maxFirmAddress)})
			return
		}
	}
	if req.NIP != nil {
		nip, code := checkNIP(*req.NIP)
		if code != "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": expenseErrors[code]})
			return
		}
		now.NIP = nip
	}
	changes, rows := []string{}, []models.Setting{}
	for _, f := range []struct {
		key, label, old, new string
	}{{keyFirmName, "фирма", was.Name, now.Name}, {keyFirmNIP, "NIP", was.NIP, now.NIP}, {keyFirmAddress, "адрес", was.Address, now.Address}} {
		if f.old != f.new {
			rows = append(rows, models.Setting{Key: f.key, Value: f.new})
			changes = append(changes, f.label+": "+dash(f.old)+" → "+dash(f.new))
		}
	}
	if len(rows) > 0 {
		err := db.Transaction(func(tx *gorm.DB) error { // все поля — одной транзакцией, как PUT /admin/settings
			for i := range rows {
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}},
					DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&rows[i]).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "Не сохранилось — попробуй ещё раз"})
			return
		}
		logAdminAction(c, "requisites", nil, strings.Join(changes, "; "))
	}
	out := requisitesOut()
	out["changed"] = len(changes)
	c.JSON(http.StatusOK, out)
}
