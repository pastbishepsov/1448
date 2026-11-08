package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestCanSetComputerStatus(t *testing.T) {
	cases := []struct {
		name string
		cur  models.ComputerStatus
		next models.ComputerStatus
		want bool
	}{
		{"свободный — в ремонт", models.ComputerStatusAvailable, models.ComputerStatusMaintenance, true},
		{"из ремонта — в строй", models.ComputerStatusMaintenance, models.ComputerStatusAvailable, true},
		{"занятый в ремонт нельзя", models.ComputerStatusInSession, models.ComputerStatusMaintenance, false},
		{"занятый в строй нельзя", models.ComputerStatusInSession, models.ComputerStatusAvailable, false},
		{"резерв в ремонт нельзя", models.ComputerStatusReserved, models.ComputerStatusMaintenance, false},
		{"повтор того же статуса — ок", models.ComputerStatusMaintenance, models.ComputerStatusMaintenance, true},
		{"в in_session руками нельзя", models.ComputerStatusAvailable, models.ComputerStatusInSession, false},
	}
	for _, tc := range cases {
		if got := canSetComputerStatus(tc.cur, tc.next); got != tc.want {
			t.Errorf("%s: canSetComputerStatus(%s→%s) = %v, ожидалось %v", tc.name, tc.cur, tc.next, got, tc.want)
		}
	}
}

func TestCanTargetUser(t *testing.T) {
	cases := []struct {
		name   string
		role   models.UserRole
		target string
		actor  string
		ok     bool
		code   string
	}{
		{"гостя — можно", models.UserRolePlayer, "u1", "a1", true, ""},
		{"админа нельзя", models.UserRoleAdmin, "u2", "a1", false, "cannot_touch_staff"},
		{"владельца нельзя", models.UserRoleOwner, "u3", "a1", false, "cannot_touch_staff"},
		{"самого себя нельзя", models.UserRoleAdmin, "a1", "a1", false, "cannot_touch_self"},
		{"self проверяется раньше роли", models.UserRolePlayer, "x1", "x1", false, "cannot_touch_self"},
	}
	for _, tc := range cases {
		ok, code := canTargetUser(tc.role, tc.target, tc.actor)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: canTargetUser(%s, %s, %s) = (%v, %q), ожидалось (%v, %q)",
				tc.name, tc.role, tc.target, tc.actor, ok, code, tc.ok, tc.code)
		}
	}
}

func TestAdminDayCapExceeded(t *testing.T) {
	cases := []struct {
		name            string
		used, add, cap  float64
		want            bool
	}{
		{"лимит 0 — не ограничен", 999999, 1000, 0, false},
		{"в пределах", 300, 100, 500, false},
		{"ровно в потолок — можно", 400, 100, 500, false},
		{"перебор", 450, 100, 500, true},
		{"первая же операция больше лимита", 0, 600, 500, true},
		{"кейсы (З0-и4): второй при потолке 2 — можно", 1, 1, 2, false},
		{"кейсы (З0-и4): третий при потолке 2 — нет", 2, 1, 2, true},
	}
	for _, tc := range cases {
		if got := adminDayCapExceeded(tc.used, tc.add, tc.cap); got != tc.want {
			t.Errorf("%s: adminDayCapExceeded(%v, %v, %v) = %v, ожидалось %v",
				tc.name, tc.used, tc.add, tc.cap, got, tc.want)
		}
	}
}

func TestRoleIsStaff(t *testing.T) {
	cases := map[string]bool{
		"admin":  true,
		"owner":  true,
		"player": false,
		"":       false, // старые токены без роли — не персонал
		"Admin":  false, // регистр важен: роли в БД только строчные
	}
	for role, want := range cases {
		if got := roleIsStaff(role); got != want {
			t.Errorf("roleIsStaff(%q) = %v, ожидалось %v", role, got, want)
		}
	}
}

// Ж3-и1: одно «сегодня» — клубные сутки от report_hour: 07:59 — ещё вчера,
// 08:00 — уже сегодня; ночная смена не рвётся полуночью.
func TestClubDayStartAt(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 9, 16, h, m, 0, 0, loc) }
	yesterday8 := time.Date(2026, 9, 15, 8, 0, 0, 0, loc)
	today8 := time.Date(2026, 9, 16, 8, 0, 0, 0, loc)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"00:30 — ещё вчерашние сутки", at(0, 30), yesterday8},
		{"07:59 — ещё вчера", at(7, 59), yesterday8},
		{"08:00 — уже сегодня", at(8, 0), today8},
		{"23:50 — сегодня", at(23, 50), today8},
	}
	for _, tc := range cases {
		if got := clubDayStartAt(8, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	// report_hour 0 — клубные сутки совпадают с календарными
	if got := clubDayStartAt(0, at(0, 30)); !got.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, loc)) {
		t.Errorf("report_hour 0: %v", got)
	}
}

// Ж5-и3: сортировка списка гостей — только из белого списка, направление
// по умолчанию зависит от столбца (ник — по алфавиту, числа и даты — с большего).
func TestUsersOrder(t *testing.T) {
	cases := []struct {
		name          string
		sort, dir     string
		wantCol, want string
		wantOK        bool
	}{
		{"пусто — свежие сверху", "", "", "last_active_at", "desc", true},
		{"ник — по алфавиту", "nickname", "", "LOWER(nickname)", "asc", true},
		{"кошелёк — с большего", "wallet", "", "wallet_grosz", "desc", true},
		{"монеты вверх — как просили", "coins", "asc", "coins_balance", "asc", true},
		{"дата регистрации", "created", "desc", "created_at", "desc", true},
		{"первое появление (З5-и1) — свежие сверху", "first_seen", "", firstSeenSQL, "desc", true},
		{"уровень", "level", "", "level", "desc", true},
		{"столбец вне списка", "password_hash", "", "", "", false},
		{"инъекция в столбце", "nickname; DROP TABLE users", "", "", "", false},
		{"направление вне списка", "wallet", "sideways", "", "", false},
	}
	for _, tc := range cases {
		col, _, dir, ok := usersOrder(tc.sort, tc.dir)
		if ok != tc.wantOK || col != tc.wantCol || dir != tc.want {
			t.Errorf("%s: usersOrder(%q,%q) = (%q,%q,%v), ожидалось (%q,%q,%v)",
				tc.name, tc.sort, tc.dir, col, dir, ok, tc.wantCol, tc.want, tc.wantOK)
		}
	}
}

// З15-и3 (№29): аккаунт персонала админу не показываем — ни в списке, ни карточкой; владельцу — всех.
func TestStaffHiddenFromAdmin(t *testing.T) {
	for _, c := range []struct {
		caller string
		target models.UserRole
		want   bool
	}{
		{"admin", models.UserRolePlayer, false},
		{"admin", models.UserRoleAdmin, true},
		{"admin", models.UserRoleOwner, true},
		{"admin", models.UserRole("all"), true},
		{"owner", models.UserRoleAdmin, false},
		{"owner", models.UserRoleOwner, false},
		{"owner", models.UserRole("all"), false},
		{"owner", models.UserRolePlayer, false},
	} {
		if got := staffHiddenFromAdmin(c.caller, c.target); got != c.want {
			t.Errorf("staffHiddenFromAdmin(%q, %q) = %v, want %v", c.caller, c.target, got, c.want)
		}
	}
}

// З15-и3 (№29): правки табеля владельцем — только ему; приход и уход видят все.
func TestAuditOwnerOnly(t *testing.T) {
	for action, want := range map[string]bool{
		"work_edit": true, "shift_close": true, "staff_dismiss": true, "settings_update": true, "expense_create": true,
		"work_start": false, "work_stop": false, "shift_assign": false, "pc_hold": false, "deposit": false,
	} {
		if got := auditOwnerOnly(action); got != want {
			t.Errorf("auditOwnerOnly(%q) = %v, want %v", action, got, want)
		}
	}
}

// З18-и4г: «сегодня» админа — клубные сутки, а если его смена началась раньше — с её начала, но не дальше суток назад.
func TestAdminFromAt(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, loc) }
	day := at(8, 0, 0) // сутки с полуночи 08.11
	ptr := func(x time.Time) *time.Time { return &x }
	cases := []struct {
		name  string
		shift *time.Time
		now   time.Time
		want  time.Time
	}{
		{"не на смене — клубные сутки", nil, at(8, 2, 0), day},
		{"ночная смена с 20:00 вчера, сейчас 02:00 — с 20:00", ptr(at(7, 20, 0)), at(8, 2, 0), at(7, 20, 0)},
		{"дневная смена с 08:00 — сутки, а не позже", ptr(at(8, 8, 0)), at(8, 10, 0), day},
		{"смена ровно с полуночи — сутки", ptr(day), at(8, 3, 0), day},
		{"забытая смена с 06.11 — не дальше суток назад", ptr(at(6, 9, 0)), at(8, 14, 0), at(7, 14, 0)},
		{"забытая смена, сейчас 00:30 — последние сутки", ptr(at(6, 9, 0)), at(8, 0, 30), at(7, 0, 30)},
	}
	for _, tc := range cases {
		if got := adminFromAt(day, tc.shift, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
