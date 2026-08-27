package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveApp(t *testing.T) {
	cfg := &Config{Apps: map[string]AppEntry{
		"cs2":   {Target: "steam://rungameid/730"},
		"empty": {Target: ""},
	}}

	if _, err := resolveApp(cfg, "cs2"); err != nil {
		t.Errorf("cs2 из allowlist отклонён: %v", err)
	}
	// не из allowlist — отклоняется, ничего не выполняется
	if _, err := resolveApp(cfg, "cmd.exe"); !errors.Is(err, errUnknownApp) {
		t.Errorf("посторонний id прошёл allowlist: %v", err)
	}
	// пустой target — тоже отказ
	if _, err := resolveApp(cfg, "empty"); !errors.Is(err, errUnknownApp) {
		t.Errorf("пустой target прошёл allowlist: %v", err)
	}
}

func TestRemoteAllowlist(t *testing.T) {
	target := "steam://rungameid/730"
	empty := ""
	args := `["--flag=1","--x"]`
	badArgs := `не json`

	items := []catalogItem{
		{ID: "cs2", Target: &target, Args: &args},
		{ID: "no_target", Target: nil},
		{ID: "empty_target", Target: &empty},
		{ID: "", Target: &target}, // без id — пропускаем
		{ID: "bad_args", Target: &target, Args: &badArgs},
	}
	out := remoteAllowlist(items)

	if len(out) != 2 {
		t.Fatalf("в allowlist %d записей, ожидалось 2 (cs2, bad_args): %+v", len(out), out)
	}
	if out["cs2"].Target != target || len(out["cs2"].Args) != 2 {
		t.Errorf("cs2 собран неверно: %+v", out["cs2"])
	}
	// битые args не валят запись — просто без аргументов
	if len(out["bad_args"].Args) != 0 {
		t.Errorf("битые args должны игнорироваться: %+v", out["bad_args"])
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	// нет файла → дефолты + ошибка (не падаем)
	cfg, err := loadConfig(filepath.Join(dir, "нет.json"))
	if err == nil {
		t.Error("ожидалась ошибка отсутствующего файла")
	}
	if cfg.Listen != "127.0.0.1:1448" || cfg.BackendWS == "" {
		t.Errorf("дефолты не применились: %+v", cfg)
	}

	// валидный файл
	p := filepath.Join(dir, "agent.json")
	os.WriteFile(p, []byte(`{"computer_id":"abc","apps":{"steam":{"target":"steam://open/main"}}}`), 0o644)
	cfg, err = loadConfig(p)
	if err != nil {
		t.Fatalf("валидный конфиг не прочитался: %v", err)
	}
	if cfg.ComputerID != "abc" || len(cfg.Apps) != 1 || cfg.Listen != "127.0.0.1:1448" {
		t.Errorf("конфиг прочитан неверно: %+v", cfg)
	}

	// битый JSON → дефолты + ошибка
	os.WriteFile(p, []byte(`{битый`), 0o644)
	cfg, err = loadConfig(p)
	if err == nil {
		t.Error("ожидалась ошибка битого JSON")
	}
	if len(cfg.Apps) != 0 {
		t.Error("после битого JSON allowlist должен быть пуст")
	}
}

func TestWolMagicPacket(t *testing.T) {
	p, err := wolMagicPacket("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("валидный MAC не распарсился: %v", err)
	}
	if len(p) != 102 {
		t.Errorf("magic-пакет должен быть 102 байта, получил %d", len(p))
	}
	for i := 0; i < 6; i++ {
		if p[i] != 0xFF {
			t.Errorf("байт %d префикса должен быть 0xFF", i)
		}
	}
	if p[6] != 0xAA || p[11] != 0xFF || p[96] != 0xAA {
		t.Error("MAC должен повторяться 16 раз после префикса")
	}
	if _, err := wolMagicPacket("не-мак"); err == nil {
		t.Error("мусор вместо MAC должен давать ошибку")
	}
	if _, err := wolMagicPacket(""); err == nil {
		t.Error("пустой MAC должен давать ошибку")
	}
}

// ─────────── контракт HTTP: /apps-status и /launch (пакет «Игры»-2) ───────────

// TestAppsStatusContract — форму ответа читает гостевая витрина (web/shell.html),
// менять её без согласования нельзя. Тест фиксирует контракт побайтово по полям.
func TestAppsStatusContract(t *testing.T) {
	a := newTestAgent(&Config{Apps: map[string]AppEntry{
		"cs2":   {Target: "steam://rungameid/730"},
		"gta5":  {Target: `\\nas\games\gta5\gta5.exe`},
		"fresh": {Target: `G:\Games\new.exe`}, // ещё не проверялось
	}})
	a.avail.set("cs2", appStatus{Available: true, Kind: kindProtocol})
	a.avail.set("gta5", appStatus{Available: false, Reason: "path_unreachable", Kind: kindFile})
	a.track.put(procRecord{appID: "cs2", pid: 1234, state: stateRunning,
		since: time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)})

	rec := httptest.NewRecorder()
	a.handleAppsStatus(rec, httptest.NewRequest(http.MethodGet, "/apps-status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d", rec.Code)
	}

	var got struct {
		Apps map[string]struct {
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"apps"`
		Running *struct {
			AppID string `json:"app_id"`
			PID   int    `json:"pid"`
			Since string `json:"since"`
			State string `json:"state"`
		} `json:"running"`
		RunningAll []struct {
			AppID string `json:"app_id"`
		} `json:"running_all"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("ответ не разобрался: %v\n%s", err, rec.Body.String())
	}
	if len(got.Apps) != 3 {
		t.Fatalf("в apps %d записей, ожидалось 3: %s", len(got.Apps), rec.Body.String())
	}
	if !got.Apps["cs2"].Available || got.Apps["cs2"].Reason != "" {
		t.Errorf("cs2 должен быть доступен без причины: %+v", got.Apps["cs2"])
	}
	if got.Apps["gta5"].Available || got.Apps["gta5"].Reason != "path_unreachable" {
		t.Errorf("gta5: %+v", got.Apps["gta5"])
	}
	if !got.Apps["fresh"].Available || got.Apps["fresh"].Reason != "checking" {
		t.Errorf("непроверенное отдаётся оптимистично: %+v", got.Apps["fresh"])
	}
	if got.Running == nil || got.Running.AppID != "cs2" || got.Running.PID != 1234 ||
		got.Running.Since != "2026-08-27T15:00:00Z" || got.Running.State != "running" {
		t.Errorf("running: %+v", got.Running)
	}
	if len(got.RunningAll) != 1 {
		t.Errorf("running_all: %+v", got.RunningAll)
	}

	// ничего не запущено → running: null, running_all: [] (а не отсутствие полей)
	a.track.drop("cs2")
	rec = httptest.NewRecorder()
	a.handleAppsStatus(rec, httptest.NewRequest(http.MethodGet, "/apps-status", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"running":null`) || !strings.Contains(body, `"running_all":[]`) {
		t.Errorf("пустое состояние должно быть явным: %s", body)
	}
}

func TestLaunchRejects(t *testing.T) {
	a := newTestAgent(&Config{Apps: map[string]AppEntry{
		"ghost": {Target: "/нет-такой-папки-1448/game.exe"},
	}})

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.handleLaunch(rec, httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(body)))
		return rec
	}
	code := func(rec *httptest.ResponseRecorder) string {
		var r struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		if r.Reason != "" {
			return r.Code + "/" + r.Reason
		}
		return r.Code
	}

	// GET вместо POST
	rec := httptest.NewRecorder()
	a.handleLaunch(rec, httptest.NewRequest(http.MethodGet, "/launch", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /launch → %d", rec.Code)
	}
	if rec = post(`{}`); rec.Code != http.StatusBadRequest || code(rec) != "invalid_request" {
		t.Errorf("без app_id → %d %s", rec.Code, code(rec))
	}
	if rec = post(`{"app_id":"cmd.exe"}`); rec.Code != http.StatusNotFound || code(rec) != "unknown_app" {
		t.Errorf("не из allowlist → %d %s", rec.Code, code(rec))
	}
	// главное ради чего всё затевалось: несуществующий путь = честный отказ,
	// а не «ok» и молчаливое ничего
	rec = post(`{"app_id":"ghost"}`)
	if rec.Code != http.StatusConflict || code(rec) != "target_missing/target_missing" {
		t.Errorf("несуществующий exe → %d %s (%s)", rec.Code, code(rec), rec.Body.String())
	}
	if st, ok := a.avail.fresh("ghost", time.Minute); !ok || st.Available {
		t.Error("отказ должен был лечь в кэш доступности")
	}
}

// TestLaunchTrackStop — полный круг на живом процессе: запуск → PID в
// /apps-status → повторный клик не плодит второй → session_end закрывает.
func TestLaunchTrackStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("сценарий гоняется на Linux/CI; на Windows проверяется вживую по доку")
	}
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("нет sleep")
	}
	a := newTestAgent(&Config{Apps: map[string]AppEntry{
		"cs2": {Target: sleepBin, Args: []string{"60"}},
	}})

	rec := httptest.NewRecorder()
	a.handleLaunch(rec, httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(`{"app_id":"cs2"}`)))
	var first struct {
		OK    bool   `json:"ok"`
		PID   int    `json:"pid"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if rec.Code != http.StatusOK || !first.OK || first.PID <= 0 || first.State != "running" {
		t.Fatalf("запуск: %d %+v", rec.Code, first)
	}
	if !processAlive(first.PID) {
		t.Fatalf("процесс %d должен быть жив", first.PID)
	}

	// второй клик по запущенной игре
	rec = httptest.NewRecorder()
	a.handleLaunch(rec, httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(`{"app_id":"cs2"}`)))
	var second struct {
		PID   int    `json:"pid"`
		State string `json:"state"`
		Code  string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if second.State != "already_running" || second.Code != "already_running" || second.PID != first.PID {
		t.Errorf("повторный клик не должен плодить процессы: %+v", second)
	}

	// конец сессии закрывает именно то, что запустил агент
	a.cfg.Cleanup = CleanupConfig{} // общая очистка выключена — проверяем только нашу часть
	a.onSessionEnd()
	if processAlive(first.PID) {
		t.Errorf("процесс %d пережил session_end", first.PID)
	}
	if len(a.track.snapshot()) != 0 {
		t.Errorf("реестр после session_end должен быть пуст: %+v", a.track.snapshot())
	}
}
