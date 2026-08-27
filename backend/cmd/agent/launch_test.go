package main

// Тесты жизненного цикла игры. Вся логика launch.go — чистые функции с
// инъекцией системных вызовов, поэтому файл целиком проходит на Linux/CI.

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestAgent — агент без WS, каталога и фоновых циклов.
func newTestAgent(cfg *Config) *agent {
	if cfg == nil {
		cfg = defaultConfig()
	}
	return &agent{
		cfg:    cfg,
		remote: map[string]AppEntry{},
		launch: cfg.Launch.settings(),
		avail:  newAvailability(nil),
		track:  newTracker(nil),
	}
}

func TestClassifyTarget(t *testing.T) {
	cases := map[string]targetKind{
		`C:\Games\CS2\cs2.exe`:             kindFile,
		`c:/games/cs2.exe`:                 kindFile,
		`G:\Games\Team Fortress 2\hl2.exe`: kindFile,
		`\\nas\games\CS2\cs2.exe`:          kindFile,
		`//nas/games/cs2.exe`:              kindFile,
		`/usr/bin/sleep`:                   kindFile,
		`cs2.exe`:                          kindFile,
		`D:`:                               kindFile,
		`steam://rungameid/730`:            kindProtocol,
		`ms-settings:display`:              kindProtocol,
		`com.epicgames.launcher://apps/fn`: kindProtocol,
		`https://14-48.example/shell`:      kindProtocol,
		``:                                 kindEmpty,
		`   `:                              kindEmpty,
	}
	for target, want := range cases {
		if got := classifyTarget(target); got != want {
			t.Errorf("classifyTarget(%q) = %q, ожидалось %q", target, got, want)
		}
	}
}

func TestTargetRoot(t *testing.T) {
	cases := map[string]string{
		`\\nas\games\CS2\cs2.exe`: `\\nas\games`,
		`\\nas\games`:             `\\nas\games`,
		`\\nas`:                   `\\nas`,
		`//nas/games/cs2.exe`:     `//nas/games`,
		`G:\Games\cs2.exe`:        `G:\`,
		`c:/games/cs2.exe`:        `c:\`,
		`/mnt/games/cs2`:          `/`,
		`cs2.exe`:                 ``,
		``:                        ``,
	}
	for target, want := range cases {
		if got := targetRoot(target); got != want {
			t.Errorf("targetRoot(%q) = %q, ожидалось %q", target, got, want)
		}
	}
}

func TestProbeTarget(t *testing.T) {
	// «Диск G: подключён, шара \\nas\games — нет».
	present := map[string]bool{
		`G:\`:              true,
		`G:\Games\cs2.exe`: true,
	}
	exists := func(p string) error {
		if present[p] {
			return nil
		}
		return errors.New("нет такого пути")
	}

	if st := probeTarget(`G:\Games\cs2.exe`, exists); !st.Available || st.Reason != "" || st.Kind != kindFile {
		t.Errorf("установленная игра должна быть доступна: %+v", st)
	}
	// диск на месте, файла нет → target_missing
	if st := probeTarget(`G:\Games\gta5\gta5.exe`, exists); st.Available || st.Reason != reasonMissing {
		t.Errorf("отсутствующий файл на живом диске: %+v", st)
	}
	// шара не отвечает → path_unreachable (это и увидит гость на витрине)
	if st := probeTarget(`\\nas\games\gta5\gta5.exe`, exists); st.Available || st.Reason != reasonUnreachable {
		t.Errorf("мёртвая шара: %+v", st)
	}
	// протокол проверить нечем — доступен всегда
	if st := probeTarget(`steam://rungameid/730`, exists); !st.Available || st.Kind != kindProtocol {
		t.Errorf("протокол: %+v", st)
	}
	// пустой target в каталоге — честный отказ, а не «доступно»
	if st := probeTarget(``, exists); st.Available || st.Reason != reasonNoTarget {
		t.Errorf("пустой target: %+v", st)
	}
}

func TestMemoStat(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	base := func(p string) error {
		mu.Lock()
		calls[p]++
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		if p == "bad" {
			return errors.New("нет")
		}
		return nil
	}
	memo := memoStat(base)

	// 20 игр на одной шаре → корень щупаем один раз
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = memo(`\\nas\games`) }()
	}
	wg.Wait()
	if calls[`\\nas\games`] != 1 {
		t.Errorf("корень проверен %d раз, ожидался 1", calls[`\\nas\games`])
	}
	if memo("bad") == nil || memo("bad") == nil {
		t.Error("ошибка должна запоминаться и повторяться")
	}
	if calls["bad"] != 1 {
		t.Errorf("bad проверен %d раз, ожидался 1", calls["bad"])
	}
}

func TestLaunchSettings(t *testing.T) {
	// пустой блок → рабочие дефолты
	d := LaunchConfig{}.settings()
	if d.CheckInterval != 60*time.Second || d.CheckTimeout != 5*time.Second ||
		d.AttachTimeout != 60*time.Second || d.StopTimeout != 10*time.Second ||
		d.WatchInterval != 3*time.Second {
		t.Errorf("дефолты не те: %+v", d)
	}
	if !d.StopOnEnd || !d.Telemetry {
		t.Errorf("завершение по сессии и телеметрия включены по умолчанию: %+v", d)
	}

	no := false
	off := LaunchConfig{StopOnSessionEnd: &no, Telemetry: &no}.settings()
	if off.StopOnEnd || off.Telemetry {
		t.Errorf("явное false должно выключать: %+v", off)
	}

	// границы: опечатка в конфиге не должна дать проверку раз в секунду
	c := LaunchConfig{CheckIntervalSec: 1, CheckTimeoutSec: 9999, StopTimeoutSec: 500,
		WatchIntervalSec: 0, AttachTimeoutSec: -1}.settings()
	if c.CheckInterval != 5*time.Second || c.CheckTimeout != 60*time.Second ||
		c.StopTimeout != 120*time.Second || c.WatchInterval != 3*time.Second {
		t.Errorf("границы не применились: %+v", c)
	}
	if c.AttachTimeout != 0 {
		t.Errorf("attach_timeout_sec<0 = не ждать процесс, получено %v", c.AttachTimeout)
	}
}

func TestAvailabilityCache(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	av := newAvailability(func() time.Time { return now })

	known := map[string]AppEntry{
		"cs2":  {Target: "steam://rungameid/730"},
		"gta5": {Target: `\\nas\games\gta5.exe`},
		"new":  {Target: `G:\new.exe`},
	}
	av.set("cs2", appStatus{Available: true, Kind: kindProtocol})
	av.set("gta5", appStatus{Available: false, Reason: reasonUnreachable, Kind: kindFile})

	snap := av.snapshot(known)
	if len(snap) != 3 {
		t.Fatalf("в снимке %d записей, ожидалось 3", len(snap))
	}
	if !snap["cs2"].Available || snap["cs2"].CheckedAt != "2026-08-27T15:00:00Z" {
		t.Errorf("cs2: %+v", snap["cs2"])
	}
	if snap["gta5"].Available || snap["gta5"].Reason != reasonUnreachable {
		t.Errorf("gta5: %+v", snap["gta5"])
	}
	// ещё не проверенное отдаём оптимистично — плитка не гаснет на ровном месте
	if !snap["new"].Available || snap["new"].Reason != reasonChecking {
		t.Errorf("непроверенное: %+v", snap["new"])
	}

	if _, ok := av.fresh("cs2", time.Minute); !ok {
		t.Error("свежая запись должна находиться")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := av.fresh("cs2", time.Minute); ok {
		t.Error("протухшая запись не должна считаться свежей")
	}

	// параллельная проверка одного id не запускается дважды
	if !av.beginProbe("cs2") || av.beginProbe("cs2") {
		t.Error("beginProbe должен пускать только одного")
	}
	av.endProbe("cs2")
	if !av.beginProbe("cs2") {
		t.Error("после endProbe проверка снова возможна")
	}
	av.endProbe("cs2")

	av.prune(map[string]AppEntry{"cs2": {Target: "x"}})
	if len(av.snapshot(known)) != 3 || av.snapshot(known)["gta5"].Reason != reasonChecking {
		t.Error("prune должен был выкинуть исчезнувший из allowlist gta5")
	}
}

func TestTracker(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	tr := newTracker(func() time.Time { return now })

	tr.put(procRecord{appID: "cs2", pid: 111, state: stateRunning})
	tr.put(procRecord{appID: "valorant", state: stateStarting, process: "VALORANT-Win64-Shipping.exe"})

	// «последняя живая» — свежайшая по порядку запуска
	views := runningViews(tr.snapshot())
	if len(views) != 2 || views[0].AppID != "valorant" {
		t.Fatalf("порядок снимка неверен: %+v", views)
	}
	if views[1].AppID != "cs2" || views[1].PID != 111 || views[1].Since != "2026-08-27T15:00:00Z" ||
		views[1].State != stateRunning {
		t.Errorf("вид записи неверен: %+v", views[1])
	}

	if !tr.attach("valorant", 222) {
		t.Fatal("attach должен был сработать")
	}
	if rec, _ := tr.get("valorant"); rec.pid != 222 || rec.state != stateRunning {
		t.Errorf("после attach: %+v", rec)
	}

	// dropIfPID отдаёт запись ровно одному — app_exit не задваивается
	if _, ok := tr.dropIfPID("cs2", 999); ok {
		t.Error("чужой pid не должен снимать запись")
	}
	if _, ok := tr.dropIfPID("cs2", 111); !ok {
		t.Error("свой pid должен снимать запись")
	}
	if _, ok := tr.dropIfPID("cs2", 111); ok {
		t.Error("повторное снятие должно вернуть false")
	}

	tr.put(procRecord{appID: "epic", state: stateStarting})
	tr.markLaunched("epic")
	if rec, _ := tr.get("epic"); rec.state != stateLaunched || rec.pid != 0 {
		t.Errorf("процесс не нашёлся → launched без pid: %+v", rec)
	}
}

func TestStopSequence(t *testing.T) {
	sleeps := 0
	var gentleCalls, forceCalls []int
	gentle := func(pid int) error { gentleCalls = append(gentleCalls, pid); return nil }
	force := func(pid int) error { forceCalls = append(forceCalls, pid); return nil }
	alive := func(pid int) bool {
		switch pid {
		case 1:
			return sleeps < 1 // закрылось само после первого опроса
		case 2:
			return len(forceCalls) == 0 // сдалось только принудительно
		case 3:
			return false // уже было закрыто
		case 4:
			return true // не закрывается ничем
		}
		return false
	}
	got := stopSequence(
		[]stopTarget{{"cs2", 1}, {"gta5", 2}, {"dota", 3}, {"zombie", 4}, {"steam", 0}},
		2*time.Second, 500*time.Millisecond,
		gentle, force, alive, func(time.Duration) { sleeps++ },
	)

	want := map[string]string{"cs2": "gentle", "gta5": "forced", "dota": "gone",
		"zombie": "failed", "steam": "no_pid"}
	if len(got) != len(want) {
		t.Fatalf("итогов %d, ожидалось %d: %+v", len(got), len(want), got)
	}
	for _, o := range got {
		if want[o.AppID] != o.Result {
			t.Errorf("%s: %q, ожидалось %q", o.AppID, o.Result, want[o.AppID])
		}
	}
	// мёртвых и беспидышных не трогаем вообще
	for _, pid := range gentleCalls {
		if pid == 3 || pid == 0 {
			t.Errorf("gentle не должен вызываться для pid %d", pid)
		}
	}
	if len(forceCalls) != 2 {
		t.Errorf("принудительно закрывали %d процессов, ожидалось 2 (gta5, zombie)", len(forceCalls))
	}
	// у каждого сообщения об итоге есть человеческий перевод для лога
	for _, r := range want {
		if stopResultRu[r] == "" {
			t.Errorf("нет русского текста для итога %q", r)
		}
	}
}

func TestParsePgrepPID(t *testing.T) {
	if pid, ok := parsePgrepPID("4231\n"); !ok || pid != 4231 {
		t.Errorf("pgrep: %d %v", pid, ok)
	}
	if pid, ok := parsePgrepPID("\n\n  871 \n900\n"); !ok || pid != 871 {
		t.Errorf("первая непустая строка: %d %v", pid, ok)
	}
	if _, ok := parsePgrepPID(""); ok {
		t.Error("пустой вывод — не PID")
	}
	if _, ok := parsePgrepPID("нет такого процесса"); ok {
		t.Error("мусор — не PID")
	}
}

func TestTargetMissingMessage(t *testing.T) {
	if !strings.Contains(targetMissingMessage(reasonUnreachable), "иск") {
		t.Errorf("про недоступный диск: %q", targetMissingMessage(reasonUnreachable))
	}
	if targetMissingMessage(reasonMissing) == targetMissingMessage(reasonUnreachable) {
		t.Error("«не установлена» и «диск недоступен» — разные сообщения гостю")
	}
	if targetMissingMessage(reasonNoTarget) == "" || targetMissingMessage("") == "" {
		t.Error("сообщение не должно быть пустым ни для одной причины")
	}
}

func TestAllAppsLocalWins(t *testing.T) {
	a := newTestAgent(&Config{Apps: map[string]AppEntry{
		"cs2":   {Target: `C:\local\cs2.exe`},
		"empty": {Target: ""},
	}})
	a.remote = map[string]AppEntry{
		"cs2":  {Target: "steam://rungameid/730"},
		"dota": {Target: "steam://rungameid/570"},
	}
	all := a.allApps()
	if all["cs2"].Target != `C:\local\cs2.exe` {
		t.Errorf("локальный agent.json главнее каталога: %+v", all["cs2"])
	}
	if all["dota"].Target == "" {
		t.Error("запись из каталога должна попасть в общий список")
	}
	if _, ok := all["empty"]; ok {
		t.Error("локальная запись с пустым target не должна перекрывать/появляться")
	}
}

// Витрина (web/shell.html, NA_REASON) выбирает сообщение гостю по полю code
// ответа /launch. Если «шара не отвечает» приедет общим target_missing, гость
// прочитает «игра не установлена» — и пойдёт искать её в другом месте вместо
// того, чтобы позвать администратора.
func TestLaunchErrorCode(t *testing.T) {
	if launchErrorCode(reasonUnreachable) != "path_unreachable" {
		t.Errorf("недоступная шара: %q", launchErrorCode(reasonUnreachable))
	}
	for _, r := range []string{reasonMissing, reasonNoTarget, ""} {
		if launchErrorCode(r) != "target_missing" {
			t.Errorf("причина %q → код %q, ожидался target_missing", r, launchErrorCode(r))
		}
	}
}
