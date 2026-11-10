package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З18-и5: часы работы из формы — канонический JSON по порядку недели или фраза об ошибке.
func TestClubHoursParse(t *testing.T) {
	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	ok := []struct{ in, want string }{
		{`null`, "<nil>"},
		{` null `, "<nil>"},
		{`{"sun": {"open": "12:00", "close": "02:00"}, "mon": {"open": " 10:00 ", "close": "23:00"}}`,
			`{"mon": {"open": "10:00", "close": "23:00"}, "sun": {"open": "12:00", "close": "02:00"}}`},
		{`{"mon": {"open": "00:00", "close": "24:00"}, "tue": null}`, `{"mon": {"open": "00:00", "close": "24:00"}}`},
	}
	for _, c := range ok {
		got, msg := clubHoursParse(json.RawMessage(c.in))
		if msg != "" || str(got) != c.want {
			t.Errorf("%s: %s (%q), ждали %s", c.in, str(got), msg, c.want)
		}
	}
	bad := []struct{ in, word string }{
		{`[]`, "по дням"},
		{`"круглосуточно"`, "по дням"},
		{`{}`, "хотя бы один"},
		{`{"mon": null}`, "хотя бы один"},
		{`{"xyz": {"open": "10:00", "close": "23:00"}}`, "неизвестный день «xyz»"},
		{`{"mon": {"open": "25:00", "close": "23:00"}}`, "пн:"},
		{`{"tue": {"open": "10:00", "close": "10:00"}}`, "вт:"},
		{`{"wed": {"open": "24:00", "close": "10:00"}}`, "ср:"},
		{`{"thu": {"open": "9:00", "close": "18:00"}}`, "чт:"},
		{`{"fri": {"open": "10:00"}}`, "пт:"},
	}
	for _, c := range bad {
		got, msg := clubHoursParse(json.RawMessage(c.in))
		if got != nil || !strings.Contains(msg, c.word) {
			t.Errorf("%s: %s, %q — ждали отказ со словами «%s»", c.in, str(got), msg, c.word)
		}
	}
}

// З18-и5: часы словами для журнала — подряд одинаковые дни одним отрезком, выходные списком.
func TestClubHoursText(t *testing.T) {
	canon := func(in string) *string {
		p, msg := clubHoursParse(json.RawMessage(in))
		if msg != "" {
			t.Fatalf("%s: %s", in, msg)
		}
		return p
	}
	day := `{"open": "10:00", "close": "23:00"}`
	cases := []struct {
		h    *string
		want string
	}{
		{nil, "круглосуточно"},
		{canon(`{"mon": ` + day + `, "tue": ` + day + `, "wed": ` + day + `, "thu": ` + day + `, "fri": ` + day +
			`, "sat": {"open": "12:00", "close": "02:00"}}`), "пн–пт 10:00–23:00, сб 12:00–02:00, выходной: вс"},
		{canon(`{"mon": ` + day + `, "wed": ` + day + `}`), "пн 10:00–23:00, ср 10:00–23:00, выходной: вт, чт, пт, сб, вс"},
		{canon(`{"mon": {"open": "00:00", "close": "24:00"}, "tue": {"open": "00:00", "close": "24:00"}, "wed": {"open": "00:00", "close": "24:00"}, ` +
			`"thu": {"open": "00:00", "close": "24:00"}, "fri": {"open": "00:00", "close": "24:00"}, "sat": {"open": "00:00", "close": "24:00"}, ` +
			`"sun": {"open": "00:00", "close": "24:00"}}`), "пн–вс 00:00–24:00"},
	}
	for _, c := range cases {
		if got := clubHoursText(c.h); got != c.want {
			t.Errorf("%v: «%s», ждали «%s»", c.h, got, c.want)
		}
	}
	// jsonb отдаёт ключи в своём порядке — «было» сравнивается в том же каноническом виде
	db := `{"fri": {"close": "23:00", "open": "10:00"}, "mon": {"open": "10:00", "close": "23:00"}}`
	if got := clubHoursCanon(&db); got == nil || *got != `{"mon": {"open": "10:00", "close": "23:00"}, "fri": {"open": "10:00", "close": "23:00"}}` {
		t.Errorf("clubHoursCanon: %v", got)
	}
}

// З18-и5: телефон, Telegram и Instagram из формы — без пробелов по краям, без «@» и ссылки; пусто — стереть.
func TestClubContact(t *testing.T) {
	ok := []struct{ kind, in, want string }{
		{"phone", " +48 22 123-45-67 ", "+48 22 123-45-67"},
		{"phone", "(22) 123 45 67", "(22) 123 45 67"},
		{"phone", "", "<nil>"},
		{"telegram", "@club_1448", "club_1448"},
		{"telegram", "https://t.me/club_1448", "club_1448"},
		{"telegram", "T.me/@Club1448", "Club1448"},
		{"telegram", "  ", "<nil>"},
		{"instagram", "https://www.instagram.com/club.1448/", "club.1448"},
		{"instagram", "@club_1448", "club_1448"},
	}
	for _, c := range ok {
		got, good := clubContact(c.kind, c.in)
		s := "<nil>"
		if got != nil {
			s = *got
		}
		if !good || s != c.want {
			t.Errorf("%s «%s»: %v «%s», ждали «%s»", c.kind, c.in, good, s, c.want)
		}
	}
	bad := []struct{ kind, in string }{
		{"phone", "abc"}, {"phone", "12345"}, {"phone", "+48 22 123 45 67 89 01 2"}, {"phone", "48+22 1234567"},
		{"telegram", "ab"}, {"telegram", "club-1448"}, {"telegram", "https://t.me/"},
		{"instagram", "club 1448"}, {"instagram", "клуб"},
	}
	for _, c := range bad {
		if got, good := clubContact(c.kind, c.in); good {
			t.Errorf("%s «%s»: принят как %v", c.kind, c.in, got)
		}
	}
}

// З18-и5 (из №40): наружу — только белый список полей клуба и ПК.
func TestPublicClubWhitelist(t *testing.T) {
	keys := func(m map[string]any) string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	h := `{"mon": {"open": "10:00", "close": "23:00"}}`
	cl := publicClub(models.Club{Name: "14:48", RTPModifier: 1.2, LayoutW: 12, LayoutH: 8, IsActive: true, WorkingHours: &h})
	if got := keys(cl); got != "address,base_rate_pln,id,instagram,latitude,longitude,name,phone,telegram,working_hours" {
		t.Errorf("publicClub: %s", got)
	}
	if b, _ := json.Marshal(cl["working_hours"]); string(b) != `{"mon":{"open":"10:00","close":"23:00"}}` {
		t.Errorf("часы — объектом: %s", b)
	}
	if b, _ := json.Marshal(publicClub(models.Club{})["working_hours"]); string(b) != "null" {
		t.Errorf("без часов — null: %s", b)
	}
	mac := "AA:BB:CC:DD:EE:FF"
	pc := publicComputer(models.Computer{Name: "PC-01", Zone: "VIP", MAC: &mac})
	if got := keys(pc); got != "id,name,pos_x,pos_y,status,zone" {
		t.Errorf("publicComputer: %s", got)
	}
}
