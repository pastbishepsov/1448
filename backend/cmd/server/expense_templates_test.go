package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З9-и5: срок шаблона — число месяца, нет такого — последний день.
func TestDueDay(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		month time.Time
		day   int
		want  time.Time
	}{
		{d(2026, 10, 1), 1, d(2026, 10, 1)},
		{d(2026, 9, 1), 31, d(2026, 9, 30)},
		{d(2026, 2, 1), 30, d(2026, 2, 28)},
		{d(2028, 2, 1), 31, d(2028, 2, 29)},
		{d(2026, 12, 1), 31, d(2026, 12, 31)},
	}
	for _, c := range cases {
		if got := dueDay(c.month, c.day); !got.Equal(c.want) {
			t.Errorf("dueDay(%s, %d) = %s, ждали %s", c.month.Format("2006-01"), c.day, got.Format("2006-01-02"), c.want.Format("2006-01-02"))
		}
	}
}

// З9-и5: с какого месяца ждёт шаблон и что ждёт сейчас.
func TestDueMonths(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	today := d(2026, 9, 28)
	if got := firstDueMonth(today, 28); !got.Equal(d(2026, 9, 1)) {
		t.Errorf("срок сегодня — с этого месяца: %s", got)
	}
	if got := firstDueMonth(today, 30); !got.Equal(d(2026, 9, 1)) {
		t.Errorf("срок впереди — с этого месяца: %s", got)
	}
	if got := firstDueMonth(today, 1); !got.Equal(d(2026, 10, 1)) {
		t.Errorf("срок прошёл — со следующего: %s", got)
	}
	keys := func(ms []time.Time) string {
		out := []string{}
		for _, m := range ms {
			out = append(out, monthKey(m))
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name   string
		from   time.Time
		day    int
		filled map[string]bool
		want   string
	}{
		{"срок сегодня", d(2026, 9, 1), 28, nil, "2026-09"},
		{"срок впереди — не ждёт", d(2026, 9, 1), 30, nil, ""},
		{"внесён — не ждёт", d(2026, 9, 1), 28, map[string]bool{"2026-09": true}, ""},
		{"пропущенные месяцы — по порядку", d(2026, 6, 1), 10, map[string]bool{"2026-07": true}, "2026-06,2026-08,2026-09"},
		{"первый месяц — следующий", d(2026, 10, 1), 1, nil, ""},
		{"не дальше года назад", d(2024, 1, 1), 1, nil,
			"2025-10,2025-11,2025-12,2026-01,2026-02,2026-03,2026-04,2026-05,2026-06,2026-07,2026-08,2026-09"},
	}
	for _, c := range cases {
		if got := keys(dueMonths(c.from, today, c.day, c.filled)); got != c.want {
			t.Errorf("%s: %q, ждали %q", c.name, got, c.want)
		}
	}
	// ближайший срок: ждущий, а нет — будущий
	if got := nextDue(d(2026, 6, 1), today, 10, nil); !got.Equal(d(2026, 6, 10)) {
		t.Errorf("ближайший — самый старый ждущий: %s", got)
	}
	if got := nextDue(d(2026, 9, 1), today, 28, map[string]bool{"2026-09": true}); !got.Equal(d(2026, 10, 28)) {
		t.Errorf("этот внесён — следующий месяц: %s", got)
	}
	if got := nextDue(d(2026, 10, 1), today, 1, nil); !got.Equal(d(2026, 10, 1)) {
		t.Errorf("первый месяц — следующий: %s", got)
	}
	if got := nextDue(d(2026, 9, 1), today, 30, nil); !got.Equal(d(2026, 9, 30)) {
		t.Errorf("срок впереди в этом месяце: %s", got)
	}
}

// З9-и5: поля шаблона — как у расхода, плюс название и число месяца.
func TestCheckTemplate(t *testing.T) {
	s := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	n := func(v int) *int { return &v }
	cat := uuid.New().String()
	ok := templateRequest{Name: s(" Аренда зала "), CategoryID: s(cat), AmountPLN: f(4500), Method: s("bank"), Day: n(1)}
	code, tp := checkTemplate(ok, models.ExpenseTemplate{})
	if code != "" || tp.Name != "Аренда зала" || tp.GrossGrosz != 450000 || tp.Day != 1 || tp.Method != "bank" {
		t.Errorf("верный шаблон: %q %+v", code, tp)
	}
	bad := []struct {
		req  templateRequest
		want string
	}{
		{templateRequest{Name: s(" "), CategoryID: s(cat), AmountPLN: f(1), Method: s("bank"), Day: n(1)}, "bad_template_name"},
		{templateRequest{Name: s(strings.Repeat("я", 65)), CategoryID: s(cat), AmountPLN: f(1), Method: s("bank"), Day: n(1)}, "bad_template_name"},
		{templateRequest{Name: s("x"), CategoryID: s("нет"), AmountPLN: f(1), Method: s("bank"), Day: n(1)}, "bad_category"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(0), Method: s("bank"), Day: n(1)}, "bad_amount"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(10), VatPLN: f(11), Method: s("bank"), Day: n(1)}, "bad_vat"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(10), Method: s("cash"), Day: n(1)}, "bad_method"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(10), Method: s("bank"), Day: n(0)}, "bad_day"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(10), Method: s("bank"), Day: n(32)}, "bad_day"},
		{templateRequest{Name: s("x"), CategoryID: s(cat), AmountPLN: f(10), Method: s("bank")}, "invalid_request"},
	}
	for _, b := range bad {
		if code, _ := checkTemplate(b.req, models.ExpenseTemplate{}); code != b.want {
			t.Errorf("%+v: %q, ждали %q", b.req, code, b.want)
		}
	}
	// правка — только присланное, VAT сверяется с итоговой суммой
	if code, t2 := checkTemplate(templateRequest{Day: n(31)}, tp); code != "" || t2.Day != 31 || t2.Name != "Аренда зала" {
		t.Errorf("правка числа: %q %+v", code, t2)
	}
	if code, _ := checkTemplate(templateRequest{VatPLN: f(4600)}, tp); code != "bad_vat" {
		t.Errorf("VAT больше суммы: %q", code)
	}
}

// З9-и5: строки журнала и напоминания.
func TestTemplateWords(t *testing.T) {
	cat := uuid.New()
	a := models.ExpenseTemplate{Name: "Аренда зала", CategoryID: cat, GrossGrosz: 450000, VatGrosz: 84146, Method: "bank", Day: 1, Active: true}
	if got := templateLine(&a, "Аренда"); got != "«Аренда зала» · Аренда · 4500,00 zł (VAT 841,46) · банк · 1-го числа" {
		t.Errorf("строка журнала: %q", got)
	}
	b := a
	b.GrossGrosz, b.Day, b.Active = 470000, 5, false
	if got := strings.Join(templateChanges(a, b, map[string]string{cat.String(): "Аренда"}), "; "); got != "сумма 4500,00 → 4700,00 zł; число 1 → 5; выключен" {
		t.Errorf("правка: %q", got)
	}
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if got := dueMessage(&a, day, day); !strings.Contains(got, "«Аренда зала» — 4500,00 zł, банк; срок сегодня, 01.10") {
		t.Errorf("напоминание в срок: %q", got)
	}
	if got := dueMessage(&a, day, day.AddDate(0, 0, 2)); !strings.Contains(got, "срок был 01.10") {
		t.Errorf("напоминание после срока: %q", got)
	}
}
