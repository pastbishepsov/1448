package main

// Жизненный цикл игры на гостевом ПК (трек «Игры», пакет 2).
//
// Что здесь:
//   · классификация target (файл / протокол) и проверка достижимости;
//   · кэш доступности: фоновая проверка раз в launch.check_interval_sec,
//     GET /apps-status отдаёт готовый снимок и НИКОГДА не ходит в сеть сам;
//   · реестр запущенного (app_id → pid, время старта, состояние);
//   · завершение запущенного по session_end: мягко → пауза → принудительно;
//   · телеметрия app_launch / app_exit в существующий WS-канал.
//
// Системные вызовы (запуск процесса, поиск по имени, проверка живости,
// taskkill/SIGTERM) вынесены в proc_windows.go / proc_other.go — здесь
// только логика, поэтому файл целиком покрывается тестами на Linux.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────── настройки ───────────────────────────

// LaunchConfig — блок "launch" в agent.json. Нули = дефолты (см. settings).
type LaunchConfig struct {
	CheckIntervalSec int   `json:"check_interval_sec"`  // период фоновой проверки путей, 60
	CheckTimeoutSec  int   `json:"check_timeout_sec"`   // таймаут одной проверки пути, 5
	AttachTimeoutSec int   `json:"attach_timeout_sec"`  // сколько ждать процесс после протокольного запуска, 60 (0 = не ждать)
	StopTimeoutSec   int   `json:"stop_timeout_sec"`    // мягкое завершение до принудительного, 10
	WatchIntervalSec int   `json:"watch_interval_sec"`  // период опроса живости процессов, 3
	StopOnSessionEnd *bool `json:"stop_on_session_end"` // null = true: гасить запущенное по концу сессии
	Telemetry        *bool `json:"telemetry"`           // null = true: слать app_launch/app_exit в WS
}

type launchSettings struct {
	CheckInterval time.Duration
	CheckTimeout  time.Duration
	AttachTimeout time.Duration
	StopTimeout   time.Duration
	WatchInterval time.Duration
	StopOnEnd     bool
	Telemetry     bool
}

// clampSec — секунды из конфига в Duration: 0 = дефолт, остальное в границах.
func clampSec(v, def, min, max int) time.Duration {
	if v == 0 {
		v = def
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return time.Duration(v) * time.Second
}

// settings — конфиг → рабочие значения (чистая функция, тест).
func (c LaunchConfig) settings() launchSettings {
	s := launchSettings{
		CheckInterval: clampSec(c.CheckIntervalSec, 60, 5, 3600),
		CheckTimeout:  clampSec(c.CheckTimeoutSec, 5, 1, 60),
		StopTimeout:   clampSec(c.StopTimeoutSec, 10, 1, 120),
		WatchInterval: clampSec(c.WatchIntervalSec, 3, 1, 60),
		StopOnEnd:     c.StopOnSessionEnd == nil || *c.StopOnSessionEnd,
		Telemetry:     c.Telemetry == nil || *c.Telemetry,
	}
	// attach_timeout_sec: 0 = дефолт 60с, отрицательное = не ждать процесс вовсе.
	if c.AttachTimeoutSec < 0 {
		s.AttachTimeout = 0
	} else {
		s.AttachTimeout = clampSec(c.AttachTimeoutSec, 60, 1, 600)
	}
	return s
}

// ─────────────────────────── классификация target ───────────────────────────

type targetKind string

const (
	kindFile     targetKind = "file"
	kindProtocol targetKind = "protocol"
	kindEmpty    targetKind = "empty"
)

func isDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// schemeEnd — позиция ':' если строка начинается со схемы вида steam:,
// ms-settings:, com.epicgames.launcher:. Одна буква до ':' — это буква
// диска (C:\...), а не схема.
func schemeEnd(s string) int {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == ':':
			if i < 2 {
				return -1 // "C:" — диск
			}
			return i
		case isDriveLetter(ch), ch >= '0' && ch <= '9', ch == '+', ch == '-', ch == '.':
			if i == 0 && !isDriveLetter(ch) {
				return -1 // схема начинается с буквы
			}
		default:
			return -1
		}
	}
	return -1
}

// classifyTarget — файл это или протокол (чистая функция, тест).
func classifyTarget(target string) targetKind {
	t := strings.TrimSpace(target)
	if t == "" {
		return kindEmpty
	}
	if strings.HasPrefix(t, `\\`) || strings.HasPrefix(t, "//") {
		return kindFile // UNC-путь \\nas\games\...
	}
	if strings.HasPrefix(t, "/") || strings.HasPrefix(t, `\`) {
		return kindFile
	}
	if len(t) >= 2 && t[1] == ':' && isDriveLetter(t[0]) {
		return kindFile // C:\..., G:/...
	}
	if schemeEnd(t) > 0 {
		return kindProtocol
	}
	return kindFile // относительный путь — честно провалится на проверке
}

// targetRoot — корень тома или шары для файлового target (чистая функция,
// тест). Нужен, чтобы отличить «сетевой диск отвалился» (path_unreachable)
// от «диск на месте, а файла нет» (target_missing).
//
//	\\nas\games\CS2\cs2.exe → \\nas\games
//	G:\Games\cs2.exe        → G:\
//	/mnt/games/cs2          → /
func targetRoot(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, `\\`) || strings.HasPrefix(t, "//") {
		sep := t[:2]
		rest := strings.FieldsFunc(t[2:], func(r rune) bool { return r == '\\' || r == '/' })
		if len(rest) >= 2 {
			return sep + rest[0] + sep[:1] + rest[1] // \\nas\games
		}
		if len(rest) == 1 {
			return sep + rest[0]
		}
		return sep
	}
	if len(t) >= 2 && t[1] == ':' && isDriveLetter(t[0]) {
		return t[:2] + `\`
	}
	if strings.HasPrefix(t, "/") {
		return "/"
	}
	return ""
}

// ─────────────────────────── доступность ───────────────────────────

const (
	reasonChecking    = "checking"         // ещё не проверяли (available отдаём оптимистично)
	reasonMissing     = "target_missing"   // путь достижим, файла нет
	reasonUnreachable = "path_unreachable" // корень тома/шары не отвечает
	reasonNoTarget    = "no_target"        // в каталоге пустой target
)

// appStatus — то, что уезжает в GET /apps-status по каждому приложению.
type appStatus struct {
	Available bool       `json:"available"`
	Reason    string     `json:"reason,omitempty"`
	Kind      targetKind `json:"kind,omitempty"`
	CheckedAt string     `json:"checked_at,omitempty"`
}

// probeTarget — решение о доступности (чистая функция, тест): системный
// stat инжектируется, поэтому проверяется без файловой системы.
func probeTarget(target string, exists func(string) error) appStatus {
	kind := classifyTarget(target)
	switch kind {
	case kindEmpty:
		return appStatus{Available: false, Reason: reasonNoTarget, Kind: kind}
	case kindProtocol:
		// steam://, ms-settings: — проверить нечего, обработчик решает сам.
		return appStatus{Available: true, Kind: kind}
	}
	if err := exists(strings.TrimSpace(target)); err == nil {
		return appStatus{Available: true, Kind: kind}
	}
	if root := targetRoot(target); root != "" {
		if err := exists(root); err != nil {
			return appStatus{Available: false, Reason: reasonUnreachable, Kind: kind}
		}
	}
	return appStatus{Available: false, Reason: reasonMissing, Kind: kind}
}

// statWithTimeout — os.Stat с таймаутом: зависший сетевой путь не должен
// подвешивать проверку. Горутина досидит до ответа системы и умрёт сама.
func statWithTimeout(timeout time.Duration) func(string) error {
	return func(path string) error {
		done := make(chan error, 1)
		go func() {
			_, err := os.Stat(path)
			done <- err
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			return fmt.Errorf("путь не ответил за %s", timeout)
		}
	}
}

// memoStat — один и тот же путь за цикл проверки щупаем один раз (у 50 игр
// на одной шаре корень общий). Параллельные запросы к одному пути ждут
// первого. Чистая обёртка — тестируется счётчиком вызовов.
func memoStat(fn func(string) error) func(string) error {
	type entry struct {
		done chan struct{}
		err  error
	}
	var mu sync.Mutex
	cache := map[string]*entry{}
	return func(path string) error {
		mu.Lock()
		e, ok := cache[path]
		if ok {
			mu.Unlock()
			<-e.done
			return e.err
		}
		e = &entry{done: make(chan struct{})}
		cache[path] = e
		mu.Unlock()
		e.err = fn(path)
		close(e.done)
		return e.err
	}
}

type availEntry struct {
	status appStatus
	at     time.Time
}

// availability — кэш доступности приложений. /apps-status читает только его.
type availability struct {
	mu       sync.Mutex
	entries  map[string]availEntry
	inflight map[string]bool
	now      func() time.Time
}

func newAvailability(now func() time.Time) *availability {
	if now == nil {
		now = time.Now
	}
	return &availability{
		entries:  map[string]availEntry{},
		inflight: map[string]bool{},
		now:      now,
	}
}

func (av *availability) set(id string, st appStatus) {
	av.mu.Lock()
	defer av.mu.Unlock()
	av.entries[id] = availEntry{status: st, at: av.now()}
}

// fresh — свежий вердикт по id, если он моложе ttl.
func (av *availability) fresh(id string, ttl time.Duration) (appStatus, bool) {
	av.mu.Lock()
	defer av.mu.Unlock()
	e, ok := av.entries[id]
	if !ok || av.now().Sub(e.at) > ttl {
		return appStatus{}, false
	}
	return e.status, true
}

func (av *availability) beginProbe(id string) bool {
	av.mu.Lock()
	defer av.mu.Unlock()
	if av.inflight[id] {
		return false
	}
	av.inflight[id] = true
	return true
}

func (av *availability) endProbe(id string) {
	av.mu.Lock()
	defer av.mu.Unlock()
	delete(av.inflight, id)
}

// prune — выкинуть записи о приложениях, которых больше нет в allowlist.
func (av *availability) prune(known map[string]AppEntry) {
	av.mu.Lock()
	defer av.mu.Unlock()
	for id := range av.entries {
		if _, ok := known[id]; !ok {
			delete(av.entries, id)
		}
	}
}

// snapshot — карта для /apps-status. Непроверенные пока приложения отдаются
// оптимистично (available=true, reason=checking): лучше честная ошибка на
// клике, чем ложно погашенная плитка в первые секунды после старта агента.
func (av *availability) snapshot(known map[string]AppEntry) map[string]appStatus {
	av.mu.Lock()
	defer av.mu.Unlock()
	out := make(map[string]appStatus, len(known))
	for id, app := range known {
		if e, ok := av.entries[id]; ok {
			st := e.status
			st.CheckedAt = e.at.UTC().Format(time.RFC3339)
			out[id] = st
			continue
		}
		out[id] = appStatus{Available: true, Reason: reasonChecking, Kind: classifyTarget(app.Target)}
	}
	return out
}

// ─────────────────────────── реестр запущенного ───────────────────────────

const (
	stateRunning  = "running"  // есть живой PID
	stateStarting = "starting" // протокол отдан обработчику, ждём процесс по имени
	stateLaunched = "launched" // запущено, PID узнать неоткуда
)

type procRecord struct {
	appID   string
	pid     int
	since   time.Time
	state   string
	process string // имя процесса игры (для attach и завершения)
	kind    targetKind
	seq     uint64
}

type tracker struct {
	mu    sync.Mutex
	items map[string]*procRecord
	seq   uint64
	now   func() time.Time
}

func newTracker(now func() time.Time) *tracker {
	if now == nil {
		now = time.Now
	}
	return &tracker{items: map[string]*procRecord{}, now: now}
}

func (t *tracker) put(rec procRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	rec.seq = t.seq
	if rec.since.IsZero() {
		rec.since = t.now()
	}
	cp := rec
	t.items[rec.appID] = &cp
}

func (t *tracker) get(appID string) (procRecord, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.items[appID]
	if !ok {
		return procRecord{}, false
	}
	return *rec, true
}

// attach — подставить найденный PID в запись (протокольный запуск).
func (t *tracker) attach(appID string, pid int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.items[appID]
	if !ok {
		return false
	}
	rec.pid = pid
	rec.state = stateRunning
	return true
}

// markLaunched — процесс так и не нашёлся: PID не будет, но запись остаётся.
func (t *tracker) markLaunched(appID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rec, ok := t.items[appID]; ok && rec.state == stateStarting {
		rec.state = stateLaunched
		rec.pid = 0
	}
}

// dropIfPID — снять запись, только если она всё ещё про этот PID. Возвращает
// true ровно одному вызывающему — так app_exit не задваивается между
// ожиданием процесса и фоновым опросом.
func (t *tracker) dropIfPID(appID string, pid int) (procRecord, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.items[appID]
	if !ok || rec.pid != pid {
		return procRecord{}, false
	}
	delete(t.items, appID)
	return *rec, true
}

func (t *tracker) drop(appID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, appID)
}

func (t *tracker) snapshot() []procRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]procRecord, 0, len(t.items))
	for _, rec := range t.items {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq > out[j].seq })
	return out
}

// runningView — форма записи в JSON /apps-status.
type runningView struct {
	AppID string `json:"app_id"`
	PID   int    `json:"pid"`
	Since string `json:"since"`
	State string `json:"state"`
}

// runningViews — снимок реестра в JSON-вид, свежайшее первым (чистая, тест).
func runningViews(recs []procRecord) []runningView {
	out := make([]runningView, 0, len(recs))
	for _, r := range recs {
		out = append(out, runningView{
			AppID: r.appID,
			PID:   r.pid,
			Since: r.since.UTC().Format(time.RFC3339),
			State: r.state,
		})
	}
	return out
}

// ─────────────────────────── завершение ───────────────────────────

type stopTarget struct {
	AppID string
	PID   int
}

type stopOutcome struct {
	AppID  string
	PID    int
	Result string // gone | gentle | forced | failed | no_pid
}

// stopSequence — мягко → подождать → принудительно (чистая функция, тест).
// Все системные вызовы инжектируются, поэтому тестируется без Windows и без
// реального времени.
func stopSequence(targets []stopTarget, timeout, poll time.Duration,
	gentle, force func(int) error, alive func(int) bool, sleep func(time.Duration)) []stopOutcome {

	out := make([]stopOutcome, 0, len(targets))
	pending := make([]stopTarget, 0, len(targets))

	for _, t := range targets {
		if t.PID <= 0 {
			out = append(out, stopOutcome{t.AppID, t.PID, "no_pid"})
			continue
		}
		if !alive(t.PID) {
			out = append(out, stopOutcome{t.AppID, t.PID, "gone"})
			continue
		}
		_ = gentle(t.PID)
		pending = append(pending, t)
	}

	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	for waited := time.Duration(0); len(pending) > 0 && waited < timeout; waited += poll {
		sleep(poll)
		rest := make([]stopTarget, 0, len(pending))
		for _, t := range pending {
			if alive(t.PID) {
				rest = append(rest, t)
			} else {
				out = append(out, stopOutcome{t.AppID, t.PID, "gentle"})
			}
		}
		pending = rest
	}

	for _, t := range pending {
		_ = force(t.PID)
	}
	// после принудительного даём системе короткую паузу и фиксируем итог
	for waited := time.Duration(0); len(pending) > 0 && waited < 2*poll+time.Second; waited += poll {
		sleep(poll)
		rest := make([]stopTarget, 0, len(pending))
		for _, t := range pending {
			if alive(t.PID) {
				rest = append(rest, t)
			} else {
				out = append(out, stopOutcome{t.AppID, t.PID, "forced"})
			}
		}
		pending = rest
	}
	for _, t := range pending {
		out = append(out, stopOutcome{t.AppID, t.PID, "failed"})
	}
	return out
}

// parsePgrepPID — разбор вывода pgrep (нужен только не-Windows ветке,
// живёт здесь ради теста).
func parsePgrepPID(out string) (int, bool) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if pid, err := strconv.Atoi(line); err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

// ─────────────────────────── агент: фоновые циклы ───────────────────────────

// allApps — объединённый allowlist: локальный agent.json главнее каталога.
func (a *agent) allApps() map[string]AppEntry {
	out := make(map[string]AppEntry, len(a.cfg.Apps))
	a.mu.Lock()
	for id, app := range a.remote {
		out[id] = app
	}
	a.mu.Unlock()
	for id, app := range a.cfg.Apps {
		if app.Target != "" {
			out[id] = app
		}
	}
	return out
}

// refreshAvailability — один цикл проверки путей. Ходит в файловую систему
// параллельно и с таймаутом; /apps-status этого не ждёт никогда.
func (a *agent) refreshAvailability() {
	apps := a.allApps()
	exists := memoStat(statWithTimeout(a.launch.CheckTimeout))
	var wg sync.WaitGroup
	for id, app := range apps {
		if !a.avail.beginProbe(id) {
			continue // прошлая проверка этого пути ещё висит на сети
		}
		wg.Add(1)
		go func(id string, app AppEntry) {
			defer wg.Done()
			defer a.avail.endProbe(id)
			a.avail.set(id, probeTarget(app.Target, exists))
		}(id, app)
	}
	wg.Wait()
	a.avail.prune(apps)
}

func (a *agent) runAvailability() {
	a.refreshAvailability()
	t := time.NewTicker(a.launch.CheckInterval)
	defer t.Stop()
	for range t.C {
		a.refreshAvailability()
	}
}

// runProcWatch — заметить, что игра закрылась, даже если её процесс запускал
// не сам агент (подхваченный по имени).
func (a *agent) runProcWatch() {
	t := time.NewTicker(a.launch.WatchInterval)
	defer t.Stop()
	for range t.C {
		for _, rec := range a.track.snapshot() {
			if rec.pid > 0 && !processAlive(rec.pid) {
				a.processGone(rec.appID, rec.pid)
			}
		}
	}
}

// ─────────────────────────── агент: запуск и трекинг ───────────────────────────

// sendTelemetry — «во что играют сейчас» на бэкенд по существующему WS.
// Никогда не блокирует запуск: нет связи — событие просто теряется.
func (a *agent) sendTelemetry(kind string, payload map[string]any) {
	if !a.launch.Telemetry {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["computer_id"] = a.cfg.ComputerID
	payload["at"] = time.Now().UTC().Format(time.RFC3339)
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	go func() { _ = a.wsSend(wsMessage{Type: kind, Payload: body}) }()
}

// processGone — процесс записи умер. Если у приложения задан process, сперва
// пробуем подхватить эстафету (Riot Client завершается, VALORANT остаётся).
func (a *agent) processGone(appID string, pid int) {
	rec, ok := a.track.get(appID)
	if !ok || rec.pid != pid {
		return
	}
	if rec.process != "" {
		if npid, found := findProcessByName(rec.process); found && npid != pid {
			if a.track.attach(appID, npid) {
				log.Printf("▶ %s: лаунчер закрылся, отслеживаю %s (pid %d)", appID, rec.process, npid)
				return
			}
		}
	}
	gone, dropped := a.track.dropIfPID(appID, pid)
	if !dropped {
		return
	}
	dur := time.Since(gone.since)
	log.Printf("■ %s: завершилось (pid %d, наигранно %s)", appID, pid, dur.Round(time.Second))
	a.sendTelemetry("app_exit", map[string]any{
		"app_id": appID, "pid": pid, "duration_sec": int(dur.Seconds()),
	})
}

// attachProcess — дождаться процесса игры после протокольного запуска.
func (a *agent) attachProcess(appID, process string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		rec, still := a.track.get(appID)
		if !still || rec.pid > 0 {
			return // запись сняли или PID уже подставили
		}
		if pid, found := findProcessByName(process); found {
			if a.track.attach(appID, pid) {
				log.Printf("▶ %s: процесс %s найден (pid %d)", appID, process, pid)
				a.sendTelemetry("app_launch", map[string]any{
					"app_id": appID, "pid": pid, "target_kind": string(rec.kind),
				})
			}
			return
		}
	}
	a.track.markLaunched(appID)
	log.Printf("▶ %s: процесс %q не появился за %s — запуск отдан обработчику, PID неизвестен",
		appID, process, timeout)
}

// launchErrorCode — код отказа в ответе /launch. Витрина (web/shell.html,
// NA_REASON) выбирает текст гостю ИМЕННО по code, поэтому «шара не отвечает»
// обязана доехать как path_unreachable, а не общим target_missing.
// Чистая функция, тест.
func launchErrorCode(reason string) string {
	if reason == reasonUnreachable {
		return reasonUnreachable
	}
	return "target_missing" // в т.ч. no_target: для гостя это «не установлена»
}

// targetMissingMessage — человеческий текст ошибки по коду причины
// (чистая функция, тест).
func targetMissingMessage(reason string) string {
	switch reason {
	case reasonUnreachable:
		return "Диск с играми сейчас недоступен"
	case reasonNoTarget:
		return "У приложения не задан путь запуска"
	default:
		return "Игра не установлена на этом компьютере"
	}
}

var stopResultRu = map[string]string{
	"gone":   "уже было закрыто",
	"gentle": "закрылось само",
	"forced": "закрыто принудительно",
	"failed": "НЕ закрылось",
	"no_pid": "PID неизвестен, пропущено",
}

// stopTracked — завершить всё, что запустил агент (session_end).
func (a *agent) stopTracked() {
	recs := a.track.snapshot()
	if len(recs) == 0 {
		return
	}
	targets := make([]stopTarget, 0, len(recs))
	for _, r := range recs {
		targets = append(targets, stopTarget{AppID: r.appID, PID: r.pid})
	}
	log.Printf("⛔ Конец сессии: завершаю запущенное агентом (%d)", len(targets))
	outcomes := stopSequence(targets, a.launch.StopTimeout, 500*time.Millisecond,
		terminateGentle, terminateForce, processAlive, time.Sleep)
	for _, o := range outcomes {
		log.Printf("⛔  · %s (pid %d): %s", o.AppID, o.PID, stopResultRu[o.Result])
		if rec, dropped := a.track.dropIfPID(o.AppID, o.PID); dropped {
			a.sendTelemetry("app_exit", map[string]any{
				"app_id": o.AppID, "pid": o.PID, "reason": "session_end",
				"duration_sec": int(time.Since(rec.since).Seconds()),
			})
		}
	}
	for _, r := range a.track.snapshot() { // подчистить записи без PID
		a.track.drop(r.appID)
	}
}

// ─────────────────────────── агент: HTTP ───────────────────────────

// handleAppsStatus — GET /apps-status: снимок доступности и запущенного.
// Читает только кэш и реестр — отвечает мгновенно даже при мёртвой шаре.
func (a *agent) handleAppsStatus(w http.ResponseWriter, r *http.Request) {
	apps := a.allApps()
	views := runningViews(a.track.snapshot())
	var newest *runningView
	if len(views) > 0 {
		newest = &views[0] // «последняя живая»
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"apps":        a.avail.snapshot(apps),
		"running":     newest,
		"running_all": views,
	})
}

// handleLaunch — POST /launch: честный запуск с проверкой пути и PID.
func (a *agent) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method"})
		return
	}
	var req struct {
		AppID string `json:"app_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "message": "нужен app_id"})
		return
	}
	app, err := a.lookupApp(req.AppID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "unknown_app", "message": err.Error()})
		return
	}

	// 1. Уже запущено? Второй процесс не плодим.
	if pid, ok := a.alreadyRunning(req.AppID, app); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "app_id": req.AppID, "pid": pid,
			"state": "already_running", "code": "already_running",
		})
		return
	}

	// 2. Путь достижим? Кэшу верим, только если он свежий и положительный —
	//    иначе щупаем сейчас (диск мог вернуться минуту назад).
	st, ok := a.avail.fresh(req.AppID, a.launch.CheckInterval)
	if !ok || !st.Available {
		st = probeTarget(app.Target, statWithTimeout(a.launch.CheckTimeout))
		a.avail.set(req.AppID, st)
	}
	if !st.Available {
		log.Printf("✗ Запуск %s отклонён: %s (%s)", req.AppID, st.Reason, app.Target)
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "app_id": req.AppID, "code": launchErrorCode(st.Reason),
			"reason": st.Reason, "message": targetMissingMessage(st.Reason),
		})
		return
	}

	// 3. Запуск. С2-и3: CS2 в exec-режиме сенсы стартует с +exec 1448 —
	//    игра читает game/csgo/cfg/1448.cfg (проверка пути выше шла по
	//    исходному target, стартуем по преобразованному).
	target, args := withSensArgs(req.AppID, app.Target, app.Args, a.cfg.Sens.Enabled && a.cfg.Sens.cs2Mode() == "exec")
	if st.Kind == kindProtocol {
		if err := startProtocol(target, args); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"ok": false, "app_id": req.AppID, "code": "launch_failed", "message": err.Error(),
			})
			return
		}
		state := stateLaunched
		if app.Process != "" && a.launch.AttachTimeout > 0 {
			state = stateStarting
		}
		a.track.put(procRecord{appID: req.AppID, state: state, process: app.Process, kind: st.Kind})
		log.Printf("▶ Запуск %s → %s (обработчик системы)", req.AppID, target)
		if state == stateStarting {
			go a.attachProcess(req.AppID, app.Process, a.launch.AttachTimeout)
		} else {
			a.sendTelemetry("app_launch", map[string]any{"app_id": req.AppID, "pid": 0, "target_kind": string(st.Kind)})
		}
		// state честный: starting = ждём процесс по имени, launched = PID
		// узнать неоткуда. Витрина в обоих случаях показывает «запускается» и
		// дальше следит за /apps-status.
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "app_id": req.AppID, "pid": 0, "state": state,
		})
		return
	}

	cmd, err := startFile(target, args, app.Dir)
	if err != nil {
		log.Printf("✗ Запуск %s не удался: %v", req.AppID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok": false, "app_id": req.AppID, "code": "launch_failed", "message": err.Error(),
		})
		return
	}
	pid := cmd.Process.Pid
	a.track.put(procRecord{appID: req.AppID, pid: pid, state: stateRunning, process: app.Process, kind: st.Kind})
	log.Printf("▶ Запуск %s → %s (pid %d)", req.AppID, app.Target, pid)
	a.sendTelemetry("app_launch", map[string]any{"app_id": req.AppID, "pid": pid, "target_kind": string(st.Kind)})
	go func() {
		_ = cmd.Wait() // и процесс не зомби, и о завершении узнаём сразу
		a.processGone(req.AppID, pid)
	}()
	// state из того же словаря, что и в /apps-status, — витрина зажигает
	// плитку «запущена» сразу, не дожидаясь следующего опроса.
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "app_id": req.AppID, "pid": pid, "state": stateRunning,
	})
}

// alreadyRunning — приложение уже работает? Подхватывает и процесс, который
// агент не запускал (гость поднял игру мимо витрины) — чтобы и его закрыть
// по концу сессии.
func (a *agent) alreadyRunning(appID string, app AppEntry) (int, bool) {
	if rec, ok := a.track.get(appID); ok {
		if rec.pid > 0 && processAlive(rec.pid) {
			return rec.pid, true
		}
		if rec.state == stateStarting {
			return 0, true // запуск в процессе, второй клик игнорируем
		}
	}
	if app.Process != "" {
		if pid, found := findProcessByName(app.Process); found {
			a.track.put(procRecord{appID: appID, pid: pid, state: stateRunning,
				process: app.Process, kind: classifyTarget(app.Target)})
			log.Printf("▶ %s: процесс %s уже работает (pid %d) — беру под контроль", appID, app.Process, pid)
			return pid, true
		}
	}
	return 0, false
}

// onSessionEnd — что агент делает по концу сессии: сперва корректно закрыть
// то, что запустил сам (мягко → принудительно, с таймаутом), потом прежняя
// очистка между гостями по списку имён из конфига (Г9-и4, cleanup.go).
func (a *agent) onSessionEnd() {
	if a.launch.StopOnEnd {
		a.stopTracked()
	}
	runCleanup(a.cfg.Cleanup)
	if n := a.sens.restore(); n > 0 { // К7: конфиги игр — как были до гостя
		log.Printf("сенса: после сессии возвращено конфигов: %d", n)
	}
}
