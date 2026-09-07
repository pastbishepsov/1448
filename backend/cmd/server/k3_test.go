package main

// К3 «Данные и тексты» (KIOSK.md): чистые функции спринта.

import (
	"strings"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// и1: калькулятор и начисление считают по одной формуле из настроек.
func TestSessionRewardsFollowSettings(t *testing.T) {
	r := economyRateSet{XPPerMin: 13, CoinsPerMin: 7, CoinsPerPLN: 10} // владелец поменял ставки
	cases := []struct {
		name         string
		minutes      int
		boost        float64
		rank         AccountRank
		wantXP, want int64
	}{
		// час без модификаторов: 60×13 = 780, 60×7 = 420 — кратно 5 как есть
		{"час, новичок", 60, 0, AccountRank{XPMult: 1, CoinMult: 1}, 780, 420},
		// ранг ×1.2: 936 → 935, 504 → 505 (округление до пятёрки, как r5 на клиенте)
		{"час, ранг ×1.2", 60, 0, AccountRank{XPMult: 1.2, CoinMult: 1.2}, 935, 505},
		// талант +10 % к XP: 780×1.1 = 858 → 860; монеты талант не трогает
		{"час, xp_boost 0.1", 60, 0.1, AccountRank{XPMult: 1, CoinMult: 1}, 860, 420},
		// 17 минут: 221 → 220, 119 → 120
		{"17 минут", 17, 0, AccountRank{XPMult: 1, CoinMult: 1}, 220, 120},
		{"ноль минут", 0, 0.1, AccountRank{XPMult: 1.5, CoinMult: 1.5}, 0, 0},
	}
	for _, tc := range cases {
		xp, coins := sessionRewards(tc.minutes, r, tc.boost, tc.rank)
		if xp != tc.wantXP || coins != tc.want {
			t.Errorf("%s: sessionRewards = %d XP / %d монет, ожидалось %d / %d", tc.name, xp, coins, tc.wantXP, tc.want)
		}
	}
	// формула калькулятора на клиенте: r5(m × rate × mult) — та же пятёрка
	if got, _ := sessionRewards(60, r, 0, AccountRank{XPMult: 1.2, CoinMult: 1}); got != models.RoundToStep(936, 5) {
		t.Errorf("сервер и клиент разошлись: %d vs %d", got, models.RoundToStep(936, 5))
	}
}

// и3: ошибки валидатора — код поля + русская фраза.
func TestFieldError(t *testing.T) {
	cases := []struct {
		field, tag, param, kind string
		wantCode, wantMsg       string
	}{
		{"Password", "min", "6", "string", "password_short", "Пароль — минимум 6 символов"},
		{"Password", "max", "72", "string", "password_long", "Пароль — максимум 72 символа"},
		{"Nickname", "min", "3", "string", "nickname_short", "Ник — минимум 3 символа"},
		{"Nickname", "max", "32", "string", "nickname_long", "Ник — максимум 32 символа"},
		{"Nickname", "required", "", "string", "nickname_required", "Не хватает поля «ник»"},
		{"Email", "email", "", "string", "email_invalid", "Email выглядит неправильно — проверь адрес"},
		{"PlannedMin", "min", "15", "int", "planned_min_min", "Минуты — минимум 15"},
		{"NewPassword", "min", "6", "string", "new_password_short", "Новый пароль — минимум 6 символов"},
		{"Weird", "len", "4", "string", "weird_invalid", "weird — неверное значение"},
	}
	for _, tc := range cases {
		code, msg := fieldError(tc.field, tc.tag, tc.param, tc.kind)
		if code != tc.wantCode || msg != tc.wantMsg {
			t.Errorf("%s/%s: получили %q %q, ожидалось %q %q", tc.field, tc.tag, code, msg, tc.wantCode, tc.wantMsg)
		}
	}
}

func TestBindErrorFallback(t *testing.T) {
	code, msg := bindError(errBadJSON{})
	if code != "invalid_request" || !strings.Contains(msg, "Не удалось прочитать запрос") {
		t.Errorf("не-валидаторная ошибка: %q %q", code, msg)
	}
}

type errBadJSON struct{}

func (errBadJSON) Error() string { return "unexpected end of JSON input" }

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{1: "символ", 2: "символа", 3: "символа", 4: "символа", 5: "символов", 6: "символов", 11: "символов", 21: "символ", 32: "символа", 72: "символа", 100: "символов"} {
		if got := plural(n, "символ", "символа", "символов"); got != want {
			t.Errorf("%d: %s, ожидалось %s", n, got, want)
		}
	}
}

func TestWalletLowMessage(t *testing.T) {
	got := walletLowMessage(15, 5.75, 2.61, 0, 0)
	want := "Не хватает на старт: нужно ~5,75 zł на 15 мин, на кошельке 2,61 zł. Пополни у стойки"
	if got != want {
		t.Errorf("без запасов:\n%s\nожидалось\n%s", got, want)
	}
	got = walletLowMessage(15, 5.75, 0, 3, 40)
	if !strings.Contains(got, "на кошельке 0,00 zł + 3 мин монетами + 40 мин пакетом.") {
		t.Errorf("с запасами: %s", got)
	}
}

// и2: условие «непрочитано этим клиентом».
func TestNotifUnreadCond(t *testing.T) {
	if notifClient("kiosk") != "kiosk" || notifClient("app") != "app" || notifClient("tv") != "" || notifClient("") != "" {
		t.Error("notifClient нормализует не так")
	}
	if c := notifUnreadCond("kiosk"); c != "read_kiosk_at IS NULL AND read_at IS NULL" {
		t.Errorf("kiosk: %s", c)
	}
	if c := notifUnreadCond("app"); c != "read_app_at IS NULL AND read_at IS NULL" {
		t.Errorf("app: %s", c)
	}
	if c := notifUnreadCond(""); c != "read_at IS NULL" {
		t.Errorf("без клиента: %s", c)
	}
}

// и5: DESC-выборка чата разворачивается в «старые сверху».
func TestReverseMessages(t *testing.T) {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	msgs := []models.ChatMessage{
		{Text: "3", CreatedAt: base.Add(3 * time.Minute)},
		{Text: "2", CreatedAt: base.Add(2 * time.Minute)},
		{Text: "1", CreatedAt: base.Add(1 * time.Minute)},
	}
	reverseMessages(msgs)
	if msgs[0].Text != "1" || msgs[1].Text != "2" || msgs[2].Text != "3" {
		t.Errorf("порядок после разворота: %s %s %s", msgs[0].Text, msgs[1].Text, msgs[2].Text)
	}
	reverseMessages(nil) // пустая выборка не должна паниковать
}

// К4-и4: пауза выключена по умолчанию, AFK — предупреждение за 5 мин, на пороге конец.
func TestAfkDecisionNoPause(t *testing.T) {
	if pauseLimitMinDef != 0 || afkStopMinDef != 25 {
		t.Fatalf("дефолты: пауза %d (ожидалось 0), AFK %d (ожидалось 25)", pauseLimitMinDef, afkStopMinDef)
	}
	if afkWarnSec(25) != 20*60 || afkWarnSec(10) != 5*60 || afkWarnSec(3) != 90 {
		t.Errorf("afkWarnSec: 25→%d 10→%d 3→%d", afkWarnSec(25), afkWarnSec(10), afkWarnSec(3))
	}
	cases := []struct {
		name       string
		idle       int
		stop       int64
		pauseAvail bool
		warned     bool
		want       afkAct
	}{
		{"выключено", 9999, 0, false, false, afkNone},
		{"19 минут тишины — рано", 19 * 60, 25, false, false, afkNone},
		{"20 минут — предупредить", 20 * 60, 25, false, false, afkWarn},
		{"22 минуты, уже предупреждали — молчим", 22 * 60, 25, false, true, afkNone},
		{"25 минут, паузы нет — завершить", 25 * 60, 25, false, true, afkEnd},
		{"25 минут без предупреждения (пропущен тик) — всё равно завершить", 25 * 60, 25, false, false, afkEnd},
		{"25 минут, пауза включена и бюджет есть — автопауза", 25 * 60, 25, true, true, afkPause},
		{"порог 10 у другого клуба: 5 минут — предупредить", 5 * 60, 10, false, false, afkWarn},
	}
	for _, tc := range cases {
		if got := afkDecision(tc.idle, tc.stop, tc.pauseAvail, tc.warned); got != tc.want {
			t.Errorf("%s: %d, ожидалось %d", tc.name, got, tc.want)
		}
	}
}

// К4-и3: токен агента и публичный каталог без путей.
func TestAgentTokenAndCatalogStrip(t *testing.T) {
	if !agentTokenMatches("", "anything") || !agentTokenMatches("", "") {
		t.Error("пустой серверный токен = открыто")
	}
	if agentTokenMatches("abc", "") || agentTokenMatches("abc", "abd") || !agentTokenMatches("abc", "abc") {
		t.Error("заданный токен сравнивается точно")
	}
	for s, want := range map[string]bool{
		`D:\online\CS2\cs2.exe`: true, `C:/Games/x.exe`: true, `\\nas\games\a.exe`: true, `{GAMES}\CS2\cs2.exe`: true,
		`steam://rungameid/730`: false, `https://web.telegram.org`: false, `ms-settings:mouse`: false, ``: false, `D:`: false,
	} {
		if got := isDiskPath(s); got != want {
			t.Errorf("isDiskPath(%q) = %v, ожидалось %v", s, got, want)
		}
	}
	disk, args, proto := `D:\online\CS2\cs2.exe`, `["-novid"]`, `steam://rungameid/730`
	a := models.CatalogApp{Target: &disk, Args: &args, TargetRaw: `{GAMES}\CS2\cs2.exe`}
	stripCatalogSecrets(&a)
	if a.Target != nil || a.Args != nil || a.TargetRaw != "" {
		t.Errorf("путь/аргументы/шаблон должны исчезнуть: %+v", a)
	}
	b := models.CatalogApp{Target: &proto, Args: &args}
	stripCatalogSecrets(&b)
	if b.Target == nil || *b.Target != proto || b.Args != nil {
		t.Errorf("протокол остаётся, аргументы — нет: %+v", b)
	}
}
