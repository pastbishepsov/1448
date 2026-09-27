package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З9-и1: поля расхода — на создании обязательны дата, сумма и способ; правка
// меняет только присланное, VAT сверяется с итоговой суммой.
func TestCheckExpense(t *testing.T) {
	today := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	s := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }
	ok := expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(3000), VatPLN: f(561), Method: s("bank"), DocNo: s(" FV 12/09 "), Note: s("сентябрь")}
	code, e := checkExpense(ok, models.Expense{Paid: true}, today)
	if code != "" || e.GrossGrosz != 300000 || e.VatGrosz != 56100 || e.Method != "bank" || e.DocNo != "FV 12/09" || !e.Paid ||
		e.OpDate.Format("2006-01-02") != "2026-09-20" {
		t.Fatalf("годный расход: %q %+v", code, e)
	}
	bad := []struct {
		req  expenseRequest
		want string
	}{
		{expenseRequest{OpDate: s("20.09.2026"), AmountPLN: f(10), Method: s("bank")}, "bad_date"},
		{expenseRequest{OpDate: s("2021-09-26"), AmountPLN: f(10), Method: s("bank")}, "bad_date"}, // дальше пяти лет
		{expenseRequest{OpDate: s("2027-09-28"), AmountPLN: f(10), Method: s("bank")}, "bad_date"}, // дальше года вперёд
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(0), Method: s("bank")}, "bad_amount"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(-5), Method: s("bank")}, "bad_amount"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10.005), Method: s("bank")}, "bad_amount"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(1_000_000.01), Method: s("bank")}, "bad_amount"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), VatPLN: f(10.01), Method: s("bank")}, "bad_vat"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), VatPLN: f(-1), Method: s("bank")}, "bad_vat"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), VatPLN: f(1.234), Method: s("bank")}, "bad_vat"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), Method: s("cash")}, "bad_method"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), Method: s("bank"), DocNo: s(strings.Repeat("я", 65))}, "bad_doc"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10), Method: s("bank"), Note: s(strings.Repeat("я", 501))}, "bad_note"},
		{expenseRequest{AmountPLN: f(10), Method: s("bank")}, "invalid_request"},
		{expenseRequest{OpDate: s("2026-09-20"), Method: s("bank")}, "invalid_request"},
		{expenseRequest{OpDate: s("2026-09-20"), AmountPLN: f(10)}, "invalid_request"},
	}
	for _, c := range bad {
		if code, _ := checkExpense(c.req, models.Expense{Paid: true}, today); code != c.want {
			t.Errorf("%+v: код %q, ждали %q", c.req, code, c.want)
		}
	}
	// правка: сумма ниже прежнего VAT — отказ; «не оплачен» — меняется только он
	if code, _ := checkExpense(expenseRequest{AmountPLN: f(500)}, e, today); code != "bad_vat" {
		t.Errorf("сумма ниже VAT: %q", code)
	}
	code, e2 := checkExpense(expenseRequest{Paid: b(false)}, e, today)
	if code != "" || e2.Paid || e2.GrossGrosz != e.GrossGrosz || e2.DocNo != e.DocNo {
		t.Errorf("правка «не оплачен»: %q %+v", code, e2)
	}
}

func TestNormNIP(t *testing.T) {
	for in, want := range map[string]string{"123-456-78-90": "1234567890", " 1234567890 ": "1234567890", "": ""} {
		if got, ok := normNIP(in); !ok || got != want {
			t.Errorf("normNIP(%q) = %q %v, ждали %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"12345", "12345678901", "12345abcde"} {
		if _, ok := normNIP(in); ok {
			t.Errorf("normNIP(%q) принят", in)
		}
	}
}

// Журнал правки — только изменённое, «было → стало», с именами.
func TestExpenseChanges(t *testing.T) {
	cat1, cat2, cp := uuid.New(), uuid.New(), uuid.New()
	names := map[string]string{cat1.String(): "Аренда", cat2.String(): "Прочее", cp.String(): "Landlord"}
	a := models.Expense{OpDate: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), CategoryID: cat1, CounterpartyID: &cp,
		GrossGrosz: 300000, Method: "bank", Paid: true, DocNo: "FV 1"}
	b := a
	b.OpDate, b.CategoryID, b.CounterpartyID, b.GrossGrosz, b.Method, b.Paid, b.Note = a.OpDate.AddDate(0, 0, 1), cat2, nil, 310000, "card", false, "x"
	got := strings.Join(expenseChanges(a, b, names), "; ")
	want := "дата 20.09.2026 → 21.09.2026; категория Аренда → Прочее; контрагент Landlord → —; сумма 3000.00 → 3100.00 zł; " +
		"способ банк → карта; оплачен → не оплачен; заметка изменена"
	if got != want {
		t.Errorf("изменения:\n %s\nждали\n %s", got, want)
	}
	if n := len(expenseChanges(a, a, names)); n != 0 {
		t.Errorf("без правок — %d изменений", n)
	}
	line := expenseLine(&b, "Прочее")
	if line != "Прочее · 3100.00 zł · карта · 21.09.2026 · не оплачен · FV 1" {
		t.Errorf("строка журнала: %q", line)
	}
}
