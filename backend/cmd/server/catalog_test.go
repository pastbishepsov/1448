package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestValidateCatalogApp(t *testing.T) {
	ok, _ := validateCatalogApp("cs2", "Counter-Strike 2", "game")
	if !ok {
		t.Error("валидное приложение отклонено")
	}
	cases := []struct {
		id, name, category, wantCode string
	}{
		{"CS2", "x", "game", "bad_id"},          // верхний регистр
		{"a", "x", "game", "bad_id"},            // слишком короткий
		{"тест", "x", "game", "bad_id"},         // не латиница
		{"has space", "x", "game", "bad_id"},    // пробел
		{"ok_id", "", "game", "bad_name"},       // пустое имя
		{"ok_id", "x", "games", "bad_category"}, // нет такой категории
	}
	for _, tc := range cases {
		ok, code := validateCatalogApp(tc.id, tc.name, tc.category)
		if ok || code != tc.wantCode {
			t.Errorf("(%q,%q,%q): ok=%v code=%q, ожидался %q", tc.id, tc.name, tc.category, ok, code, tc.wantCode)
		}
	}
	// Длина названия — в символах: в колонке VARCHAR(64), а по байтам русское
	// название отваливалось вдвое раньше (нашлось смоуком админки).
	long40 := strings.Repeat("я", 40) // 40 символов, 80 байт
	if ok, code := validateCatalogApp("ok_id", long40, "game"); !ok {
		t.Errorf("русское название в 40 символов отклонено: %s", code)
	}
	if ok, _ := validateCatalogApp("ok_id", strings.Repeat("я", 65), "game"); ok {
		t.Error("название в 65 символов должно отклоняться")
	}
}

// ── {GAMES}: подстановка корня игр (П1) ─────────────────────────────────────

func TestExpandGames(t *testing.T) {
	cases := []struct{ target, root, want string }{
		{`{GAMES}\CS2\cs2.exe`, `G:\Games`, `G:\Games\CS2\cs2.exe`},
		{`{GAMES}\CS2\cs2.exe`, `G:\Games\`, `G:\Games\CS2\cs2.exe`}, // хвостовой слэш срезаем
		{`{GAMES}\CS2\cs2.exe`, `\\nas\games`, `\\nas\games\CS2\cs2.exe`},
		{`{GAMES}/cs2/cs2.sh`, `/mnt/games/`, `/mnt/games/cs2/cs2.sh`},
		{`{GAMES}\a.exe`, "", `{GAMES}\a.exe`},                         // корень не задан — шаблон виден
		{`steam://rungameid/730`, `G:\Games`, `steam://rungameid/730`}, // без плейсхолдера — как есть
		{"", `G:\Games`, ""},
		{`{GAMES}\a\{GAMES}.exe`, `G:\G`, `G:\G\a\G:\G.exe`}, // подставляются все вхождения
	}
	for _, tc := range cases {
		if got := expandGames(tc.target, tc.root); got != tc.want {
			t.Errorf("expandGames(%q, %q) = %q, ожидалось %q", tc.target, tc.root, got, tc.want)
		}
	}
}

// Корень попадает в строковый литерал JSON — обратные слэши должны удвоиться,
// иначе агент не разберёт args (самая дорогая ошибка: игра не запускается).
func TestExpandGamesArgsStaysValidJSON(t *testing.T) {
	args := `["--dir={GAMES}\\CS2","--fullscreen"]`
	out := expandGamesArgs(args, `G:\Games\`)
	var arr []string
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("после подстановки args перестал быть JSON: %v (%s)", err, out)
	}
	if arr[0] != `--dir=G:\Games\CS2` {
		t.Errorf("arg[0] = %q, ожидалось %q", arr[0], `--dir=G:\Games\CS2`)
	}
	if arr[1] != "--fullscreen" {
		t.Errorf("второй аргумент потерян: %q", arr[1])
	}
	// UNC-корень тоже не должен ломать JSON
	out = expandGamesArgs(`["{GAMES}\\x.exe"]`, `\\nas\games`)
	arr = nil
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("UNC-корень сломал JSON: %v (%s)", err, out)
	}
	if arr[0] != `\\nas\games\x.exe` {
		t.Errorf("UNC: получилось %q", arr[0])
	}
	// Без корня и без плейсхолдера строка не меняется
	if got := expandGamesArgs(args, ""); got != args {
		t.Errorf("без корня args изменился: %q", got)
	}
}

func TestValidateGamesRoot(t *testing.T) {
	good := []string{"", `G:\Games`, `g:/games`, `\\nas\games`, `\\nas\games\1448`, "/mnt/games"}
	for _, r := range good {
		if ok, msg := validateGamesRoot(r); !ok {
			t.Errorf("корень %q отклонён: %s", r, msg)
		}
	}
	bad := []string{"Games", `G:`, `\\nas`, "G:\\Games\n", `G:\Games\{X}`, strings.Repeat("/a", 200)}
	for _, r := range bad {
		if ok, _ := validateGamesRoot(r); ok {
			t.Errorf("корень %q принят, а не должен", r)
		}
	}
}

// ── Обложки ─────────────────────────────────────────────────────────────────

func TestValidateCoverURL(t *testing.T) {
	good := []string{"", "https://cdn.example.com/cs2.jpg", "http://nas.local/cs2.png",
		"/covers/cs2.jpg", "/api/v1/catalog/cs2/cover?v=1"}
	for _, u := range good {
		if ok, msg := validateCoverURL(u); !ok {
			t.Errorf("ссылка %q отклонена: %s", u, msg)
		}
	}
	bad := []string{"data:image/png;base64,AAAA", "javascript:alert(1)", "//evil.example.com/x.jpg",
		"covers/cs2.jpg", "https://x.example/a b.jpg", `https://x.example/"onerror=`}
	for _, u := range bad {
		if ok, _ := validateCoverURL(u); ok {
			t.Errorf("ссылка %q принята, а не должна", u)
		}
	}
}

func TestCatalogCoverURLUploadWins(t *testing.T) {
	link := "https://cdn.example.com/cs2.jpg"
	a := models.CatalogApp{ID: "cs2", CoverURL: &link}
	if got := catalogCoverURL(&a); got != link {
		t.Errorf("без загрузки должна быть ссылка, получено %q", got)
	}
	at := time.Unix(1700000000, 0)
	a.CoverAt = &at
	if got := catalogCoverURL(&a); got != "/api/v1/catalog/cs2/cover?v=1700000000" {
		t.Errorf("загруженная обложка не победила ссылку: %q", got)
	}
	empty := models.CatalogApp{ID: "x"}
	if got := catalogCoverURL(&empty); got != "" {
		t.Errorf("без обложки ожидалась пустая строка, получено %q", got)
	}
}

// В публичной выдаче cover_url совпадает с cover: витрина (shell.html) читает
// `cover_url || cover`, и разъезд полей показал бы гостю старую ссылку вместо
// загруженной обложки. В админке, наоборот, cover_url остаётся тем, что ввёл
// владелец, — иначе форма правки затирала бы ссылку своим же значением.
func TestDecorateCatalogAppCoverFields(t *testing.T) {
	link := "/covers/cs2.jpg"
	at := time.Unix(1700000000, 0)
	pub := models.CatalogApp{ID: "cs2", CoverURL: &link, CoverAt: &at}
	decorateCatalogApp(&pub, "", true)
	if pub.Cover != "/api/v1/catalog/cs2/cover?v=1700000000" || deref(pub.CoverURL) != pub.Cover {
		t.Errorf("публичная выдача: cover=%q cover_url=%q", pub.Cover, deref(pub.CoverURL))
	}
	adm := models.CatalogApp{ID: "cs2", CoverURL: &link, CoverAt: &at}
	decorateCatalogApp(&adm, "", false)
	if deref(adm.CoverURL) != link {
		t.Errorf("админка должна отдавать ссылку владельца, а вернула %q", deref(adm.CoverURL))
	}
	// подстановка корня — только в публичной выдаче, с сохранением шаблона
	tpl := `{GAMES}\CS2\cs2.exe`
	p2 := models.CatalogApp{ID: "cs2", Target: &tpl}
	decorateCatalogApp(&p2, `G:\Games`, true)
	if deref(p2.Target) != `G:\Games\CS2\cs2.exe` || p2.TargetRaw != tpl {
		t.Errorf("публичная подстановка: target=%q target_raw=%q", deref(p2.Target), p2.TargetRaw)
	}
	a2 := models.CatalogApp{ID: "cs2", Target: &tpl}
	decorateCatalogApp(&a2, `G:\Games`, false)
	if deref(a2.Target) != tpl || a2.TargetRaw != "" {
		t.Errorf("админке нужен исходный шаблон, а пришло %q", deref(a2.Target))
	}
}

// ── args: массив, строка с массивом, мусор ──────────────────────────────────

func TestNormalizeArgs(t *testing.T) {
	cases := []struct {
		raw   string
		want  string // "" = nil
		valid bool
	}{
		{`["--a","--b"]`, `["--a","--b"]`, true}, // настоящий массив (экспорт, руками)
		{`"[\"--a\"]"`, `["--a"]`, true},         // строка с JSON (форма админки, jsonb)
		{`null`, "", true},                       // явный null
		{``, "", true},                           // поля не было
		{`""`, "", true},                         // пустая строка
		{`[]`, "", true},                         // пустой массив
		{`["--a", 5]`, "", false},                // не строки
		{`{"a":1}`, "", false},                   // не массив
		{`"[--a]"`, "", false},                   // внутри строки не JSON
	}
	for _, tc := range cases {
		got, ok, _ := normalizeArgs(json.RawMessage(tc.raw))
		if ok != tc.valid {
			t.Errorf("normalizeArgs(%s): ok=%v, ожидалось %v", tc.raw, ok, tc.valid)
			continue
		}
		if !ok {
			continue
		}
		if tc.want == "" && got != nil {
			t.Errorf("normalizeArgs(%s) = %q, ожидался nil", tc.raw, *got)
		}
		if tc.want != "" && (got == nil || *got != tc.want) {
			t.Errorf("normalizeArgs(%s) = %v, ожидалось %q", tc.raw, got, tc.want)
		}
	}
}

// ── Проверка полей записи (общая для формы и импорта) ───────────────────────

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }
func bp(b bool) *bool     { return &b }

func TestValidateCatalogFieldsFull(t *testing.T) {
	ok := catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
		Target: sp(`{GAMES}\CS2\cs2.exe`), ColorA: sp("#f59e0b"), Sort: ip(10)}
	if code, msg := validateCatalogFields(&ok, true); code != "" {
		t.Errorf("валидная запись отклонена: %s / %s", code, msg)
	}
	bad := []struct {
		name string
		f    catalogFields
		code string
	}{
		{"без имени", catalogFields{ID: "cs2", Category: sp("game")}, "bad_name"},
		{"без категории", catalogFields{ID: "cs2", Name: sp("CS2")}, "bad_category"},
		{"кривой id", catalogFields{ID: "CS 2", Name: sp("CS2"), Category: sp("game")}, "bad_id"},
		{"длинный tag", catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
			Tag: sp("СЛИШКОМ-ДЛИННЫЙ-ТЕГ")}, "bad_tag"},
		{"кривой цвет", catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
			ColorA: sp("оранжевый")}, "bad_color"},
		{"data-обложка", catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
			CoverURL: sp("data:image/png;base64,AA")}, "bad_cover"},
		{"дикий sort", catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
			Sort: ip(99999)}, "bad_sort"},
		{"мусор в args", catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
			Args: json.RawMessage(`{"a":1}`)}, "bad_args"},
	}
	for _, tc := range bad {
		code, _ := validateCatalogFields(&tc.f, true)
		if code != tc.code {
			t.Errorf("%s: code=%q, ожидался %q", tc.name, code, tc.code)
		}
	}
}

// Импорт умеет частичную правку: у существующей записи можно прислать один
// только target (массовая смена путей), не переписывая имя и категорию.
func TestValidateCatalogFieldsPartial(t *testing.T) {
	f := catalogFields{ID: "cs2", Target: sp(`{GAMES}\CS2\cs2.exe`)}
	if code, msg := validateCatalogFields(&f, false); code != "" {
		t.Errorf("частичная правка отклонена: %s / %s", code, msg)
	}
	if code, _ := validateCatalogFields(&f, true); code != "bad_name" {
		t.Errorf("новая запись без имени должна падать в bad_name, получено %q", code)
	}
	empty := catalogFields{ID: "cs2", Name: sp("  ")}
	if code, _ := validateCatalogFields(&empty, false); code != "bad_name" {
		t.Errorf("пустое имя при правке должно падать, получено %q", code)
	}
}

// ── Импорт: что именно уедет в UPDATE ───────────────────────────────────────

func TestCatalogImportUpdatesOnlySentFields(t *testing.T) {
	f := catalogFields{ID: "cs2", Target: sp(`{GAMES}\CS2\cs2.exe`), Subtitle: sp(""), Enabled: bp(false)}
	up := catalogImportUpdates(&f)
	if up["target"] != `{GAMES}\CS2\cs2.exe` {
		t.Errorf("target не попал в UPDATE: %v", up["target"])
	}
	if v, ok := up["subtitle"]; !ok || v != nil {
		t.Errorf(`пустая строка должна очищать поле (NULL), получено %v`, v)
	}
	if up["enabled"] != false {
		t.Errorf("enabled из файла не уважён: %v", up["enabled"])
	}
	for _, absent := range []string{"name", "category", "tag", "emoji", "color_a", "color_b", "cover_url", "args", "sort"} {
		if _, ok := up[absent]; ok {
			t.Errorf("поле %q не присылали, а оно в UPDATE: %v", absent, up[absent])
		}
	}
	if _, ok := up["updated_at"]; !ok {
		t.Error("updated_at должен обновляться всегда")
	}
	// args настоящим массивом — в БД уезжает компактный JSON-текст
	f2 := catalogFields{ID: "cs2", Args: json.RawMessage(`["--a","--b"]`)}
	if got := catalogImportUpdates(&f2)["args"]; got != `["--a","--b"]` {
		t.Errorf("args в UPDATE: %v", got)
	}
}

func TestCatalogImportChanges(t *testing.T) {
	old := models.CatalogApp{ID: "cs2", Name: "CS2", Category: "game",
		Target: sp(`C:\old\cs2.exe`), Sort: 10, Enabled: true}
	f := catalogFields{ID: "cs2", Name: sp("CS2"), Category: sp("game"),
		Target: sp(`{GAMES}\CS2\cs2.exe`), Sort: ip(10), Enabled: bp(true)}
	ch := catalogImportChanges(&f, &old)
	if len(ch) != 1 || ch[0] != "target" {
		t.Errorf("ожидалось изменение только target, получено %v", ch)
	}
	f.Enabled = bp(false)
	f.Sort = ip(20)
	ch = catalogImportChanges(&f, &old)
	if len(ch) != 3 {
		t.Errorf("ожидались target/sort/enabled, получено %v", ch)
	}
	same := catalogFields{ID: "cs2", Name: sp("CS2")}
	if ch := catalogImportChanges(&same, &old); len(ch) != 0 {
		t.Errorf("ничего не менялось, а показано %v", ch)
	}
}

// Реимпорт собственного экспорта не должен показывать ложных изменений: в БД
// args лежит в jsonb, и Postgres отдаёт его со своими пробелами.
func TestCatalogImportChangesArgsWhitespace(t *testing.T) {
	stored := `["--launch-product=valorant", "--launch-patchline=live"]` // как вернул Postgres
	old := models.CatalogApp{ID: "valorant", Name: "Valorant", Category: "game", Args: &stored}
	f := catalogFields{ID: "valorant", Args: json.RawMessage(`["--launch-product=valorant","--launch-patchline=live"]`)}
	if ch := catalogImportChanges(&f, &old); len(ch) != 0 {
		t.Errorf("те же аргументы, а показано изменение %v", ch)
	}
	f.Args = json.RawMessage(`["--launch-product=valorant"]`)
	if ch := catalogImportChanges(&f, &old); len(ch) != 1 || ch[0] != "args" {
		t.Errorf("реальная правка args не замечена: %v", ch)
	}
}

// ── Маршруты каталога ───────────────────────────────────────────────────────

// Статические ручки каталога (import/export/config) соседствуют с /catalog/:id.
// gin 1.10 это разводит — и на регистрации, и на совпадении, включая возврат к
// :id, когда статика совпала лишь частично. Тест держит это свойство под
// контролем: если апгрейд роутера его сломает, узнаем здесь, а не на стенде.
func TestCatalogRoutesRegisterAndMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("роутер упал на регистрации маршрутов каталога: %v", r)
		}
	}()
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/catalog", handleGetCatalog)
	v1.GET("/catalog/:id/cover", handleCatalogCover)
	adm := v1.Group("/admin")
	adm.GET("/catalog", handleAdminCatalog)
	adm.POST("/catalog/:id/toggle", handleAdminCatalogToggle)
	adm.POST("/catalog/:id/sort", handleAdminCatalogSort)
	own := adm.Group("")
	own.POST("/catalog", handleAdminCatalogUpsert)
	own.DELETE("/catalog/:id", handleAdminCatalogDelete)
	own.GET("/catalog/config", handleAdminCatalogConfigGet)
	own.PUT("/catalog/config", handleAdminCatalogConfigPut)
	own.GET("/catalog/export", handleAdminCatalogExport)
	own.POST("/catalog/import", handleAdminCatalogImport)
	own.PUT("/catalog/:id/cover", handleAdminCatalogCoverPut)
	own.DELETE("/catalog/:id/cover", handleAdminCatalogCoverDelete)
	if n := len(r.Routes()); n != 13 {
		t.Errorf("зарегистрировано %d маршрутов каталога, ожидалось 13", n)
	}

	// Совпадение проверяем на заглушках: настоящие хендлеры полезли бы в БД.
	m := gin.New()
	hit := ""
	stub := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) { hit = name + ":" + c.Param("id") }
	}
	g := m.Group("/api/v1/admin")
	g.POST("/catalog/:id/toggle", stub("toggle"))
	g.POST("/catalog/import", stub("import"))
	g.PUT("/catalog/config", stub("config"))
	g.PUT("/catalog/:id/cover", stub("cover"))
	cases := []struct{ method, path, want string }{
		{"POST", "/api/v1/admin/catalog/import", "import:"},
		{"POST", "/api/v1/admin/catalog/cs2/toggle", "toggle:cs2"},
		{"POST", "/api/v1/admin/catalog/imp/toggle", "toggle:imp"}, // общий префикс со статикой
		{"PUT", "/api/v1/admin/catalog/config", "config:"},
		{"PUT", "/api/v1/admin/catalog/cs2/cover", "cover:cs2"},
	}
	for _, tc := range cases {
		hit = ""
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusOK || hit != tc.want {
			t.Errorf("%s %s → код %d, попал в %q, ожидалось %q", tc.method, tc.path, w.Code, hit, tc.want)
		}
	}
}

// ── Сид каталога пилотного клуба (Фаза 0) ──────────────────────────────────
//
// seed/catalog-pilot.json собран из docs/РАЗВЕДКА-ИГРЫ.md и уезжает в клуб
// кнопкой «Импорт JSON». Тест прогоняет файл ровно теми проверками, что и
// ручка импорта (handleAdminCatalogImport): лимиты тела и количества,
// дубликаты id, validateCatalogFields по каждой записи. Ошибка здесь
// означает, что живой импорт откажет и не сохранит ничего.

// seedPath — путь к сиду от backend/cmd/server.
const seedPath = "../../../seed/catalog-pilot.json"

// loadSeed — файл сида, разобранный как тело импорта.
func loadSeed(t *testing.T) ([]byte, []catalogFields) {
	t.Helper()
	data, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("сид не читается: %v", err)
	}
	var req catalogImportRequest
	if err := json.Unmarshal(bytes.TrimSpace(data), &req); err != nil {
		t.Fatalf("сид не разобрался как тело импорта: %v", err)
	}
	return data, req.Apps
}

// seedFull — так же, как импорт, решаем, требовать ли обязательные поля:
// запись с name/category — новая (полная проверка), запись из одного id —
// правка существующей (проверяется только присланное).
func seedFull(f *catalogFields) bool { return f.Name != nil || f.Category != nil }

func TestSeedCatalogPilotPassesImport(t *testing.T) {
	data, apps := loadSeed(t)
	if len(data) > maxImportBytes {
		t.Fatalf("сид %d байт — импорт принимает до %d", len(data), maxImportBytes)
	}
	if len(apps) == 0 || len(apps) > maxImportApps {
		t.Fatalf("в сиде %d записей — импорт принимает 1..%d", len(apps), maxImportApps)
	}
	seen := map[string]int{}
	for i := range apps {
		f := &apps[i]
		f.ID = strings.TrimSpace(f.ID)
		if prev, dup := seen[f.ID]; dup {
			t.Errorf("запись №%d: id %q повторяется (уже в записи №%d)", i+1, f.ID, prev+1)
			continue
		}
		seen[f.ID] = i
		if code, msg := validateCatalogFields(f, seedFull(f)); code != "" {
			t.Errorf("запись №%d (%s): %s — %s", i+1, f.ID, code, msg)
		}
	}
}

// TestSeedCatalogPilotPatchesOnlyExistingRows — записи без name/category
// (fortnite, lol, ow2, …) импорт считает правкой существующей строки. Если
// такой строки в базе нет, он потребует name/category и откажет всему файлу.
// Проверяем, что каждый такой id заведён миграциями.
func TestSeedCatalogPilotPatchesOnlyExistingRows(t *testing.T) {
	_, apps := loadSeed(t)
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatalf("миграции не читаются: %v", err)
	}
	var sql strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("../../migrations", e.Name()))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		sql.Write(b)
	}
	all := sql.String()
	for i := range apps {
		f := &apps[i]
		if seedFull(f) {
			continue
		}
		if !strings.Contains(all, "('"+f.ID+"',") {
			t.Errorf("запись №%d (%s): без name/category, но миграции такой строки не заводят — импорт ответит bad_name",
				i+1, f.ID)
		}
	}
}

// TestSeedCatalogPilotGamesRoot — как ложится корень клуба на пути сида:
// {GAMES} разворачивается без двойных слэшей и без остатка шаблона, и точно
// так же на ПК с другой буквой диска. Абсолютные пути (EA app, CS 1.6 из
// чужого образа) корнем не задеваются, steam:// — тем более.
func TestSeedCatalogPilotGamesRoot(t *testing.T) {
	_, apps := loadSeed(t)
	roots := []string{`D:\online`, `D:\online\`, `E:\games\online`, `\\nas\online`}
	tmpl, steam, abs := 0, 0, 0
	for _, root := range roots {
		if ok, msg := validateGamesRoot(root); !ok {
			t.Fatalf("корень %q забракован: %s", root, msg)
		}
		for i := range apps {
			f := &apps[i]
			if f.Target == nil || *f.Target == "" {
				continue
			}
			got := expandGames(*f.Target, root)
			if strings.Contains(got, gamesPlaceholder) {
				t.Errorf("%s: {GAMES} остался в %q", f.ID, got)
			}
			if strings.Contains(strings.TrimPrefix(got, `\\`), `\\`) {
				t.Errorf("%s: двойной слэш в %q (корень %q)", f.ID, got, root)
			}
			if root == roots[0] {
				switch {
				case strings.Contains(*f.Target, gamesPlaceholder):
					tmpl++
					if !strings.HasPrefix(got, `D:\online\`) {
						t.Errorf("%s: %q не под корнем клуба", f.ID, got)
					}
				case strings.HasPrefix(*f.Target, "steam://"):
					steam++
					if got != *f.Target {
						t.Errorf("%s: steam-цель изменилась: %q", f.ID, got)
					}
				default:
					abs++
				}
			}
		}
	}
	if tmpl == 0 || steam == 0 || abs == 0 {
		t.Errorf("сид потерял вид целей: {GAMES}=%d steam=%d абсолютных=%d", tmpl, steam, abs)
	}
}
