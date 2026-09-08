package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// К4-и2: нормализация запроса тоста.
func TestNormalizeNotify(t *testing.T) {
	if _, ok := normalizeNotify(notifyRequest{Text: "   "}); ok {
		t.Error("пустой текст должен отклоняться")
	}
	r, ok := normalizeNotify(notifyRequest{Text: " через 5 мин закончатся деньги "})
	if !ok || r.Title != "14:48" || r.Text != "через 5 мин закончатся деньги" {
		t.Errorf("дефолтный заголовок/трим: %+v", r)
	}
	long := strings.Repeat("я", 300)
	r, _ = normalizeNotify(notifyRequest{Title: strings.Repeat("т", 100), Text: long})
	if rl := []rune(r.Text); len(rl) != notifyMaxText || rl[len(rl)-1] != '…' {
		t.Errorf("текст режется по рунам до %d с многоточием: %d", notifyMaxText, len(rl))
	}
	if len([]rune(r.Title)) != notifyMaxTitle {
		t.Errorf("заголовок режется до %d: %d", notifyMaxTitle, len([]rune(r.Title)))
	}
}

// Дроссель: два тоста подряд — второй молча пропускается; выключено — ничего.
func TestNotifierThrottleAndDisable(t *testing.T) {
	var shown []string
	n := newNotifier(NotifyConfig{})
	n.show = func(appID, title, text string) error {
		if appID != powershellAUMID {
			t.Errorf("AUMID по умолчанию — PowerShell, получили %q", appID)
		}
		shown = append(shown, text)
		return nil
	}
	t0 := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if ok, _ := n.notify(notifyRequest{Text: "первый"}, t0); !ok {
		t.Error("первый тост должен показаться")
	}
	if ok, _ := n.notify(notifyRequest{Text: "второй сразу"}, t0.Add(500*time.Millisecond)); ok {
		t.Error("тост через 0,5 с — спам, пропускаем")
	}
	if ok, _ := n.notify(notifyRequest{Text: "третий позже"}, t0.Add(3*time.Second)); !ok {
		t.Error("через 3 с — снова можно")
	}
	if len(shown) != 2 || shown[0] != "первый" || shown[1] != "третий позже" {
		t.Errorf("показано: %v", shown)
	}
	off := false
	n2 := newNotifier(NotifyConfig{Enabled: &off, AppID: "My.App"})
	n2.show = func(string, string, string) error {
		t.Error("выключено — показывать нельзя")
		return nil
	}
	if ok, _ := n2.notify(notifyRequest{Text: "x"}, t0); ok {
		t.Error("выключенный notifier вернул shown=true")
	}
	if n2.cfg.appID() != "My.App" {
		t.Error("свой AUMID из конфига не применился")
	}
	n3 := newNotifier(NotifyConfig{})
	n3.show = func(string, string, string) error { return errors.New("powershell missing") }
	if _, err := n3.notify(notifyRequest{Text: "x"}, t0); err == nil {
		t.Error("ошибка платформы должна возвращаться наружу")
	}
}

// Скрипт тоста: XML-экранирование и кавычки PowerShell — иначе текст с
// апострофом или «<» ломает скрипт.
func TestToastScriptEscapes(t *testing.T) {
	s := toastScript(`{1AC14E77}\powershell.exe`, "14:48", `Bob's <burger> & fries`)
	if !strings.Contains(s, "Bob&#39;s &lt;burger&gt; &amp; fries") {
		t.Errorf("XML-экранирование: %s", s)
	}
	if !strings.Contains(s, `CreateToastNotifier('{1AC14E77}\powershell.exe')`) {
		t.Errorf("AUMID в скрипте: %s", s)
	}
	if s2 := toastScript(`It's.App`, "t", "x"); !strings.Contains(s2, `CreateToastNotifier('It''s.App')`) {
		t.Errorf("апостроф в строке PowerShell удваивается: %s", s2)
	}
	if !strings.Contains(s, `template="ToastGeneric"`) || !strings.HasSuffix(s, "\r\n") {
		t.Errorf("формат скрипта: %q", s[len(s)-20:])
	}
}
