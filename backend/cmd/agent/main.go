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
}

type Config struct {
	BackendWS  string              `json:"backend_ws"`  // ws://localhost:8080/api/v1/ws/shell
	ComputerID string              `json:"computer_id"` // UUID ПК из таблицы computers
	Listen     string              `json:"listen"`      // 127.0.0.1:1448
	CatalogURL string              `json:"catalog_url"` // откуда тянуть каталог (пусто = не тянуть)
	LockAction string              `json:"lock_action"` // none | lock_windows | kiosk
	KioskURL   string              `json:"kiosk_url"`   // для lock_action=kiosk
	Apps       map[string]AppEntry `json:"apps"`        // локальный allowlist (главнее каталога)
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

// startDetached — запустить и не ждать. На Windows `start` понимает и exe-пути,
// и протоколы (steam://, discord://), и ms-settings:.
func startDetached(target string, args []string) error {
	switch runtime.GOOS {
	case "windows":
		cmdArgs := append([]string{"/C", "start", "", target}, args...)
		return exec.Command("cmd", cmdArgs...).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
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
}

// ── Каталог с сервера (Admin Panel → все гостевые ПК) ──

type catalogItem struct {
	ID     string  `json:"id"`
	Target *string `json:"target"`
	Args   *string `json:"args"` // JSON-массив строк
}

type catalogResponse struct {
	Games     []catalogItem `json:"games"`
	Apps      []catalogItem `json:"apps"`
	System    []catalogItem `json:"system"`
	Platforms []catalogItem `json:"platforms"` // клиенты платформ (Steam, Riot, Epic...)
}

// remoteAllowlist — чистая сборка allowlist из каталога (тестируется отдельно).
func remoteAllowlist(items []catalogItem) map[string]AppEntry {
	out := map[string]AppEntry{}
	for _, it := range items {
		if it.ID == "" || it.Target == nil || *it.Target == "" {
			continue
		}
		e := AppEntry{Target: *it.Target}
		if it.Args != nil && *it.Args != "" {
			var args []string
			if json.Unmarshal([]byte(*it.Args), &args) == nil {
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
	remote := remoteAllowlist(items)
	a.mu.Lock()
	a.remote = remote
	a.mu.Unlock()
	log.Printf("Каталог с сервера: %d приложений с запуском", len(remote))
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
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := a.wsSend(wsMessage{Type: "session_tick"}); err != nil {
				return
			}
		}
	}
}

func (a *agent) handleCommand(msg wsMessage) {
	switch msg.Type {
	case "session_start":
		log.Printf("Команда: session_start %s", string(msg.Payload))
	case "session_end":
		log.Printf("Команда: session_end %s", string(msg.Payload))
		a.applyLockAction()
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
	a := &agent{cfg: cfg, remote: map[string]AppEntry{}}
	go a.runWS()
	go a.runCatalog()

	mux := http.NewServeMux()

	mux.HandleFunc("/ping", cors(func(w http.ResponseWriter, r *http.Request) {
		seen := map[string]bool{}
		for id := range cfg.Apps {
			seen[id] = true
		}
		a.mu.Lock()
		for id := range a.remote {
			seen[id] = true
		}
		a.mu.Unlock()
		ids := make([]string, 0, len(seen))
		for id := range seen {
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

	mux.HandleFunc("/launch", cors(func(w http.ResponseWriter, r *http.Request) {
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
		if err := startDetached(app.Target, app.Args); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "launch_failed", "message": err.Error()})
			return
		}
		log.Printf("Запуск: %s → %s", req.AppID, app.Target)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))

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
