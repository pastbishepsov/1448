package main

// Картинки из базы (З18-и3, OWNER.md; ревью 23.09 №35, №36, ETag — из №40). Обложки плиток (catalog_apps.cover), фото
// позиций кухни (goods.photo) и документов расходов (expenses.photo) лежат BYTEA в тех же таблицах, что и записи.
//
// Списки: модели байтов не знают, но GORM без Select пишет SELECT *, и каждое чтение каталога или товаров тянуло из
// Postgres байты всех картинок (30 обложек по 400 КБ — 12 МБ на каждый экран киоска). omitBlobs перечисляет колонки
// модели явно — без картинки.
//
// Отдача: ETag и версия в адресе (?v=) — момент загрузки в миллисекундах (в секундах перезалитая в ту же секунду
// картинка оставалась старой); по адресу с актуальной версией — кэш на сутки, повтор с ETag — 304 без чтения байтов.

import (
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// blobColumns — BYTEA-колонка таблицы модели: байты читает и пишет только сырой SQL.
var blobColumns = map[reflect.Type]string{
	reflect.TypeOf(models.CatalogApp{}): "cover",
	reflect.TypeOf(models.Good{}):       "photo",
	reflect.TypeOf(models.Expense{}):    "photo",
}

// omitBlobs — колбэк перед чтением: модель с картинкой в таблице читается списком своих колонок, а не SELECT *.
// Запрос со своим Select или Omit, сырой SQL и чтение в другую («меньшую») структуру — как есть: там SELECT * нет.
func omitBlobs(d *gorm.DB) {
	st := d.Statement
	if st.Schema == nil || st.SQL.Len() > 0 || len(st.Selects) > 0 || len(st.Omits) > 0 || !st.ReflectValue.IsValid() {
		return
	}
	col, ok := blobColumns[st.Schema.ModelType]
	if !ok {
		return
	}
	t := st.ReflectValue.Type()
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == st.Schema.ModelType {
		st.Omits = append(st.Omits, col)
	}
}

// registerOmitBlobs — omitBlobs на Find/First и на Scan/Rows (main.go — сразу после подключения к базе).
func registerOmitBlobs(d *gorm.DB) error {
	if err := d.Callback().Query().Before("gorm:query").Register("1448:omit_blobs", omitBlobs); err != nil {
		return err
	}
	return d.Callback().Row().Before("gorm:row").Register("1448:omit_blobs_row", omitBlobs)
}

// imageVersion — версия картинки для ?v= и ETag: момент загрузки в миллисекундах.
func imageVersion(at time.Time) string { return strconv.FormatInt(at.UnixMilli(), 10) }

// imageCacheControl — по адресу с актуальной версией картинка не меняется: сутки без перепроверки. Без версии или со
// старой — 5 минут, как было.
func imageCacheControl(v, ver string) string {
	if v != "" && v == ver {
		return "public, max-age=86400, immutable"
	}
	return "public, max-age=300"
}

// dbImage — где лежит картинка: таблица, колонки байтов, типа и момента загрузки; tag — буква в ETag.
type dbImage struct{ table, bytes, ctype, at, tag string }

var (
	coverImage = dbImage{"catalog_apps", "cover", "cover_type", "cover_at", "c"}
	photoImage = dbImage{"goods", "photo", "photo_type", "photo_at", "g"}
)

// serve — публичный GET картинки: тип и момент загрузки (ETag) — первым запросом, 304 — без байтов; байты — вторым,
// только для ответа 200.
func (im dbImage) serve(c *gin.Context, id string) {
	var meta struct {
		Type *string
		At   *time.Time
	}
	err := db.Raw("SELECT "+im.ctype+" AS type, "+im.at+" AS at FROM "+im.table+" WHERE id = ? AND "+im.bytes+
		" IS NOT NULL", id).Scan(&meta).Error
	if err != nil || meta.At == nil {
		c.Status(http.StatusNotFound)
		return
	}
	ver := imageVersion(*meta.At)
	etag := `"` + im.tag + ver + `"`
	c.Header("Cache-Control", imageCacheControl(c.Query("v"), ver))
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	var data []byte
	if db.Raw("SELECT "+im.bytes+" FROM "+im.table+" WHERE id = ?", id).Row().Scan(&data) != nil || len(data) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	ct := "image/jpeg"
	if meta.Type != nil && *meta.Type != "" {
		ct = *meta.Type
	}
	c.Data(http.StatusOK, ct, data)
}
