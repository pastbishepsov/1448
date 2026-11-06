package main

// Каталог приложений гостевого экрана (ТЗ 6.2: конфигурируется через Admin Panel).
// GET /catalog — публичный (гостевой экран и shell-agent читают без токена; пути
// запуска в LAN клуба не секрет).
// Права (Б0-и3, решение №2 в ADMIN.md): admin — вкл/выкл и порядок;
// создание/правка/удаление/импорт/обложки/корень игр (target/args = что
// запускается на гостевых ПК) — owner.
// До пилота агент получит собственную аутентификацию (MAC+токен, см. STATUS).
//
// Пакет П1 (docs/КАТАЛОГ-ИГР.md) — каталог наполняет владелец, без разработчика:
//   • обложка у каждой записи: ссылка (cover_url) или файл, загруженный в БД;
//   • target вида {GAMES}\CS2\cs2.exe — сервер подставляет корень игр клуба
//     (настройка games_root) при отдаче /catalog;
//   • импорт/экспорт JSON — 50 игр заводятся пачкой и переносятся между клубами.
//
// Статические ручки (/catalog/import, /catalog/config, /catalog/export) стоят
// рядом с /catalog/:id — gin 1.10 разводит их правильно, включая возврат к
// :id, если статика не совпала целиком; закреплено тестом
// TestCatalogRoutesRegisterAndMatch, чтобы апгрейд gin не сломал это молча.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	gamesRootKey     = "games_root" // настройка клуба: корень сетевого диска с играми
	gamesPlaceholder = "{GAMES}"    // подстановка в target/args
	maxCoverBytes    = 500 << 10    // обложка ≤500КБ (клиент жмёт до ~512px, как фото кухни)
	maxCoverURLLen   = 500
	maxTargetLen     = 500
	maxImportApps    = 500     // записей за один импорт
	maxImportBytes   = 1 << 20 // тело импорта ≤1 МБ — общий потолок maxRequestBody (З18-и2: было «4 МБ»)
	catalogExportVer = 1       // версия формата импорта/экспорта
)

var (
	catalogIDRe    = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)
	catalogColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
	// Корень игр: буква диска (G:\Games), UNC (\\nas\games) или POSIX (/mnt/games).
	gamesRootRe = regexp.MustCompile(`^([A-Za-z]:[\\/]|\\\\[^\\/]+[\\/]|/)[^<>"|?*{}]*$`)
)

// catalogFields — набор полей записи каталога. Одинаково приходит из формы
// админки (upsert) и из JSON-импорта, поэтому и проверяется одной функцией.
// Указатели различают «поле не прислали» и «прислали пустым»: импорт правит
// только присланное, форма — перезаписывает запись целиком.
type catalogFields struct {
	ID       string          `json:"id"`
	Name     *string         `json:"name"`
	Category *string         `json:"category"`
	Subtitle *string         `json:"subtitle"`
	Tag      *string         `json:"tag"`
	Emoji    *string         `json:"emoji"`
	ColorA   *string         `json:"color_a"`
	ColorB   *string         `json:"color_b"`
	Target   *string         `json:"target"`
	Args     json.RawMessage `json:"args"`
	CoverURL *string         `json:"cover_url"`
	Sort     *int            `json:"sort"`
	Enabled  *bool           `json:"enabled"`
}

// ── Чистые проверки и подстановки (тестируются без БД) ──────────────────────

// validateCatalogApp — обязательный минимум записи (id/название/категория).
func validateCatalogApp(id, name, category string) (ok bool, code string) {
	if !catalogIDRe.MatchString(id) {
		return false, "bad_id"
	}
	// длину считаем в символах, а не в байтах: колонка в БД — VARCHAR(64),
	// то есть 64 символа, а len() резал русские названия вдвое раньше срока
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return false, "bad_name"
	}
	if category != "game" && category != "app" && category != "system" && category != "platform" {
		return false, "bad_category"
	}
	return true, ""
}

// validateCoverURL — ссылка на обложку: внешняя http(s) или свой путь от корня
// (/covers/cs2.jpg из репо, /api/v1/catalog/<id>/cover). Схемы data:/javascript:
// не пускаем: картинку вставляет гостевой экран, туда нельзя произвольный URL.
func validateCoverURL(u string) (ok bool, msg string) {
	u = strings.TrimSpace(u)
	if u == "" {
		return true, ""
	}
	if utf8.RuneCountInString(u) > maxCoverURLLen {
		return false, fmt.Sprintf("Ссылка на обложку — до %d символов", maxCoverURLLen)
	}
	if strings.ContainsAny(u, " \t\r\n\"'<>") {
		return false, "Ссылка на обложку: пробелы и кавычки недопустимы"
	}
	low := strings.ToLower(u)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return true, ""
	}
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return true, ""
	}
	return false, "Ссылка на обложку: http(s)://… или путь от корня (/covers/cs2.jpg)"
}

// validateGamesRoot — корень игр клуба. Пусто = подстановка выключена.
func validateGamesRoot(root string) (ok bool, msg string) {
	if root == "" {
		return true, ""
	}
	if utf8.RuneCountInString(root) > 200 {
		return false, "Корень игр — до 200 символов"
	}
	if strings.ContainsAny(root, "\x00\n\r\t\"") {
		return false, "Корень игр: недопустимые символы"
	}
	if !gamesRootRe.MatchString(root) {
		return false, `Корень игр: «G:\Games», «\\nas\games» или «/mnt/games»`
	}
	return true, ""
}

// expandGames — подстановка {GAMES} в путь запуска. Хвостовой слэш в настройке
// срезаем: «G:\Games\» + «\CS2\cs2.exe» дало бы «G:\Games\\CS2\cs2.exe».
// Корень не задан — оставляем шаблон видимым: пусть агент честно не найдёт
// файл и это будет заметно, чем молча собрать битый путь.
func expandGames(target, root string) string {
	if target == "" || root == "" || !strings.Contains(target, gamesPlaceholder) {
		return target
	}
	return strings.ReplaceAll(target, gamesPlaceholder, strings.TrimRight(root, `\/`))
}

// expandGamesArgs — та же подстановка внутри JSON-массива аргументов. Корень
// попадает в строковый литерал JSON, поэтому обратные слэши экранируем:
// без этого «["--dir=G:\Games"]» перестанет разбираться у агента.
func expandGamesArgs(args, root string) string {
	if args == "" || root == "" || !strings.Contains(args, gamesPlaceholder) {
		return args
	}
	esc := strings.ReplaceAll(strings.TrimRight(root, `\/`), `\`, `\\`)
	return strings.ReplaceAll(args, gamesPlaceholder, esc)
}

// normalizeArgs — args приходят двумя способами: настоящим JSON-массивом (так
// пишет экспорт и так удобно руками) и строкой с JSON внутри (так хранит
// колонка jsonb и так шлёт форма админки). Наружу — текст для БД.
func normalizeArgs(raw json.RawMessage) (val *string, ok bool, msg string) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return nil, true, ""
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return nil, false, "args: строка не разобралась"
		}
		if strings.TrimSpace(s) == "" {
			return nil, true, ""
		}
		t = []byte(strings.TrimSpace(s))
	}
	var arr []string
	if err := json.Unmarshal(t, &arr); err != nil {
		return nil, false, `args: JSON-массив строк, например ["--flag=1"]`
	}
	if len(arr) == 0 {
		return nil, true, ""
	}
	out, err := json.Marshal(arr)
	if err != nil {
		return nil, false, "args: не сериализуется"
	}
	s := string(out)
	return &s, true, ""
}

// validateCatalogFields — проверка присланных полей. full=true (форма админки,
// новая запись импорта) требует id/название/категорию; для правки существующей
// записи импортом проверяется только то, что прислали.
func validateCatalogFields(f *catalogFields, full bool) (code, msg string) {
	if !catalogIDRe.MatchString(f.ID) {
		return "bad_id", "id: строчные латинские буквы/цифры/_, 2–32 символа"
	}
	if full {
		name, cat := "", ""
		if f.Name != nil {
			name = strings.TrimSpace(*f.Name)
		}
		if f.Category != nil {
			cat = strings.TrimSpace(*f.Category)
		}
		if ok, code := validateCatalogApp(f.ID, name, cat); !ok {
			return code, map[string]string{
				"bad_id":       "id: строчные латинские буквы/цифры/_, 2–32 символа",
				"bad_name":     "Название: 1–64 символа",
				"bad_category": "category: game | app | system | platform",
			}[code]
		}
	} else {
		if f.Name != nil && (strings.TrimSpace(*f.Name) == "" || utf8.RuneCountInString(*f.Name) > 64) {
			return "bad_name", "Название: 1–64 символа"
		}
		if f.Category != nil {
			switch strings.TrimSpace(*f.Category) {
			case "game", "app", "system", "platform":
			default:
				return "bad_category", "category: game | app | system | platform"
			}
		}
	}
	lim := func(v *string, n int, code, msg string) (string, string) {
		if v != nil && utf8.RuneCountInString(*v) > n {
			return code, msg
		}
		return "", ""
	}
	if c, m := lim(f.Subtitle, 64, "bad_subtitle", "Подпись — до 64 символов"); c != "" {
		return c, m
	}
	if c, m := lim(f.Tag, 12, "bad_tag", "TAG на тайле — до 12 символов"); c != "" {
		return c, m
	}
	if c, m := lim(f.Emoji, 8, "bad_emoji", "Эмодзи — до 8 символов"); c != "" {
		return c, m
	}
	if c, m := lim(f.Target, maxTargetLen, "bad_target",
		fmt.Sprintf("target — до %d символов", maxTargetLen)); c != "" {
		return c, m
	}
	for _, col := range []*string{f.ColorA, f.ColorB} {
		if col != nil && strings.TrimSpace(*col) != "" && !catalogColorRe.MatchString(strings.TrimSpace(*col)) {
			return "bad_color", "Цвет — #rrggbb (можно #rrggbbaa)"
		}
	}
	if f.CoverURL != nil {
		if ok, m := validateCoverURL(*f.CoverURL); !ok {
			return "bad_cover", m
		}
	}
	if !validSort(f.Sort) {
		return "bad_sort", sortMessage
	}
	if _, ok, m := normalizeArgs(f.Args); !ok {
		return "bad_args", m
	}
	return "", ""
}

// ── Обложка и подстановка при отдаче ────────────────────────────────────────

// catalogCoverURL — что показать витрине: загруженная в БД обложка приоритетнее
// ссылки, введённой руками (владелец загрузил файл — значит, им и заменил).
func catalogCoverURL(a *models.CatalogApp) string {
	if a.CoverAt != nil {
		return fmt.Sprintf("/api/v1/catalog/%s/cover?v=%d", a.ID, a.CoverAt.Unix())
	}
	if a.CoverURL != nil {
		return strings.TrimSpace(*a.CoverURL)
	}
	return ""
}

// decorateCatalogApp — то, что считается на лету: обложка и подстановка корня
// игр. public=false для админки: там показываем и правим исходное — шаблон
// {GAMES} в target и ту ссылку на обложку, которую ввёл владелец.
func decorateCatalogApp(a *models.CatalogApp, root string, public bool) {
	a.Cover = catalogCoverURL(a)
	if !public {
		return
	}
	// В публичной выдаче cover_url = cover: витрине незачем знать, что было
	// ссылкой, а что загруженным файлом. Иначе клиент, читающий поля в другом
	// порядке, показал бы старую ссылку вместо только что загруженной обложки
	// (web/shell.html берёт `cover_url || cover`).
	if a.Cover == "" {
		a.CoverURL = nil
	} else {
		cv := a.Cover
		a.CoverURL = &cv
	}
	if a.Target != nil && strings.Contains(*a.Target, gamesPlaceholder) {
		a.TargetRaw = *a.Target
		t := expandGames(*a.Target, root)
		a.Target = &t
	}
	if a.Args != nil && strings.Contains(*a.Args, gamesPlaceholder) {
		s := expandGamesArgs(*a.Args, root)
		a.Args = &s
	}
}

// GET /catalog — включённые приложения по категориям (для экрана и агента).
// target отдаётся уже с подставленным корнем игр; исходный шаблон — в
// target_raw, а сам корень — полем games_root, чтобы агент при желании мог
// собрать путь по-своему (per-PC буква диска, пакет П2).
//
// К4-и3: полная выдача — только агенту: GET /catalog?full=1 с токеном. Без
// токена (гостевой экран, кто угодно с адресом сервера) пути на диске,
// аргументы, target_raw и games_root вырезаются; ссылки и протоколы
// (steam://…, https://…) остаются — по ним киоск запускает без агента.
// ?full=1 без верного токена — 401, а не «тихо урезанный» ответ.
func handleGetCatalog(c *gin.Context) {
	full := c.Query("full") == "1"
	if full && !agentAuthorized(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "agent_token_invalid", "message": "Полный каталог — только агенту с токеном"})
		return
	}
	var apps []models.CatalogApp
	if err := db.Where("enabled = ?", true).Order("sort").Order("name").Find(&apps).Error; err != nil {
		respondDBError(c, err)
		return
	}
	root := settingString(gamesRootKey, "")
	games, appsList, system, platforms := []models.CatalogApp{}, []models.CatalogApp{}, []models.CatalogApp{}, []models.CatalogApp{}
	for i := range apps {
		a := apps[i]
		decorateCatalogApp(&a, root, true)
		if !full {
			stripCatalogSecrets(&a)
		}
		switch a.Category {
		case "game":
			games = append(games, a)
		case "app":
			appsList = append(appsList, a)
		case "system":
			system = append(system, a)
		case "platform":
			platforms = append(platforms, a)
		}
	}
	out := gin.H{"games": games, "apps": appsList, "system": system, "platforms": platforms}
	if full {
		out["games_root"] = root
	}
	c.JSON(http.StatusOK, out)
}

// stripCatalogSecrets — публичная выдача без того, что нужно только агенту
// (К4-и3): путь на диске (C:\…, \\nas\…), аргументы, исходный шаблон.
// Ссылки и протоколы остаются — их и так видно в адресной строке.
func stripCatalogSecrets(a *models.CatalogApp) {
	if a.Target != nil && isDiskPath(*a.Target) {
		a.Target = nil
	}
	a.Args = nil
	a.TargetRaw = ""
}

// isDiskPath — путь Windows (буква диска или UNC) либо шаблон {GAMES}
// (чистая, тест). Всё остальное — протокол/ссылка/имя команды.
func isDiskPath(t string) bool {
	t = strings.TrimSpace(t)
	if strings.Contains(t, gamesPlaceholder) || strings.HasPrefix(t, `\\`) {
		return true
	}
	return len(t) >= 3 && t[1] == ':' && (t[2] == '\\' || t[2] == '/') &&
		((t[0] >= 'a' && t[0] <= 'z') || (t[0] >= 'A' && t[0] <= 'Z'))
}

// GET /admin/catalog — все приложения, включая выключенные. Без подстановки:
// админка правит и показывает исходный шаблон {GAMES}\…
func handleAdminCatalog(c *gin.Context) {
	var apps []models.CatalogApp
	db.Order("category").Order("sort").Order("name").Find(&apps)
	root := settingString(gamesRootKey, "")
	for i := range apps {
		decorateCatalogApp(&apps[i], root, false)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(apps), "apps": apps, "games_root": root})
}

// POST /admin/catalog — создать или обновить приложение (upsert по id).
// Форма присылает запись целиком: пустое поле = очистить.
func handleAdminCatalogUpsert(c *gin.Context) {
	var req catalogFields
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if code, msg := validateCatalogFields(&req, true); code != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}
	args, _, _ := normalizeArgs(req.Args)

	app := models.CatalogApp{
		ID: req.ID, Name: strings.TrimSpace(*req.Name), Category: strings.TrimSpace(*req.Category),
		Subtitle: trimPtr(req.Subtitle), Tag: trimPtr(req.Tag), Emoji: trimPtr(req.Emoji),
		ColorA: trimPtr(req.ColorA), ColorB: trimPtr(req.ColorB),
		Target: trimPtr(req.Target), Args: args, CoverURL: trimPtr(req.CoverURL),
		Sort: 100, Enabled: true, UpdatedAt: time.Now(),
	}
	if req.Sort != nil {
		app.Sort = *req.Sort
	}

	// upsert: сохранить как новый или обновить существующий. Колонки перечислены
	// явно — enabled остаётся за тумблером (Б0-и3), а загруженная обложка
	// (cover/cover_type/cover_at) переживает правку формы.
	var existing models.CatalogApp
	var was *models.CatalogApp // З18-и2: запись до правки — для «было → стало» в журнале
	if db.First(&existing, "id = ?", req.ID).Error == nil {
		prev := existing
		was = &prev
	}
	details := app.ID
	if was == nil {
		details += " (новая)"
	}
	if d := catalogCommandDiff(formFields(&req), was); len(d) > 0 && was != nil {
		details += ": " + cmdDiffText(d)
	} else if len(d) > 0 {
		details += ": " + cmdSetText(d)
	}
	if was != nil {
		app.Enabled = existing.Enabled
		if err := db.Model(&existing).Select("name", "category", "subtitle", "tag", "emoji",
			"color_a", "color_b", "target", "args", "cover_url", "sort", "updated_at").
			Updates(app).Error; err != nil {
			respondDBError(c, err)
			return
		}
	} else if err := db.Create(&app).Error; err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "catalog_upsert", nil, details)
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "ok": true})
}

// formFields — поля формы как поля импорта: форма присылает запись целиком, и не присланная (null) команда или
// обложка — значит «очистить», а у импорта «не трогать». Копия: сама форма не меняется.
func formFields(req *catalogFields) *catalogFields {
	f := *req
	empty := ""
	if f.Target == nil {
		f.Target = &empty
	}
	if f.CoverURL == nil {
		f.CoverURL = &empty
	}
	if len(bytes.TrimSpace(f.Args)) == 0 {
		f.Args = json.RawMessage("null")
	}
	return &f
}

// trimPtr — обрезать пробелы; пустая строка = NULL в БД.
func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// POST /admin/catalog/:id/toggle — включить/выключить приложение.
func handleAdminCatalogToggle(c *gin.Context) {
	var app models.CatalogApp
	if err := db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	newEnabled := !app.Enabled
	if err := db.Model(&app).Update("enabled", newEnabled).Error; err != nil {
		respondDBError(c, err)
		return
	}
	state := "выключено"
	if newEnabled {
		state = "включено"
	}
	logAdminAction(c, "catalog_toggle", nil, app.ID+" — "+state)
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "enabled": newEnabled})
}

// POST /admin/catalog/:id/sort — только порядок (Б0-и3). Единственная
// правка каталога, доступная роли admin помимо тумблера; формы — у owner.
func handleAdminCatalogSort(c *gin.Context) {
	var app models.CatalogApp
	if err := db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	var req struct {
		Sort *int `json:"sort" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужно поле sort (целое)"})
		return
	}
	if !validSort(req.Sort) { // З6-и3: 3·10⁹ падало на int4 базы ошибкой сервера
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_sort", "message": sortMessage})
		return
	}
	if err := db.Model(&app).Update("sort", *req.Sort).Error; err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "catalog_sort", nil, fmt.Sprintf("%s → %d", app.ID, *req.Sort))
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "sort": *req.Sort})
}

// DELETE /admin/catalog/:id — удалить приложение из каталога (owner).
func handleAdminCatalogDelete(c *gin.Context) {
	res := db.Delete(&models.CatalogApp{}, "id = ?", c.Param("id"))
	if res.Error != nil {
		respondDBError(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	logAdminAction(c, "catalog_delete", nil, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "deleted": true})
}

// ── Обложка: BYTEA в БД, байты живут только в этих трёх ручках ──────────────
// (как фото позиции кухни, Г7: папки для загрузок нет ни в клубе — /web
// примонтирован read-only, ни на Railway — эфемерная ФС.)

// GET /catalog/:id/cover — публичная отдача обложки.
func handleCatalogCover(c *gin.Context) {
	var row struct {
		Cover     []byte
		CoverType *string
		CoverAt   *time.Time
	}
	if err := db.Raw("SELECT cover, cover_type, cover_at FROM catalog_apps WHERE id = ?", c.Param("id")).
		Scan(&row).Error; err != nil || len(row.Cover) == 0 || row.CoverAt == nil {
		c.Status(http.StatusNotFound)
		return
	}
	etag := fmt.Sprintf(`"c%d"`, row.CoverAt.Unix())
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	ct := "image/jpeg"
	if row.CoverType != nil && *row.CoverType != "" {
		ct = *row.CoverType
	}
	c.Data(http.StatusOK, ct, row.Cover)
}

// PUT /admin/catalog/:id/cover — владелец кладёт обложку (сырое тело запроса,
// Content-Type = image/jpeg|png|webp), один в один как фото позиции кухни.
func handleAdminCatalogCoverPut(c *gin.Context) {
	var app models.CatalogApp
	if err := db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	ct := c.ContentType()
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_cover", "message": "Обложка — JPEG, PNG или WebP"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCoverBytes+1))
	if err != nil || len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_cover", "message": "Пустое тело запроса"})
		return
	}
	if len(data) > maxCoverBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "cover_too_big",
			"message": fmt.Sprintf("Обложка — до %d КБ (сожми на клиенте)", maxCoverBytes>>10)})
		return
	}
	if err := db.Exec("UPDATE catalog_apps SET cover = ?, cover_type = ?, cover_at = now(), updated_at = now() WHERE id = ?",
		data, ct, app.ID).Error; err != nil {
		respondDBError(c, err)
		return
	}
	// перечитываем в ЧИСТУЮ структуру: GORM не сбрасывает уже заполненные
	// поля-указатели при NULL в колонке, и ответ соврал бы про обложку
	var fresh models.CatalogApp
	db.First(&fresh, "id = ?", app.ID)
	logAdminAction(c, "catalog_cover", nil, app.ID+": обложка обновлена")
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "cover": catalogCoverURL(&fresh)})
}

// DELETE /admin/catalog/:id/cover — убрать загруженную обложку (ссылка остаётся).
func handleAdminCatalogCoverDelete(c *gin.Context) {
	var app models.CatalogApp
	if err := db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "app_not_found", "message": "Приложение не найдено"})
		return
	}
	if err := db.Exec("UPDATE catalog_apps SET cover = NULL, cover_type = NULL, cover_at = NULL, updated_at = now() WHERE id = ?",
		app.ID).Error; err != nil {
		respondDBError(c, err)
		return
	}
	var fresh models.CatalogApp
	db.First(&fresh, "id = ?", app.ID)
	logAdminAction(c, "catalog_cover", nil, app.ID+": обложка убрана")
	c.JSON(http.StatusOK, gin.H{"id": app.ID, "cover": catalogCoverURL(&fresh), "deleted": true})
}

// ── Корень игр ──────────────────────────────────────────────────────────────

// GET /admin/catalog/config — корень игр и сколько записей на него завязано.
func handleAdminCatalogConfigGet(c *gin.Context) {
	var uses int64
	db.Model(&models.CatalogApp{}).
		Where("target LIKE ? OR args LIKE ?", "%"+gamesPlaceholder+"%", "%"+gamesPlaceholder+"%").
		Count(&uses)
	c.JSON(http.StatusOK, gin.H{
		"games_root":  settingString(gamesRootKey, ""),
		"placeholder": gamesPlaceholder,
		"apps_using":  uses,
	})
}

// PUT /admin/catalog/config — сменить корень игр (owner). Одна правка вместо
// переписывания всех target при смене буквы диска или адреса NAS.
func handleAdminCatalogConfigPut(c *gin.Context) {
	var req struct {
		GamesRoot *string `json:"games_root"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.GamesRoot == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужно поле games_root"})
		return
	}
	root := strings.TrimSpace(*req.GamesRoot)
	if ok, msg := validateGamesRoot(root); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_games_root", "message": msg})
		return
	}
	old := settingString(gamesRootKey, "")
	if err := settingSetString(gamesRootKey, root); err != nil {
		respondDBError(c, err)
		return
	}
	if old != root {
		logAdminAction(c, "catalog_config", nil, fmt.Sprintf("games_root: %q → %q", old, root))
	}
	c.JSON(http.StatusOK, gin.H{"games_root": root})
}

// ── Импорт и экспорт ────────────────────────────────────────────────────────

// GET /admin/catalog/export — весь каталог в формате импорта (owner).
func handleAdminCatalogExport(c *gin.Context) {
	var apps []models.CatalogApp
	db.Order("category").Order("sort").Order("name").Find(&apps)
	items := make([]gin.H, 0, len(apps))
	for i := range apps {
		a := &apps[i]
		it := gin.H{"id": a.ID, "name": a.Name, "category": a.Category, "sort": a.Sort, "enabled": a.Enabled}
		put := func(k string, v *string) {
			if v != nil && *v != "" {
				it[k] = *v
			}
		}
		put("subtitle", a.Subtitle)
		put("tag", a.Tag)
		put("emoji", a.Emoji)
		put("color_a", a.ColorA)
		put("color_b", a.ColorB)
		put("target", a.Target)
		put("cover_url", a.CoverURL)
		if a.Args != nil && *a.Args != "" {
			// args — настоящим массивом, чтобы файл читался и правился глазами.
			var arr []string
			if json.Unmarshal([]byte(*a.Args), &arr) == nil {
				it["args"] = arr
			} else {
				it["args"] = *a.Args
			}
		}
		if a.CoverAt != nil {
			// Загруженные байты в файл не кладём: обложка останется в своём
			// клубе, в другом её надо загрузить заново или дать ссылку.
			it["cover_uploaded"] = true
		}
		items = append(items, it)
	}
	c.JSON(http.StatusOK, gin.H{
		"version":     catalogExportVer,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"games_root":  settingString(gamesRootKey, ""),
		"count":       len(items),
		"apps":        items,
	})
}

type catalogImportRequest struct {
	Apps   []catalogFields `json:"apps"`
	DryRun bool            `json:"dry_run"`
	// З18-и2 (из №40): корень игр из файла экспорта — в плане; применить вместе с импортом — по галочке
	GamesRoot      *string `json:"games_root"`
	ApplyGamesRoot bool    `json:"apply_games_root"`
}

// POST /admin/catalog/import — массовое наполнение каталога (owner).
// Семантика (решения 27.08): только upsert — записи, которых нет в файле,
// остаются нетронутыми; enabled из файла уважается, если поле прислано;
// у существующей записи меняются ТОЛЬКО присланные поля (пустая строка —
// очистить, отсутствие поля — не трогать). dry_run=true считает план, ничего
// не записывая; запись идёт одной транзакцией — либо всё, либо ничего.
// З18-и2: в плане — «было → стало» по команде и обложке и корень игр из файла; тело больше потолка (без
// заявленного размера его обрывает MaxBytesReader общей мидлвари) — 413 «до 1 МБ», а не «Пустое тело».
func handleAdminCatalogImport(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxImportBytes+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || len(body) > maxImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "import_too_big",
			"message": fmt.Sprintf("Файл импорта — до %d МБ", maxImportBytes>>20)})
		return
	}
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Пустое тело запроса"})
		return
	}
	var req catalogImportRequest
	trimmed := bytes.TrimSpace(body)
	if trimmed[0] == '[' { // голый массив записей — тоже принимаем
		if err := json.Unmarshal(trimmed, &req.Apps); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_json", "message": "JSON не разобрался: " + err.Error()})
			return
		}
	} else if err := json.Unmarshal(trimmed, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_json", "message": "JSON не разобрался: " + err.Error()})
		return
	}
	if c.Query("dry_run") == "1" {
		req.DryRun = true
	}
	if len(req.Apps) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "empty_import",
			"message": `Ни одной записи. Ожидается массив или {"apps":[…]}`})
		return
	}
	if len(req.Apps) > maxImportApps {
		c.JSON(http.StatusBadRequest, gin.H{"code": "import_too_many",
			"message": fmt.Sprintf("За один раз — до %d записей", maxImportApps)})
		return
	}

	var all []models.CatalogApp
	db.Find(&all)
	existing := make(map[string]*models.CatalogApp, len(all))
	for i := range all {
		existing[all[i].ID] = &all[i]
	}

	type importStep struct {
		f       *catalogFields
		isOld   bool
		changes []string
		diff    []cmdDiff // З18-и2: «было → стало» по target, args, cover_url
	}
	steps := make([]importStep, 0, len(req.Apps))
	plan := make([]gin.H, 0, len(req.Apps))
	errs := make([]gin.H, 0)
	seen := make(map[string]int, len(req.Apps))
	created, updated, unchanged, commands := 0, 0, 0, 0
	for i := range req.Apps {
		f := &req.Apps[i]
		f.ID = strings.TrimSpace(f.ID)
		if prev, dup := seen[f.ID]; dup {
			errs = append(errs, gin.H{"index": i, "id": f.ID, "code": "duplicate_id",
				"message": fmt.Sprintf("id повторяется (уже в записи №%d)", prev+1)})
			continue
		}
		seen[f.ID] = i
		old, isOld := existing[f.ID]
		if code, msg := validateCatalogFields(f, !isOld); code != "" {
			errs = append(errs, gin.H{"index": i, "id": f.ID, "code": code, "message": msg})
			continue
		}
		row := gin.H{"index": i, "id": f.ID}
		step := importStep{f: f, isOld: isOld}
		if isOld {
			step.diff = catalogCommandDiff(f, old)
			if cmdChanged(step.diff) {
				commands++
			}
			step.changes = catalogImportChanges(f, old)
			if len(step.changes) == 0 {
				unchanged++
			} else {
				updated++
			}
			row["action"] = "update"
			row["name"] = old.Name
			row["changes"] = step.changes
		} else {
			created++
			row["action"] = "create"
			if f.Name != nil {
				row["name"] = strings.TrimSpace(*f.Name)
			}
			step.diff = catalogCommandDiff(f, nil)
		}
		if len(step.diff) > 0 {
			row["diff"] = step.diff
		}
		steps = append(steps, step)
		plan = append(plan, row)
	}
	if len(errs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "import_invalid", "total": len(req.Apps),
			"message": fmt.Sprintf("Ошибок в файле: %d — ничего не сохранено", len(errs)),
			"errors":  errs, "items": plan})
		return
	}
	// корень игр из файла (экспорт его пишет): пустой — как не прислан; применить — только верный и по галочке
	curRoot, fileRoot := settingString(gamesRootKey, ""), ""
	if req.GamesRoot != nil {
		fileRoot = strings.TrimSpace(*req.GamesRoot)
	}
	var root gin.H
	if fileRoot != "" {
		ok, msg := validateGamesRoot(fileRoot)
		root = gin.H{"file": fileRoot, "current": curRoot, "differs": fileRoot != curRoot, "ok": ok}
		if !ok {
			root["message"] = msg
		}
	}
	setRoot := req.ApplyGamesRoot && root != nil && root["differs"] == true
	if req.ApplyGamesRoot && (root == nil || root["ok"] != true) {
		msg := "Корня игр в файле нет"
		if root != nil {
			msg, _ = root["message"].(string)
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_games_root", "message": msg + " — ничего не сохранено"})
		return
	}
	if req.DryRun {
		out := gin.H{"dry_run": true, "total": len(req.Apps), "created": created,
			"updated": updated, "unchanged": unchanged, "commands": commands, "items": plan}
		if root != nil {
			out["games_root"] = root
		}
		c.JSON(http.StatusOK, out)
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if setRoot {
			if e := catalogSetRootTx(tx, fileRoot); e != nil {
				return e
			}
		}
		for _, st := range steps {
			f := st.f
			if st.isOld {
				// нечего менять — не трогаем строку, чтобы updated_at остался
				// честной датой последней правки (его читает и админка)
				if len(st.changes) == 0 {
					continue
				}
				if e := tx.Model(&models.CatalogApp{}).Where("id = ?", f.ID).
					Updates(catalogImportUpdates(f)).Error; e != nil {
					return e
				}
				continue
			}
			args, _, _ := normalizeArgs(f.Args)
			app := models.CatalogApp{
				ID: f.ID, Name: strings.TrimSpace(*f.Name), Category: strings.TrimSpace(*f.Category),
				Subtitle: trimPtr(f.Subtitle), Tag: trimPtr(f.Tag), Emoji: trimPtr(f.Emoji),
				ColorA: trimPtr(f.ColorA), ColorB: trimPtr(f.ColorB),
				Target: trimPtr(f.Target), Args: args, CoverURL: trimPtr(f.CoverURL),
				Sort: 100, Enabled: true, UpdatedAt: time.Now(),
			}
			if f.Sort != nil {
				app.Sort = *f.Sort
			}
			if f.Enabled != nil {
				app.Enabled = *f.Enabled
			}
			if e := tx.Create(&app).Error; e != nil {
				return e
			}
			// enabled=false отдельным UPDATE: у колонки DEFAULT TRUE, а GORM
			// не пишет в INSERT нулевые значения полей с тегом default —
			// выключенная в файле запись иначе приезжала бы включённой.
			if f.Enabled != nil && !*f.Enabled {
				if e := tx.Model(&models.CatalogApp{}).Where("id = ?", f.ID).
					Update("enabled", false).Error; e != nil {
					return e
				}
			}
		}
		return nil
	})
	if err != nil {
		respondDBError(c, err)
		return
	}
	// З18-и2: строка на запись, у которой меняются команда или обложка, — «было → стало», откатить есть чем
	for _, st := range steps {
		if st.isOld && len(st.diff) > 0 {
			logAdminAction(c, "catalog_import", nil, st.f.ID+": "+cmdDiffText(st.diff))
		}
	}
	if setRoot {
		logAdminAction(c, "catalog_config", nil, fmt.Sprintf("games_root: %q → %q (из файла импорта)", curRoot, fileRoot))
		root["applied"] = true
	}
	sum := fmt.Sprintf("импорт каталога: %d записей (+%d новых, %d обновлено, %d без изменений)",
		len(req.Apps), created, updated, unchanged)
	if commands > 0 {
		sum += fmt.Sprintf("; команды запуска изменены у %d", commands)
	}
	logAdminAction(c, "catalog_import", nil, sum)
	out := gin.H{"ok": true, "total": len(req.Apps), "created": created,
		"updated": updated, "unchanged": unchanged, "commands": commands, "items": plan}
	if root != nil {
		out["games_root"] = root
	}
	c.JSON(http.StatusOK, out)
}

// catalogSetRootTx — корень игр в той же транзакции, что импорт (upsert по ключу, как settingSetString).
func catalogSetRootTx(tx *gorm.DB, root string) error {
	var s models.Setting
	if err := tx.First(&s, "key = ?", gamesRootKey).Error; err == nil {
		return tx.Model(&s).Update("value", root).Error
	}
	return tx.Create(&models.Setting{Key: gamesRootKey, Value: root}).Error
}

// ── З18-и2 (OWNER.md, №33 «а»): «было → стало» по команде запуска и обложке ──────────────────────────────────────────
//
// target и args — команда, которую агент выполняет на каждом ПК зала (exec.Command(target, args…)), cover_url — что
// видит гость. Превью импорта показывало «cs2 → target» без значений, журнал — счётчики (импорт) и id (форма): старого
// значения нигде не было, откатить нечем (ревью 23.09, №30). Теперь у записи в плане импорта — «было → стало» по этим
// трём полям (у новой — что будет), путь с буквой диска или сетевой помечен; в журнал — та же строка.

// cmdDiff — «было → стало» одного из полей; Disk — в новом значении путь с буквой диска или сетевой (UNC).
type cmdDiff struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
	Disk  bool   `json:"disk,omitempty"`
}

// diskPathRe — буква диска (C:\, D:/ — не в середине слова: «https://» и «steam://» не путь) или UNC (\\nas\games).
var diskPathRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9])[A-Za-z]:[\\/]|\\\\[^\\/\s"]+[\\/]`)

// hasDiskPath — чистая (юнит): в команде есть путь с буквой диска или сетевой. {GAMES}\…, steam://…, https://…,
// ms-settings:… — нет.
func hasDiskPath(s string) bool { return diskPathRe.MatchString(s) }

// argsHaveDiskPath — то же по аргументам: JSON-массив строк, каждая — без экранирования JSON.
func argsHaveDiskPath(args string) bool {
	var arr []string
	if json.Unmarshal([]byte(args), &arr) != nil {
		return hasDiskPath(args)
	}
	for _, a := range arr {
		if hasDiskPath(a) {
			return true
		}
	}
	return false
}

// compactArgs — args из БД в компактном виде: Postgres хранит jsonb со своими пробелами, а сравниваем с присланным.
func compactArgs(s string) string {
	if s == "" {
		return ""
	}
	if v, ok, _ := normalizeArgs(json.RawMessage(s)); ok {
		if v == nil {
			return ""
		}
		return *v
	}
	return s
}

// catalogCommandDiff — чистая (юнит): «было → стало» по target, args и cover_url — у существующей записи только
// изменившиеся из присланных (как catalogImportChanges), у новой (old == nil) — что задано.
func catalogCommandDiff(f *catalogFields, old *models.CatalogApp) []cmdDiff {
	cur := map[string]string{}
	if old != nil {
		cur["target"], cur["args"], cur["cover_url"] = deref(old.Target), compactArgs(deref(old.Args)), deref(old.CoverURL)
	}
	next := catalogImportUpdates(f)
	var out []cmdDiff
	for _, col := range []string{"target", "args", "cover_url"} {
		v, sent := next[col]
		if !sent {
			continue
		}
		s := ""
		if v != nil {
			s = fmt.Sprint(v)
		}
		if s == cur[col] {
			continue
		}
		d := cmdDiff{Field: col, Old: cur[col], New: s}
		switch col {
		case "target":
			d.Disk = hasDiskPath(s)
		case "args":
			d.Disk = argsHaveDiskPath(s)
		}
		out = append(out, d)
	}
	return out
}

// cmdChanged — меняется ли команда запуска (target или args), а не только обложка.
func cmdChanged(ds []cmdDiff) bool {
	for _, d := range ds {
		if d.Field != "cover_url" {
			return true
		}
	}
	return false
}

// cmdDiffText — чистая (юнит): «было → стало» словами для журнала — «target «{GAMES}\x.exe» → «C:\x.exe» · args
// пусто → «["-novid"]»».
func cmdDiffText(ds []cmdDiff) string {
	q := func(s string) string {
		if s == "" {
			return "пусто"
		}
		return "«" + s + "»"
	}
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		parts = append(parts, d.Field+" "+q(d.Old)+" → "+q(d.New))
	}
	return strings.Join(parts, " · ")
}

// cmdSetText — что задано у новой записи (форма): «target «steam://rungameid/1» · args «["-novid"]»».
func cmdSetText(ds []cmdDiff) string {
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		parts = append(parts, d.Field+" «"+d.New+"»")
	}
	return strings.Join(parts, " · ")
}

// catalogImportUpdates — колонки, которые импорт меняет у существующей записи:
// только присланные. Пустая строка очищает поле, отсутствие поля — не трогает.
func catalogImportUpdates(f *catalogFields) map[string]any {
	up := map[string]any{"updated_at": time.Now()}
	set := func(col string, v *string) {
		if v == nil {
			return
		}
		if t := strings.TrimSpace(*v); t != "" {
			up[col] = t
		} else {
			up[col] = nil
		}
	}
	if f.Name != nil {
		up["name"] = strings.TrimSpace(*f.Name)
	}
	if f.Category != nil {
		up["category"] = strings.TrimSpace(*f.Category)
	}
	set("subtitle", f.Subtitle)
	set("tag", f.Tag)
	set("emoji", f.Emoji)
	set("color_a", f.ColorA)
	set("color_b", f.ColorB)
	set("target", f.Target)
	set("cover_url", f.CoverURL)
	if len(bytes.TrimSpace(f.Args)) > 0 {
		if args, ok, _ := normalizeArgs(f.Args); ok {
			if args != nil {
				up["args"] = *args
			} else {
				up["args"] = nil
			}
		}
	}
	if f.Sort != nil {
		up["sort"] = *f.Sort
	}
	if f.Enabled != nil {
		up["enabled"] = *f.Enabled
	}
	return up
}

// catalogImportChanges — какие поля реально изменятся (для превью импорта).
func catalogImportChanges(f *catalogFields, old *models.CatalogApp) []string {
	// args в БД — jsonb: Postgres хранит его со своими пробелами, поэтому
	// сравниваем оба значения в одном (компактном) виде, иначе реимпорт
	// собственного экспорта показывал бы «изменился args» на ровном месте.
	oldArgs := compactArgs(deref(old.Args))
	cur := map[string]string{
		"name": old.Name, "category": old.Category,
		"subtitle": deref(old.Subtitle), "tag": deref(old.Tag), "emoji": deref(old.Emoji),
		"color_a": deref(old.ColorA), "color_b": deref(old.ColorB),
		"target": deref(old.Target), "cover_url": deref(old.CoverURL), "args": oldArgs,
	}
	next := catalogImportUpdates(f)
	changed := make([]string, 0, 4)
	for _, col := range []string{"name", "category", "subtitle", "tag", "emoji",
		"color_a", "color_b", "target", "cover_url", "args"} {
		v, sent := next[col]
		if !sent {
			continue
		}
		s := ""
		if v != nil {
			s = fmt.Sprint(v)
		}
		if s != cur[col] {
			changed = append(changed, col)
		}
	}
	if v, sent := next["sort"]; sent && v.(int) != old.Sort {
		changed = append(changed, "sort")
	}
	if v, sent := next["enabled"]; sent && v.(bool) != old.Enabled {
		changed = append(changed, "enabled")
	}
	return changed
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
