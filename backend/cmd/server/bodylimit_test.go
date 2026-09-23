package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// З0-и2 (№32): потолок тела — у всех ручек, включая /admin/goods*. Заявленный
// размер больше 1 МБ — 413 до ручки; без заявленного размера ручка читает не
// больше потолка; обычное тело проходит.
func TestBodyLimitAppliesToEveryRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(bodyLimitMiddleware())
	read := func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "read_cut", "n": len(b)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"n": len(b)})
	}
	r.POST("/api/v1/admin/goods/:id/stock", read)
	r.POST("/api/v1/admin/zones", read)

	big := bytes.Repeat([]byte("a"), 3<<20)
	for _, path := range []string{"/api/v1/admin/goods/x/stock", "/api/v1/admin/zones"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(big)))
		if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "body_too_large") {
			t.Errorf("%s, заявлено 3 МБ: %d %s", path, w.Code, w.Body.String())
		}
		req := httptest.NewRequest(http.MethodPost, path, io.NopCloser(bytes.NewReader(big)))
		req.ContentLength = -1 // chunked: размер не заявлен
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "read_cut") {
			t.Errorf("%s, без заявленного размера: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/goods/x/stock", strings.NewReader(`{"delta":1}`)))
	if w.Code != http.StatusOK {
		t.Errorf("обычное тело: %d %s", w.Code, w.Body.String())
	}
}
