package main

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З18-и3: какие SQL строит GORM с omitBlobs — без базы (DryRun, без пинга; Scan в DryRun GORM не выполняет и пишет
// об этом в лог — лог тихий, SQL всё равно собран).
func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	d, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=x dbname=x sslmode=disable"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := registerOmitBlobs(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOmitBlobs(t *testing.T) {
	d := dryDB(t)
	sqlOf := func(tx *gorm.DB) string { return tx.Statement.SQL.String() }
	var apps []models.CatalogApp
	var app models.CatalogApp
	var goods []*models.Good
	var g models.Good
	var exps []models.Expense
	for name, c := range map[string][2]string{
		"каталог списком":        {sqlOf(d.Where("enabled = ?", true).Find(&apps)), "catalog_apps"},
		"запись каталога":        {sqlOf(d.First(&app, "id = ?", "cs2")), "catalog_apps"},
		"товары (указатели)":     {sqlOf(d.Find(&goods)), "goods"},
		"товар":                  {sqlOf(d.First(&g, "id = ?", "x")), "goods"},
		"расходы":                {sqlOf(d.Find(&exps)), "expenses"},
		"товар через Model+Scan": {sqlOf(d.Model(&models.Good{}).Where("active").Scan(&goods)), "goods"},
	} {
		q, tbl := c[0], c[1]
		blob := strings.Contains(q, `."cover",`) || strings.Contains(q, `."cover" `) ||
			strings.Contains(q, `."photo",`) || strings.Contains(q, `."photo" `)
		if !strings.HasPrefix(q, `SELECT "`+tbl+`"."`) || strings.Contains(q, "*") || blob {
			t.Errorf("%s: без SELECT * и без байтов картинки, колонки модели — списком: %s", name, q)
		}
	}
	if q := sqlOf(d.First(&app, "id = ?", "cs2")); !strings.Contains(q, `"catalog_apps"."cover_at"`) ||
		!strings.Contains(q, `"catalog_apps"."cover_url"`) {
		t.Errorf("момент загрузки и ссылка на обложку — в списке колонок: %s", q)
	}
	var n int64
	var small []struct{ ID, Name string }
	var users []models.User
	for name, c := range map[string][2]string{
		"свой Select":       {sqlOf(d.Model(&models.CatalogApp{}).Select("id").Find(&apps)), `SELECT "id" FROM`},
		"Count":             {sqlOf(d.Model(&models.Good{}).Count(&n)), "SELECT count(*) FROM"},
		"меньшая структура": {sqlOf(d.Model(&models.Good{}).Find(&small)), `SELECT "goods"."id","goods"."name" FROM`},
		"сырой SQL":         {sqlOf(d.Raw("SELECT photo FROM goods").Scan(&g)), "SELECT photo FROM goods"},
		"другая модель":     {sqlOf(d.Find(&users)), `SELECT * FROM "users"`},
	} {
		if !strings.HasPrefix(c[0], c[1]) {
			t.Errorf("%s — как просили: ждали начало %q, получено %s", name, c[1], c[0])
		}
	}
}

func TestImageVersionAndCache(t *testing.T) {
	a, b := time.Unix(1700000000, 100e6), time.Unix(1700000000, 900e6) // одна секунда, разные миллисекунды
	if imageVersion(a) != "1700000000100" || imageVersion(a) == imageVersion(b) {
		t.Errorf("версия — в миллисекундах: %s, %s", imageVersion(a), imageVersion(b))
	}
	for _, c := range [][3]string{
		{"1700000000100", "1700000000100", "public, max-age=86400, immutable"},
		{"", "1700000000100", "public, max-age=300"},
		{"1700000000", "1700000000100", "public, max-age=300"},
	} {
		if got := imageCacheControl(c[0], c[1]); got != c[2] {
			t.Errorf("?v=%q при версии %s: %q, ждали %q", c[0], c[1], got, c[2])
		}
	}
}

func TestRateLimitImage(t *testing.T) {
	for _, c := range []struct {
		method, route string
		want          bool
	}{
		{"GET", "/api/v1/catalog/:id/cover", true}, {"GET", "/api/v1/goods/:id/photo", true},
		{"PUT", "/api/v1/admin/catalog/:id/cover", false}, {"GET", "/api/v1/admin/expenses/:id/photo", false},
		{"GET", "/api/v1/catalog", false}, {"POST", "/api/v1/goods/:id/photo", false},
	} {
		if got := rateLimitImage(c.method, c.route); got != c.want {
			t.Errorf("%s %s: вне лимитера %v, ждали %v", c.method, c.route, got, c.want)
		}
	}
}
