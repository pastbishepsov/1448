package main

// К4-и2 (KIOSK.md, №28): уведомления гостю поверх игры.
//
// Гость в матче не видит экран киоска: тосты «через 5 минут закончатся
// деньги», «твоя бронь через 10 минут», «бургер несут», «ответил админ» тонут
// за игрой. Гостевой экран дублирует КРИТИЧНЫЕ из них сюда — POST /notify
// {title, text, kind} — а агент показывает системный тост Windows (WinRT
// ToastNotification через встроенный PowerShell, без сторонних модулей;
// notify_windows.go). Не на Windows — строка в лог (notify_other.go).
//
// Честно про полноэкранные игры: эксклюзивный fullscreen тосты Windows
// прячет (см. README агента); borderless/оконный режим — показывает.
// Текст агент не знает и не переводит — локализация словарями шелла.

import (
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// NotifyConfig — блок "notify" в agent.json.
type NotifyConfig struct {
	Enabled *bool  `json:"enabled"` // nil = включено
	AppID   string `json:"app_id"`  // AUMID отправителя тоста; пусто = встроенный PowerShell
}

// powershellAUMID — зарегистрированный в любом Windows отправитель: тост
// от него показывается без установки приложения и без своего ярлыка.
const powershellAUMID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

const (
	notifyMaxTitle = 60
	notifyMaxText  = 200
	notifyMinGap   = 1500 * time.Millisecond // чаще — не тост, а спам; лишнее склеивается на экране
)

func (c NotifyConfig) enabled() bool { return c.Enabled == nil || *c.Enabled }

func (c NotifyConfig) appID() string {
	if c.AppID != "" {
		return c.AppID
	}
	return powershellAUMID
}

type notifyRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Kind  string `json:"kind"` // balance_low | booking_soon | kitchen | admin_reply | … — для лога
}

// normalizeNotify — обрезать и подчистить (чистая, тест): пустой текст —
// отказ, длинное режется по рунам, заголовок по умолчанию «14:48».
func normalizeNotify(req notifyRequest) (notifyRequest, bool) {
	req.Title = strings.TrimSpace(req.Title)
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return req, false
	}
	if req.Title == "" {
		req.Title = "14:48"
	}
	req.Title = cutRunes(req.Title, notifyMaxTitle)
	req.Text = cutRunes(req.Text, notifyMaxText)
	return req, true
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// notifier — дросселирует тосты и зовёт платформенный showToast.
type notifier struct {
	cfg  NotifyConfig
	mu   sync.Mutex
	last time.Time
	show func(appID, title, text string) error // подменяется в тесте
}

func newNotifier(cfg NotifyConfig) *notifier {
	return &notifier{cfg: cfg, show: showToast}
}

// notify — показать тост. Возвращает shown=false, если выключено или слишком
// часто; ошибку — если платформа не смогла.
func (n *notifier) notify(req notifyRequest, now time.Time) (bool, error) {
	if !n.cfg.enabled() {
		return false, nil
	}
	n.mu.Lock()
	if now.Sub(n.last) < notifyMinGap {
		n.mu.Unlock()
		return false, nil
	}
	n.last = now
	n.mu.Unlock()
	return true, n.show(n.cfg.appID(), req.Title, req.Text)
}

// toastScript — PowerShell для одного тоста (чистая, тест; исполняется только
// на Windows, notify_windows.go). Заголовок и текст идут внутрь XML —
// экранируем как XML (апостроф тоже — &#39;), а строку целиком берём в
// одинарные кавычки PowerShell, удваивая апострофы внутри.
func toastScript(appID, title, text string) string {
	xml := `<toast duration="short"><visual><binding template="ToastGeneric"><text>` +
		html.EscapeString(title) + `</text><text>` + html.EscapeString(text) +
		`</text></binding></visual><audio src="ms-winsoundevent:Notification.Default"/></toast>`
	ps := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	return strings.Join([]string{
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null`,
		`[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null`,
		`$xml = New-Object Windows.Data.Xml.Dom.XmlDocument`,
		`$xml.LoadXml(` + ps(xml) + `)`,
		`$toast = New-Object Windows.UI.Notifications.ToastNotification $xml`,
		`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(` + ps(appID) + `).Show($toast)`,
	}, "\r\n") + "\r\n"
}

// decodeBody — JSON тела запроса (до 4 КБ: тост — не роман).
func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(v)
}

// handleNotify — POST /notify от гостевого экрана.
func (a *agent) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method"})
		return
	}
	var req notifyRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "message": "нужен JSON {title, text, kind}"})
		return
	}
	req, ok := normalizeNotify(req)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "message": "нужен text"})
		return
	}
	shown, err := a.notes.notify(req, time.Now())
	if err != nil {
		log.Printf("🔔 notify (%s) не показался: %v — %s", req.Kind, err, req.Text)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shown": false, "error": err.Error()})
		return
	}
	if shown {
		log.Printf("🔔 notify (%s): %s — %s", req.Kind, req.Title, req.Text)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shown": shown})
}
