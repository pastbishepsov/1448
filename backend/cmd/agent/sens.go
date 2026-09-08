package main

// К7 «Сенса в игру» (KIOSK.md, решение основателя Р9): профиль сенсы гостя
// (DPI + сенса по играм, GET /me/sensitivity) подставляется в конфиги игр
// НА ЭТОМ ПК — гость садится, и CS2 стартует с его сенсой.
//
// Как это работает:
//   - блок "sens" в agent.json: enabled (выкл по умолчанию), dry_run (ВКЛ по
//     умолчанию — только лог, файлы не трогаем; боевой режим владелец
//     включает после сухого прогона на пилотном ПК), steam_dir / cs2_cfg /
//     apex_cfg — где лежат конфиги (пусто — ищем сами);
//   - кто просит: гостевой экран (POST /sens-apply {dpi, games} — кнопка
//     «Применить в игре» в студии сенсы) и сервер — в session_start приходит
//     sens {dpi, games}, если у гостя сохранён профиль (автоматически);
//   - что умеем (и1/и2): CS2 — "sensitivity" в cs2_user_convars_0_slot0.vcfg
//     пользователя Steam; Apex — mouse_sensitivity в settings.cfg. Valorant
//     НЕ умеем — настройки в облаке Riot (об этом честно говорит студия);
//     OW2/CoD/Quake/Deadlock — нет локального конфига, который стоит трогать;
//   - откат: перед первой записью в сессии файл копируется в <файл>.1448bak,
//     на session_end (очистка между гостями) бэкап возвращается на место;
//     на старте агента найденные бэкапы возвращаются тоже (агент упал посреди
//     сессии). Значит, следующий гость не унаследует чужую сенсу, а профиль
//     клубного аккаунта не «расползается».
//
// Оговорка (KIOSK.md): у общего клубного Steam-аккаунта включённый Cloud Sync
// может перезаписать vcfg при старте игры — проверяется на пилотном ПК; если
// подтвердится, следующий шаг — сенса через параметры запуска (+sensitivity).

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SensConfig — блок "sens" в agent.json.
type SensConfig struct {
	Enabled  bool   `json:"enabled"`   // выкл по умолчанию
	DryRun   *bool  `json:"dry_run"`   // nil = true: только лог
	SteamDir string `json:"steam_dir"` // корень Steam; пусто — стандартные пути
	CS2Cfg   string `json:"cs2_cfg"`   // явный путь к cs2_user_convars_0_slot0.vcfg
	ApexCfg  string `json:"apex_cfg"`  // явный путь к settings.cfg Apex
}

func (c SensConfig) dryRun() bool { return c.DryRun == nil || *c.DryRun }

const sensBackupSuffix = ".1448bak"

// sensProfile — что подставляем: DPI (для лога) и сенса по играм.
type sensProfile struct {
	DPI   int                `json:"dpi"`
	Games map[string]float64 `json:"games"`
}

// sensGameKind — формат конфига игры.
type sensGameKind int

const (
	sensKindVCFG sensGameKind = iota // Valve KeyValues: "sensitivity" "2.5" внутри "convars" { }
	sensKindApex                     // строки вида: mouse_sensitivity "1.400000"
)

// sensTarget — конфиг одной игры на этом ПК.
type sensTarget struct {
	Game string
	Path string
	Key  string
	Kind sensGameKind
}

// sensUnsupported — почему игру не подставляем (для ответа студии).
var sensUnsupported = map[string]string{
	"valorant": "cloud",       // настройки в облаке Riot — локального файла нет
	"ow2":      "unsupported", // нет локального конфига, который стоит трогать
	"cod":      "unsupported",
	"quake":    "unsupported",
	"deadlock": "unsupported",
}

// sensGamesSupported — что умеем (для /ping и подсказки в студии).
var sensGamesSupported = []string{"apex", "cs2"}

// formatSens — число для конфига: 2.5 → "2.5", 2 → "2", лишние нули не пишем.
func formatSens(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// ── чистые правки конфигов (тесты) ──

var (
	vcfgKeyRe  = regexp.MustCompile(`^(\s*)"([^"]+)"(\s+)"([^"]*)"(\s*)$`)
	apexLineRe = regexp.MustCompile(`^(\s*)([A-Za-z0-9_]+)(\s+)"([^"]*)"(\s*)$`)
)

// splitLines — строки с сохранением перевода строки каждой (CRLF/LF как есть).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

func lineEnding(lines []string) string {
	for _, l := range lines {
		if strings.HasSuffix(l, "\r\n") {
			return "\r\n"
		}
		if strings.HasSuffix(l, "\n") {
			return "\n"
		}
	}
	return "\n"
}

// vcfgSetConvar — поставить "key" "value" внутри блока "convars" { … } файла
// KeyValues, не трогая остальные строки байт в байт. Ключа нет — вставляем
// первой строкой блока; блока нет — дописываем целиком. Возвращает новый
// текст и был ли он изменён.
func vcfgSetConvar(content, key, value string) (string, bool) {
	lines := splitLines(content)
	nl := lineEnding(lines)
	trimNL := func(l string) string { return strings.TrimRight(l, "\r\n") }
	inConvars, depth := false, 0
	convarsOpen := -1
	for i, raw := range lines {
		l := trimNL(raw)
		t := strings.TrimSpace(l)
		if !inConvars {
			if strings.EqualFold(t, `"convars"`) {
				inConvars = true
				depth = 0
				continue
			}
			continue
		}
		if t == "{" {
			depth++
			if depth == 1 {
				convarsOpen = i
			}
			continue
		}
		if t == "}" {
			depth--
			if depth == 0 { // блок закрылся, ключа не было — вставляем в начало блока
				if convarsOpen >= 0 {
					indent := "\t\t"
					if m := vcfgKeyRe.FindStringSubmatch(trimNL(lines[convarsOpen+1])); m != nil && convarsOpen+1 < i {
						indent = m[1]
					}
					newLine := indent + `"` + key + `"` + "\t\t" + `"` + value + `"` + nl
					out := append([]string{}, lines[:convarsOpen+1]...)
					out = append(out, newLine)
					out = append(out, lines[convarsOpen+1:]...)
					return strings.Join(out, ""), true
				}
				break
			}
			continue
		}
		if depth != 1 {
			continue
		}
		if m := vcfgKeyRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[2], key) {
			if m[4] == value {
				return content, false
			}
			lines[i] = m[1] + `"` + m[2] + `"` + m[3] + `"` + value + `"` + m[5] + nl
			return strings.Join(lines, ""), true
		}
	}
	// блока convars нет (пустой/чужой файл) — дописываем структуру целиком
	if strings.TrimSpace(content) == "" {
		content = ""
	} else if !strings.HasSuffix(content, "\n") {
		content += nl
	}
	block := `"config"` + nl + "{" + nl + "\t" + `"convars"` + nl + "\t{" + nl +
		"\t\t" + `"` + key + `"` + "\t\t" + `"` + value + `"` + nl + "\t}" + nl + "}" + nl
	return content + block, true
}

// apexCfgSet — поставить key "value" в settings.cfg Apex (плоский список
// строк `имя "значение"`); нет ключа — дописать в конец.
func apexCfgSet(content, key, value string) (string, bool) {
	lines := splitLines(content)
	nl := lineEnding(lines)
	for i, raw := range lines {
		l := strings.TrimRight(raw, "\r\n")
		if m := apexLineRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[2], key) {
			if m[4] == value {
				return content, false
			}
			lines[i] = m[1] + m[2] + m[3] + `"` + value + `"` + m[5] + nl
			return strings.Join(lines, ""), true
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += nl
	}
	return content + key + ` "` + value + `"` + nl, true
}

// ── где лежат конфиги ──

// steamDirs — кандидаты корня Steam: из конфига, из окружения, стандартные.
func steamDirs(cfg SensConfig, getenv func(string) string) []string {
	var out []string
	add := func(p string) {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	add(cfg.SteamDir)
	if pf := getenv("ProgramFiles(x86)"); pf != "" {
		add(filepath.Join(pf, "Steam"))
	}
	if pf := getenv("ProgramFiles"); pf != "" {
		add(filepath.Join(pf, "Steam"))
	}
	add(`C:\Program Files (x86)\Steam`)
	add(`C:\Program Files\Steam`)
	if home := getenv("HOME"); home != "" { // dev-стенд на Linux
		add(filepath.Join(home, ".steam", "steam"))
	}
	return out
}

// findCS2Cfg — cs2_user_convars_0_slot0.vcfg: явный путь из конфига, иначе
// самый свежий среди userdata/*/730/local/cfg/.
func findCS2Cfg(cfg SensConfig, getenv func(string) string) string {
	if cfg.CS2Cfg != "" {
		return cfg.CS2Cfg
	}
	var best string
	var bestMod int64
	for _, dir := range steamDirs(cfg, getenv) {
		matches, _ := filepath.Glob(filepath.Join(dir, "userdata", "*", "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg"))
		for _, m := range matches {
			st, err := os.Stat(m)
			if err != nil {
				continue
			}
			if best == "" || st.ModTime().Unix() > bestMod {
				best, bestMod = m, st.ModTime().Unix()
			}
		}
		if best != "" {
			break
		}
	}
	return best
}

// findApexCfg — settings.cfg Apex: явный путь, иначе Saved Games текущего пользователя.
func findApexCfg(cfg SensConfig, getenv func(string) string) string {
	if cfg.ApexCfg != "" {
		return cfg.ApexCfg
	}
	if up := getenv("USERPROFILE"); up != "" {
		return filepath.Join(up, "Saved Games", "Respawn", "Apex", "local", "settings.cfg")
	}
	return ""
}

// sensTargets — конфиги игр, которые умеем править на этом ПК.
func sensTargets(cfg SensConfig, getenv func(string) string) map[string]sensTarget {
	out := map[string]sensTarget{}
	if p := findCS2Cfg(cfg, getenv); p != "" {
		out["cs2"] = sensTarget{Game: "cs2", Path: p, Key: "sensitivity", Kind: sensKindVCFG}
	}
	if p := findApexCfg(cfg, getenv); p != "" {
		out["apex"] = sensTarget{Game: "apex", Path: p, Key: "mouse_sensitivity", Kind: sensKindApex}
	}
	return out
}

// ── применение и откат ──

// sensResult — что вышло (ответ студии и лог).
type sensResult struct {
	Applied []string          `json:"applied"` // игры, куда записали (в dry_run — куда записали БЫ)
	Skipped map[string]string `json:"skipped"` // игра → причина: cloud | unsupported | no_config | error
	DryRun  bool              `json:"dry_run"`
}

// sensApplier — состояние подстановки на этом ПК: какие файлы подменены.
type sensApplier struct {
	cfg    SensConfig
	getenv func(string) string
	mu     sync.Mutex
	backed map[string]bool // путь → бэкап уже сделан в этой сессии
}

func newSensApplier(cfg SensConfig) *sensApplier {
	return &sensApplier{cfg: cfg, getenv: os.Getenv, backed: map[string]bool{}}
}

// apply — подставить профиль в конфиги поддерживаемых игр.
func (s *sensApplier) apply(p sensProfile) sensResult {
	if s == nil { // агент собран без блока sens (тесты)
		return sensResult{Applied: []string{}, Skipped: map[string]string{}, DryRun: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res := sensResult{Applied: []string{}, Skipped: map[string]string{}, DryRun: s.cfg.dryRun()}
	targets := sensTargets(s.cfg, s.getenv)
	games := make([]string, 0, len(p.Games))
	for g := range p.Games {
		games = append(games, g)
	}
	sort.Strings(games)
	for _, g := range games {
		v := p.Games[g]
		if v <= 0 {
			continue
		}
		if why, ok := sensUnsupported[g]; ok {
			res.Skipped[g] = why
			continue
		}
		t, ok := targets[g]
		if !ok { // умеем, но конфиг не нашли — или игру не знаем вовсе
			if slices.Contains(sensGamesSupported, g) {
				res.Skipped[g] = "no_config"
			} else {
				res.Skipped[g] = "unsupported"
			}
			continue
		}
		if _, err := os.Stat(t.Path); err != nil { // игра не стояла / конфиг ещё не создан
			res.Skipped[g] = "no_config"
			continue
		}
		if err := s.applyOne(t, formatSens(v)); err != nil {
			log.Printf("сенса: %s — %v", g, err)
			res.Skipped[g] = "error"
			continue
		}
		res.Applied = append(res.Applied, g)
	}
	return res
}

func (s *sensApplier) applyOne(t sensTarget, value string) error {
	data, err := os.ReadFile(t.Path)
	if err != nil {
		return fmt.Errorf("конфиг не читается (%s): %w", t.Path, err)
	}
	var next string
	var changed bool
	switch t.Kind {
	case sensKindApex:
		next, changed = apexCfgSet(string(data), t.Key, value)
	default:
		next, changed = vcfgSetConvar(string(data), t.Key, value)
	}
	if !changed {
		log.Printf("сенса: %s уже %s = %s (%s)", t.Game, t.Key, value, t.Path)
		return nil
	}
	if s.cfg.dryRun() {
		log.Printf("сенса [пробный режим]: %s — записал бы %s = %s в %s", t.Game, t.Key, value, t.Path)
		return nil
	}
	if !s.backed[t.Path] { // первая запись в сессии — бэкап оригинала
		bak := t.Path + sensBackupSuffix
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, data, 0o644); err != nil {
				return fmt.Errorf("бэкап не записался: %w", err)
			}
		}
		s.backed[t.Path] = true
	}
	if err := os.WriteFile(t.Path, []byte(next), 0o644); err != nil {
		return fmt.Errorf("конфиг не записался: %w", err)
	}
	log.Printf("сенса: %s — %s = %s записано в %s", t.Game, t.Key, value, t.Path)
	return nil
}

// restore — вернуть оригиналы из бэкапов (session_end, старт агента).
func (s *sensApplier) restore() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range sensTargets(s.cfg, s.getenv) {
		if restoreSensBackup(t.Path) {
			n++
		}
	}
	s.backed = map[string]bool{}
	return n
}

// restoreSensBackup — <path>.1448bak → path; true, если вернули.
func restoreSensBackup(path string) bool {
	bak := path + sensBackupSuffix
	data, err := os.ReadFile(bak)
	if err != nil {
		return false
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("сенса: откат %s не удался: %v", path, err)
		return false
	}
	_ = os.Remove(bak)
	log.Printf("сенса: %s возвращён из бэкапа", path)
	return true
}

// ── HTTP для студии сенсы и хук сессии ──

// POST /sens-apply {dpi, games} — «Применить в игре» из студии сенсы.
func (a *agent) handleSensApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "method"})
		return
	}
	if !a.cfg.Sens.Enabled {
		writeJSON(w, http.StatusConflict, map[string]any{"code": "sens_disabled",
			"message": "Подстановка сенсы на этом ПК выключена (agent.json → sens.enabled)"})
		return
	}
	var p sensProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || len(p.Games) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "bad_request", "message": "Нужны games {игра: сенса}"})
		return
	}
	res := a.sens.apply(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"applied": res.Applied, "skipped": res.Skipped, "dry_run": res.DryRun, "supported": sensGamesSupported,
	})
}

// sensOnSessionStart — сервер прислал профиль гостя в session_start.
func (a *agent) sensOnSessionStart(payload json.RawMessage) {
	if !a.cfg.Sens.Enabled {
		return
	}
	var p struct {
		Sens *sensProfile `json:"sens"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Sens == nil || len(p.Sens.Games) == 0 {
		return
	}
	res := a.sens.apply(*p.Sens)
	log.Printf("сенса на старте сессии: подставлено %v, пропущено %v (пробный режим: %v)", res.Applied, res.Skipped, res.DryRun)
}
