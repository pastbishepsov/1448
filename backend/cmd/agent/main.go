package main

// shell-agent — локальный агент гостевого ПК (спринт 2, демо-класс).
//
// Делает две вещи:
//  1. HTTP на 127.0.0.1:1448 для гостевого экрана (web/shell.html):
//     запуск программ по allowlist из agent.json и вызов администратора.
//  2. WS-связь с бэкендом (/ws/shell): heartbeat session_tick, приём команд
//     session_start / session_end (блокировка экрана — спринт 3).
//
// Безопасность: слушает ТОЛЬКО 127.0.0.1; выполняет ТОЛЬКО записи из
// allowlist конфига — произвольные команды с экрана невозможны.
//
// Сборка Windows-exe через Docker (из корня репо):
//   docker compose run --rm --no-deps -e GOOS=windows -e GOARCH=amd64 \
//     backend go build -o bin/shell-agent.exe ./cmd/agent
// Экзешник появится в backend/bin/shell-agent.exe.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ─────────────────────────── конфиг ───────────────────────────

type AppEntry struct {
	Name   string   `json:"name,omitempty"`
	Target string   `json:"target"` // exe-путь, протокол (steam://...) или ms-settings:
	Args   []string `json:"args,omitempty"`
	Dir    string   `json:"dir,omitempty"` // рабочая папка; пусто = папка exe
	// Process — имя процесса игры (cs2.exe). Нужно там, где PID запуска не
	// принадлежит игре: протоколы (steam://) и лаунчеры, которые сами
	// закрываются после старта игры (Riot Client → VALORANT).
	Process string `json:"process,omitempty"`
}

type Config struct {
	BackendWS  string              `json:"backend_ws"`  // ws://localhost:8080/api/v1/ws/shell
	ComputerID string              `json:"computer_id"` // UUID ПК из таблицы computers
	Listen     string              `json:"listen"`      // 127.0.0.1:1448
	CatalogURL string              `json:"catalog_url"` // откуда тянуть каталог (пусто = не тянуть)
	LockAction string              `json:"lock_action"` // none | lock_windows | kiosk
	KioskURL   string              `json:"kiosk_url"`   // для lock_action=kiosk
	Apps       map[string]AppEntry `json:"apps"`        // локальный allowlist (главнее каталога)
	// GamesRoot — корень игр ИМЕННО ЭТОГО ПК, если он отличается от клубного
	// (у клуба D:\online, а тут, скажем, E:\online). Пусто — обычный случай:
	// берём готовый target, который сервер уже развернул своим games_root.
	// Задан — разворачиваем шаблон target_raw сами (docs/КАТАЛОГ-ИГР.md §2).
	GamesRoot string        `json:"games_root"`
	Cleanup   CleanupConfig `json:"cleanup"` // Г9-и4: очистка между гостями (cleanup.go)
	Launch    LaunchConfig  `json:"launch"`  // жизненный цикл игры (launch.go)
}

func defaultConfig() *Config {
	return &Config{
		BackendWS:  "ws://localhost:8080/api/v1/ws/shell",
		Listen:     "127.0.0.1:1448",
		CatalogURL: "http://localhost:8080/api/v1/catalog",
		LockAction: "none",
		Apps:       map[string]AppEntry{},
	}
}

func loadConfig(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig(), fmt.Errorf("agent.json повреждён: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:1448"
	}
	if cfg.BackendWS == "" {
		cfg.BackendWS = "ws://localhost:8080/api/v1/ws/shell"
	}
	return cfg, nil
}

// ─────────────────────────── запуск программ ───────────────────────────

var errUnknownApp = errors.New("приложение не в allowlist")

// resolveApp — чистая проверка allowlist (тестируется отдельно).
func resolveApp(cfg *Config, id string) (AppEntry, error) {
	app, ok := cfg.Apps[id]
	if !ok || app.Target == "" {
		return AppEntry{}, fmt.Errorf("%w: %q", errUnknownApp, id)
	}
	return app, nil
}

// startDetached — «просто открой это и не жди» (kiosk_url в applyLockAction).
// Запуск игр идёт не через него, а через handleLaunch: там нужны проверка
// пути, PID и трекинг (launch.go).
func startDetached(target string, args []string) error {
	if classifyTarget(target) == kindFile {
		_, err := startFile(target, args, "")
		return err
	}
	return startProtocol(target, args)
}

// ─────────────────────────── WS-клиент ───────────────────────────

type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type agent struct {
	cfg    *Config
	mu     sync.Mutex
	conn   *websocket.Conn     // nil = связи нет
	remote map[string]AppEntry // allowlist из каталога сервера

	launch launchSettings // блок "launch" из конфига, с дефолтами
	avail  *availability  // кэш доступности target'ов (launch.go)
	track  *tracker       // что агент запустил на этом ПК (launch.go)
}

// ── Каталог с сервера (Admin Panel → все гостевые ПК) ──

type catalogItem struct {
	ID        string  `json:"id"`
	Target    *string `json:"target"`     // путь с уже подставленным корнем клуба
	TargetRaw *string `json:"target_raw"` // исходный шаблон {GAMES}\… (если он был)
	Args      *string `json:"args"`       // JSON-массив строк
}

type catalogResponse struct {
	Games     []catalogItem `json:"games"`
	Apps      []catalogItem `json:"apps"`
	System    []catalogItem `json:"system"`
	Platforms []catalogItem `json:"platforms"`  // клиенты платформ (Steam, Riot, Epic...)
	GamesRoot string        `json:"games_root"` // корень игр клуба (справочно, для лога)
}

// gamesPlaceholder — та же переменная, что и на сервере (docs/КАТАЛОГ-ИГР.md §2).
const gamesPlaceholder = "{GAMES}"

// expandGamesRoot — подстановка {GAMES} своим корнем. Ровно та же, что делает
// сервер: хвостовой слэш корня срезается, иначе «D:\online\» + «\Steam» дало бы
// «D:\online\\Steam». Для args корень попадает внутрь строки JSON, поэтому там
// обратные слэши экранируются. Чистая функция, тест.
func expandGamesRoot(s, root string, jsonEscaped bool) string {
	if s == "" || root == "" || !strings.Contains(s, gamesPlaceholder) {
		return s
	}
	r := strings.TrimRight(root, `\/`)
	if jsonEscaped {
		r = strings.ReplaceAll(r, `\`, `\\`)
	}
	return strings.ReplaceAll(s, gamesPlaceholder, r)
}

// remoteAllowlist — чистая сборка allowlist из каталога (тестируется отдельно).
//
// localRoot — games_root этого ПК из agent.json. Пусто (обычный случай) —
// берём target, который сервер уже развернул клубным корнем. Задан — сами
// разворачиваем шаблон target_raw: так ПК с другой буквой диска не требует
// правок на сервере и не ломает остальным путь (docs/КАТАЛОГ-ИГР.md §2).
func remoteAllowlist(items []catalogItem, localRoot string) map[string]AppEntry {
	out := map[string]AppEntry{}
	for _, it := range items {
		target := ""
		if it.Target != nil {
			target = *it.Target
		}
		if localRoot != "" && it.TargetRaw != nil && *it.TargetRaw != "" {
			target = expandGamesRoot(*it.TargetRaw, localRoot, false)
		}
		if it.ID == "" || target == "" {
			continue
		}
		e := AppEntry{Target: target}
		if it.Args != nil && *it.Args != "" {
			raw := expandGamesRoot(*it.Args, localRoot, true)
			var args []string
			if json.Unmarshal([]byte(raw), &args) == nil {
				e.Args = args
			}
		}
		out[it.ID] = e
	}
	return out
}

func (a *agent) refreshCatalog() {
	if a.cfg.CatalogURL == "" {
		return
	}
	resp, err := http.Get(a.cfg.CatalogURL)
	if err != nil {
		log.Printf("Каталог: сервер недоступен (%v)", err)
		return
	}
	defer resp.Body.Close()
	var cr catalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		log.Printf("Каталог: некорректный ответ (%v)", err)
		return
	}
	items := append(append(append(cr.Games, cr.Apps...), cr.System...), cr.Platforms...)
	remote := remoteAllowlist(items, a.cfg.GamesRoot)
	a.mu.Lock()
	a.remote = remote
	a.mu.Unlock()
	if a.cfg.GamesRoot != "" {
		log.Printf("Каталог с сервера: %d приложений с запуском (корень этого ПК: %s, у клуба %s)",
			len(remote), a.cfg.GamesRoot, cr.GamesRoot)
	} else {
		log.Printf("Каталог с сервера: %d приложений с запуском", len(remote))
	}
	go a.refreshAvailability() // новые игры проверяем сразу, а не через цикл
}

func (a *agent) runCatalog() {
	a.refreshCatalog()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		a.refreshCatalog()
	}
}

// lookupApp — локальный agent.json главнее, затем каталог сервера.
func (a *agent) lookupApp(id string) (AppEntry, error) {
	if app, err := resolveApp(a.cfg, id); err == nil {
		return app, nil
	}
	a.mu.Lock()
	app, ok := a.remote[id]
	a.mu.Unlock()
	if ok {
		return app, nil
	}
	return AppEntry{}, fmt.Errorf("%w: %q", errUnknownApp, id)
}

// wolMagicPacket — magic-пакет Wake-on-LAN: 6×0xFF + 16×MAC
// (чистая функция, тест в agent_test.go).
func wolMagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil {
		return nil, err
	}
	if len(hw) != 6 {
		return nil, fmt.Errorf("нужен 48-битный MAC")
	}
	p := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		p = append(p, 0xFF)
	}
	for i := 0; i < 16; i++ {
		p = append(p, hw...)
	}
	return p, nil
}

// sendWOL — послать magic-пакет broadcast'ом в LAN (UDP :9).
func sendWOL(mac string) error {
	p, err := wolMagicPacket(mac)
	if err != nil {
		return err
	}
	conn, err := net.Dial("udp", "255.255.255.255:9")
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(p)
	return err
}

// applyPCPower — перезагрузка/выключение (Б8): зашитые действия, только Windows.
func applyPCPower(action string) {
	if runtime.GOOS != "windows" {
		log.Printf("pc_power %s: поддерживается только Windows", action)
		return
	}
	switch action {
	case "restart":
		_ = exec.Command("shutdown", "/r", "/t", "0").Start()
	case "shutdown":
		_ = exec.Command("shutdown", "/s", "/t", "0").Start()
	default:
		log.Printf("pc_power: неизвестное действие %q", action)
	}
}

// applyLockAction — реакция на конец сессии (настраивается в agent.json).
func (a *agent) applyLockAction() {
	switch a.cfg.LockAction {
	case "lock_windows":
		if runtime.GOOS == "windows" {
			_ = exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Start()
			log.Println("🔒 lock_action=lock_windows: экран Windows заблокирован")
		}
	case "kiosk":
		if a.cfg.KioskURL != "" {
			_ = startDetached(a.cfg.KioskURL, nil)
			log.Println("🔒 lock_action=kiosk: гостевой экран поднят поверх")
		}
	default:
		log.Println("lock_action=none: блокировку показывает сам гостевой экран")
	}
}

func (a *agent) wsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn != nil
}

func (a *agent) wsSend(msg wsMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return errors.New("нет связи с бэкендом")
	}
	return a.conn.WriteJSON(msg)
}

func (a *agent) runWS() {
	if a.cfg.ComputerID == "" {
		log.Println("⚠ computer_id пуст — WS отключён (запуск приложений работает).")
		log.Println("  UUID ПК: docker compose exec postgres psql -U 1448_user -d 1448_db -c \"SELECT id,name FROM computers;\"")
		return
	}
	url := a.cfg.BackendWS + "?computer_id=" + a.cfg.ComputerID
	for {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Printf("WS: бэкенд недоступен (%v), повтор через 5с", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Println("WS: связь с бэкендом установлена ✓")
		a.mu.Lock()
		a.conn = conn
		a.mu.Unlock()

		stop := make(chan struct{})
		go a.heartbeat(stop)
		for {
			var msg wsMessage
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}
			a.handleCommand(msg)
		}
		close(stop)
		a.mu.Lock()
		a.conn = nil
		a.mu.Unlock()
		conn.Close()
		log.Println("WS: связь потеряна, переподключаюсь…")
	}
}

func (a *agent) heartbeat(stop <-chan struct{}) {
	runHeartbeat(60*time.Second, stop, func() error {
		// Г2: в heartbeat едет AFK-датчик — секунды простоя ввода
		// (GetLastInputInfo, idle_windows.go; -1 = датчика нет).
		payload, _ := json.Marshal(map[string]int{"idle_sec": idleSeconds()})
		return a.wsSend(wsMessage{Type: "session_tick", Payload: payload})
	})
}

// runHeartbeat — таймерный цикл heartbeat'а: раз в interval зовёт send, пока
// не закрыт stop или send не вернул ошибку (связь упала — цикл умирает вместе
// с соединением, runWS поднимет новый). Чистая функция — тестируется на
// виртуальном времени (heartbeat_test.go, testing/synctest).
func runHeartbeat(interval time.Duration, stop <-chan struct{}, send func() error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := send(); err != nil {
				return
			}
		}
	}
}

func (a *agent) handleCommand(msg wsMessage) {
	switch msg.Type {
	case "session_start":
		log.Printf("Команда: session_start %s", string(msg.Payload))
		disableAccessibilityShortcuts() // Г9-и3: 5×Shift не роняет гостя в диалог Windows
	case "session_end":
		log.Printf("Команда: session_end %s", string(msg.Payload))
		a.applyLockAction()
		// Фоном — экран уже заблокирован (тело в launch.go, чтобы порядок
		// «сначала запущенное агентом, потом общая очистка» был тестируемым).
		go a.onSessionEnd()
	case "lock":
		// К0-и1 (KIOSK.md, Р1): пауза или AFK — экран запереть, игру НЕ трогать.
		// Раньше сервер слал сюда session_end, и агент закрывал матч гостя.
		// Закрытие игры и очистка между гостями — только по session_end.
		var p struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(msg.Payload, &p)
		log.Printf("Команда: lock (%s) — экран заперт, запущенное остаётся", p.Reason)
		a.applyLockAction()
	case "unlock":
		// Пауза снята: с lock_action=none/kiosk снимать нечего — гостевой экран
		// сам показывает, что сессия идёт; lock_windows отпирает гость паролем.
		log.Printf("Команда: unlock %s", string(msg.Payload))
	case "xp_update":
		// Пробрасывать некуда: браузерный шелл живёт поллингом и
		// уведомлениями (Б4); XP-оверлей — задача C#-киоска (shell/).
		log.Printf("Команда: xp_update %s", string(msg.Payload))
	case "pc_power": // Б8: только restart|shutdown, ничего другого
		var p struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(msg.Payload, &p)
		log.Printf("Команда: pc_power %s", p.Action)
		applyPCPower(p.Action)
	case "wol": // Б8: разбудить соседа по MAC (агент — WoL-прокси в LAN)
		var p struct {
			MAC string `json:"mac"`
		}
		_ = json.Unmarshal(msg.Payload, &p)
		if err := sendWOL(p.MAC); err != nil {
			log.Printf("wol: %v", err)
		} else {
			log.Printf("🔌 WoL отправлен: %s", p.MAC)
		}
	default:
		log.Printf("Неизвестная команда: %s", msg.Type)
	}
}

// ─────────────────────────── HTTP для гостевого экрана ───────────────────────────

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	configPath := flag.String("config", "agent.json", "путь к конфигу")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Printf("⚠ %v — использую дефолты (allowlist пуст!)", err)
	}
	a := &agent{
		cfg:    cfg,
		remote: map[string]AppEntry{},
		launch: cfg.Launch.settings(),
		avail:  newAvailability(nil),
		track:  newTracker(nil),
	}
	disableAccessibilityShortcuts() // Г9-и3: и сразу на старте — ПК клубный, «как было» не нужно
	go a.runWS()
	go a.runCatalog()
	go a.runAvailability() // фоновая проверка путей к играм, в т.ч. сетевых
	go a.runProcWatch()    // заметить, что игра закрылась

	mux := http.NewServeMux()

	mux.HandleFunc("/ping", cors(func(w http.ResponseWriter, r *http.Request) {
		apps := a.allApps()
		ids := make([]string, 0, len(apps))
		for id := range apps {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok", "ws_connected": a.wsConnected(),
			"computer_id": cfg.ComputerID, "apps": ids,
			"lock_action": cfg.LockAction,
		})
	}))

	// Студия сенсы (S2): чтение настроек мыши и выключение акселерации.
	mux.HandleFunc("/mouse-info", cors(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, readMouse())
	}))
	mux.HandleFunc("/mouse-accel-off", cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method"})
			return
		}
		if err := disableAccel(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "accel_failed", "message": err.Error()})
			return
		}
		log.Println("🖱 Акселерация мыши выключена по запросу гостевого экрана")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accel": false})
	}))

	// Запуск игры: проверка достижимости target, честный PID, коды ошибок
	// target_missing / launch_failed / already_running (launch.go).
	mux.HandleFunc("/launch", cors(a.handleLaunch))

	// Доступность игр и что сейчас запущено — это читает гостевая витрина.
	// Отвечает из кэша, в сеть на запросе не ходит.
	mux.HandleFunc("/apps-status", cors(a.handleAppsStatus))

	mux.HandleFunc("/admin-call", cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method"})
			return
		}
		if err := a.wsSend(wsMessage{Type: "admin_call"}); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "ws_down", "message": err.Error()})
			return
		}
		log.Println("🛎 Вызов администратора отправлен")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))

	log.Printf("14:48 shell-agent слушает http://%s (конфиг: %s, приложений: %d)", cfg.Listen, *configPath, len(cfg.Apps))
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Fatalf("Ошибка запуска: %v", err)
	}
}
