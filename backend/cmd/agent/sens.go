package main

// К7 «Сенса в игру» (KIOSK.md, решение основателя Р9) + С2 «Сенса на ПК»
// (SENS.md, Р1–Р2, Т2–Т5): профиль сенсы гостя (DPI + сенса по играм, GET
// /me/sensitivity) подставляется в игры НА ЭТОМ ПК — гость садится, и CS2
// стартует с его сенсой.
//
// Как это работает:
//   - блок "sens" в agent.json: enabled (выкл по умолчанию), dry_run (ВКЛ по
//     умолчанию — только лог, файлы не трогаем; боевой режим владелец
//     включает после сухого прогона на пилотном ПК), cs2_mode ("exec" по
//     умолчанию | "vcfg"), cs2_dir / steam_dir / cs2_cfg / apex_cfg — где что
//     лежит (пусто — ищем сами);
//   - кто просит: сервер — в session_start (и при пересадке) приходит sens
//     {dpi, games}, уже пересчитанный под DPI мыши клуба; сервер же шлёт
//     sens_apply, когда гость сохранил профиль во время сессии («применить
//     сейчас» идёт через сервер, а не браузером в 127.0.0.1 — С2-и4, Т5);
//     POST /sens-apply остаётся для curl-проверки на живом ПК;
//   - CS2, режим exec (С2-и3, Т2): пишем game/csgo/cfg/1448.cfg со строкой
//     sensitivity "…", а запуск CS2 идёт с +exec 1448 (launch.go,
//     withSensArgs). Файл живёт ВСЕГДА: с сенсой гостя после session_start
//     или sens_apply, пустой (один комментарий) после session_end. Работает
//     под любым Steam-аккаунтом и не зависит от момента входа в Steam — на
//     session_start гость ещё не в Steam, и «самый свежий userdata» К7
//     попадал в чужую папку (Р1);
//   - CS2, режим vcfg (К7, запасной): "sensitivity" в
//     cs2_user_convars_0_slot0.vcfg самого свежего аккаунта;
//   - Apex — mouse_sensitivity в settings.cfg (файл на Windows-пользователя,
//     не на аккаунт); Valorant НЕ умеем — настройки в облаке Riot (об этом
//     честно говорит студия); OW2/CoD/Quake/Deadlock — нет локального
//     конфига, который стоит трогать;
//   - откат (Т3): на session_start с cs2 в профиле ВСЕ существующие vcfg
//     (userdata/*/730) копируются в <файл>.1448bak — CS2 при выходе пишет
//     сенсу в vcfg текущего аккаунта, и клубный аккаунт иначе унаследовал бы
//     её; на session_end бэкапы возвращаются, 1448.cfg пустеет. Аккаунт
//     гостя, появившийся во время сессии, в бэкапах не был — его не трогаем:
//     сенса, которую он сам сохранил у нас, остаётся его. На старте агента
//     всё то же (агент упал посреди сессии);
//   - /ping отдаёт sens_current — текущую сенсу и разрешение из игры на
//     этом ПК (аккаунт — из config/loginusers.vdf, MostRecent), студия
//     показывает «в CS2 на этом ПК сейчас N» и подставляет aspect в
//     калибратор (С2-и2).

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	CS2Cfg   string `json:"cs2_cfg"`   // явный путь к cs2_user_convars_0_slot0.vcfg (режим vcfg и чтение sens_current)
	ApexCfg  string `json:"apex_cfg"`  // явный путь к settings.cfg Apex
	CS2Mode  string `json:"cs2_mode"`  // С2-и3: "exec" (по умолчанию) | "vcfg"
	CS2Dir   string `json:"cs2_dir"`   // С2-и3: корень CS2 (…\steamapps\common\Counter-Strike Global Offensive); пусто — ищем по Steam и его библиотекам
}

func (c SensConfig) dryRun() bool { return c.DryRun == nil || *c.DryRun }

// cs2Mode — "vcfg" только по явной просьбе; всё остальное — exec.
func (c SensConfig) cs2Mode() string {
	if strings.EqualFold(strings.TrimSpace(c.CS2Mode), "vcfg") {
		return "vcfg"
	}
	return "exec"
}

const (
	sensBackupSuffix = ".1448bak"
	sensExecName     = "1448.cfg" // game/csgo/cfg/1448.cfg → +exec 1448
	steamID64Base    = uint64(76561197960265728)
	cs2Folder        = "Counter-Strike Global Offensive"
)

// sensExecHeader — первая строка 1448.cfg (без перевода строки).
const sensExecHeader = "// 14:48 — сенса гостя. Файл переписывает агент клуба; править бессмысленно."

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
	sensKindExec                     // С2-и3: наш файл game/csgo/cfg/1448.cfg, исполняется через +exec 1448
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

// sensLaunchArgs — С2-и3: что дописать в запуск игры, чтобы она прочитала
// нашу сенсу (exec-режим). Только CS2: у Apex файл читается сам.
var sensLaunchArgs = map[string]string{"cs2": "+exec 1448"}

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
	vcfgKeyRe   = regexp.MustCompile(`^(\s*)"([^"]+)"(\s+)"([^"]*)"(\s*)$`)
	apexLineRe  = regexp.MustCompile(`^(\s*)([A-Za-z0-9_]+)(\s+)"([^"]*)"(\s*)$`)
	steamRunRe  = regexp.MustCompile(`^steam://(rungameid|run)/(\d+)/?$`)
	vdfPathRe   = regexp.MustCompile(`(?i)"path"\s+"([^"]+)"`)
	vdfID64Re   = regexp.MustCompile(`^\s*"(7656\d{13})"\s*$`)
	vdfRecentRe = regexp.MustCompile(`(?i)^\s*"MostRecent"\s+"1"\s*$`)
	cs2VideoRe  = regexp.MustCompile(`(?i)"setting\.(defaultres|defaultresheight|aspectratiomode)"\s+"(\d+)"`)
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

// vcfgGetConvar — С2-и2: значение "key" внутри блока "convars" (зеркало
// vcfgSetConvar, только чтение).
func vcfgGetConvar(content, key string) (string, bool) {
	inConvars, depth := false, 0
	for _, raw := range splitLines(content) {
		l := strings.TrimRight(raw, "\r\n")
		t := strings.TrimSpace(l)
		if !inConvars {
			inConvars = strings.EqualFold(t, `"convars"`)
			continue
		}
		if t == "{" {
			depth++
			continue
		}
		if t == "}" {
			depth--
			if depth == 0 {
				return "", false
			}
			continue
		}
		if depth != 1 {
			continue
		}
		if m := vcfgKeyRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[2], key) {
			return m[4], true
		}
	}
	return "", false
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

// apexCfgGet — С2-и2: значение key из settings.cfg Apex.
func apexCfgGet(content, key string) (string, bool) {
	for _, raw := range splitLines(content) {
		if m := apexLineRe.FindStringSubmatch(strings.TrimRight(raw, "\r\n")); m != nil && strings.EqualFold(m[2], key) {
			return m[4], true
		}
	}
	return "", false
}

// execCfgContent — С2-и3: содержимое 1448.cfg. value пустой — файл «пустой»
// (один комментарий): ничего не менять.
func execCfgContent(value string) string {
	if value == "" {
		return sensExecHeader + "\n"
	}
	return sensExecHeader + "\n" + `sensitivity "` + value + `"` + "\n"
}

// withSensArgs — С2-и3: запуск CS2 с +exec 1448, чтобы игра прочитала наш
// файл. steam://rungameid/730 и steam://run/730 → steam://run/730//+exec%201448/
// (аргументы после // Steam передаёт игре; пробел кодируем), exe-цель —
// аргументы командной строки. Другие игры и выключенный exec-режим — как есть.
// Чистая функция (тестируется).
func withSensArgs(appID, target string, args []string, enabled bool) (string, []string) {
	extra, ok := sensLaunchArgs[appID]
	if !enabled || !ok {
		return target, args
	}
	t := strings.TrimSpace(target)
	if m := steamRunRe.FindStringSubmatch(strings.ToLower(t)); m != nil {
		return "steam://run/" + m[2] + "//" + url.PathEscape(extra) + "/", args
	}
	if strings.Contains(strings.ToLower(t), "://") { // другой протокол — не знаем, как передать
		return target, args
	}
	out := append([]string{}, args...)
	return target, append(out, strings.Fields(extra)...)
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

// allCS2Vcfg — С2-и3 (Т3): ВСЕ vcfg CS2 на этом ПК (все аккаунты Steam) —
// их бэкапим на старте сессии и возвращаем на конце.
func allCS2Vcfg(cfg SensConfig, getenv func(string) string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(cfg.CS2Cfg)
	for _, dir := range steamDirs(cfg, getenv) {
		matches, _ := filepath.Glob(filepath.Join(dir, "userdata", "*", "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg"))
		for _, m := range matches {
			add(m)
		}
	}
	sort.Strings(out)
	return out
}

// parseLibraryFolders — пути библиотек Steam из steamapps/libraryfolders.vdf
// ("path" "D:\\Games\\SteamLibrary" — обратные слэши удвоены).
func parseLibraryFolders(content string) []string {
	var out []string
	for _, m := range vdfPathRe.FindAllStringSubmatch(content, -1) {
		p := strings.ReplaceAll(m[1], `\\`, `\`)
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// findCS2Dir — корень CS2: из конфига; иначе <steam>/steamapps/common/… у
// каждого корня Steam и у каждой библиотеки из libraryfolders.vdf.
func findCS2Dir(cfg SensConfig, getenv func(string) string) string {
	if cfg.CS2Dir != "" {
		return cfg.CS2Dir
	}
	isDir := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.IsDir()
	}
	for _, dir := range steamDirs(cfg, getenv) {
		if p := filepath.Join(dir, "steamapps", "common", cs2Folder); isDir(p) {
			return p
		}
		if b, err := os.ReadFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, lib := range parseLibraryFolders(string(b)) {
				if p := filepath.Join(lib, "steamapps", "common", cs2Folder); isDir(p) {
					return p
				}
			}
		}
	}
	return ""
}

// cs2ExecPath — game/csgo/cfg/1448.cfg внутри корня CS2.
func cs2ExecPath(cs2Dir string) string {
	return filepath.Join(cs2Dir, "game", "csgo", "cfg", sensExecName)
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
	if cfg.cs2Mode() == "exec" {
		if d := findCS2Dir(cfg, getenv); d != "" {
			out["cs2"] = sensTarget{Game: "cs2", Path: cs2ExecPath(d), Key: "sensitivity", Kind: sensKindExec}
		}
	} else if p := findCS2Cfg(cfg, getenv); p != "" {
		out["cs2"] = sensTarget{Game: "cs2", Path: p, Key: "sensitivity", Kind: sensKindVCFG}
	}
	if p := findApexCfg(cfg, getenv); p != "" {
		out["apex"] = sensTarget{Game: "apex", Path: p, Key: "mouse_sensitivity", Kind: sensKindApex}
	}
	return out
}

// ── текущая сенса с ПК (С2-и2) ──

// parseLoginUsers — SteamID64 аккаунта с "MostRecent" "1" из config/loginusers.vdf.
func parseLoginUsers(content string) string {
	cur := ""
	for _, raw := range splitLines(content) {
		l := strings.TrimRight(raw, "\r\n")
		if m := vdfID64Re.FindStringSubmatch(l); m != nil {
			cur = m[1]
			continue
		}
		if vdfRecentRe.MatchString(l) && cur != "" {
			return cur
		}
	}
	return ""
}

// steamAccountID — папка userdata/<id> из SteamID64.
func steamAccountID(id64 string) string {
	v, err := strconv.ParseUint(id64, 10, 64)
	if err != nil || v <= steamID64Base {
		return ""
	}
	return strconv.FormatUint(v-steamID64Base, 10)
}

// activeCS2Vcfg — vcfg аккаунта, под которым сейчас Steam (loginusers.vdf);
// нет данных — самый свежий, как в К7.
func activeCS2Vcfg(cfg SensConfig, getenv func(string) string) string {
	if cfg.CS2Cfg != "" {
		return cfg.CS2Cfg
	}
	for _, dir := range steamDirs(cfg, getenv) {
		b, err := os.ReadFile(filepath.Join(dir, "config", "loginusers.vdf"))
		if err != nil {
			continue
		}
		if acc := steamAccountID(parseLoginUsers(string(b))); acc != "" {
			p := filepath.Join(dir, "userdata", acc, "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return findCS2Cfg(cfg, getenv)
}

// parseCS2Video — разрешение и режим соотношения из cs2_video.txt
// (setting.defaultres / defaultresheight / aspectratiomode).
func parseCS2Video(content string) (w, h, mode int) {
	for _, m := range cs2VideoRe.FindAllStringSubmatch(content, -1) {
		v, _ := strconv.Atoi(m[2])
		switch strings.ToLower(m[1]) {
		case "defaultres":
			w = v
		case "defaultresheight":
			h = v
		case "aspectratiomode":
			mode = v
		}
	}
	return
}

// aspectLabel — 1280×1024 → "5:4" и т.п. (для студии/калибратора).
func aspectLabel(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	r := float64(w) / float64(h)
	switch {
	case r < 1.29:
		return "5:4"
	case r < 1.45:
		return "4:3"
	case r < 1.68:
		return "16:10"
	default:
		return "16:9"
	}
}

// sensCurrent — что стоит в играх на этом ПК прямо сейчас: /ping → sens_current.
// Нет файла — игры в ответе нет.
func sensCurrent(cfg SensConfig, getenv func(string) string) map[string]any {
	out := map[string]any{}
	if p := activeCS2Vcfg(cfg, getenv); p != "" {
		cs2 := map[string]any{}
		if b, err := os.ReadFile(p); err == nil {
			if v, ok := vcfgGetConvar(string(b), "sensitivity"); ok {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
					cs2["sens"] = f
				}
			}
		}
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "cs2_video.txt")); err == nil {
			if w, h, mode := parseCS2Video(string(b)); w > 0 && h > 0 {
				cs2["res_w"], cs2["res_h"], cs2["aspect"], cs2["aspect_mode"] = w, h, aspectLabel(w, h), mode
			}
		}
		if len(cs2) > 0 {
			out["cs2"] = cs2
		}
	}
	if p := findApexCfg(cfg, getenv); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if v, ok := apexCfgGet(string(b), "mouse_sensitivity"); ok {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
					out["apex"] = map[string]any{"sens": f}
				}
			}
		}
	}
	return out
}

// ── применение и откат ──

// sensResult — что вышло (ответ студии/сервера и лог).
type sensResult struct {
	Applied []string          `json:"applied"` // игры, куда записали (в dry_run — куда записали БЫ)
	Skipped map[string]string `json:"skipped"` // игра → причина: cloud | unsupported | no_config | disabled | error
	DryRun  bool              `json:"dry_run"`
	Running []string          `json:"running,omitempty"` // С2-и4: игра уже запущена — подхватит при перезапуске (CS2: exec 1448 в консоли)
}

// sensApplier — состояние подстановки на этом ПК: какие файлы подменены.
type sensApplier struct {
	cfg        SensConfig
	getenv     func(string) string
	mu         sync.Mutex
	backed     map[string]bool // путь → бэкап уже сделан в этой сессии
	vcfgBacked bool            // С2-и3 (Т3): все vcfg этой сессии уже отложены
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
		if !s.cfg.Enabled {
			res.Skipped[g] = "disabled"
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
		if t.Kind != sensKindExec { // exec-файл наш и может ещё не существовать
			if _, err := os.Stat(t.Path); err != nil { // игра не стояла / конфиг ещё не создан
				res.Skipped[g] = "no_config"
				continue
			}
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
	if t.Kind == sensKindExec {
		return s.applyExec(t, value)
	}
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

// applyExec — С2-и3: 1448.cfg с сенсой + бэкап всех vcfg (Т3) перед первой
// записью в сессии.
func (s *sensApplier) applyExec(t sensTarget, value string) error {
	if s.cfg.dryRun() {
		log.Printf("сенса [пробный режим]: %s — записал бы sensitivity %s в %s (+ бэкап %d vcfg)", t.Game, value, t.Path, len(allCS2Vcfg(s.cfg, s.getenv)))
		return nil
	}
	if !s.vcfgBacked {
		for _, p := range allCS2Vcfg(s.cfg, s.getenv) {
			bak := p + sensBackupSuffix
			if _, err := os.Stat(bak); os.IsNotExist(err) {
				data, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				if err := os.WriteFile(bak, data, 0o644); err != nil {
					return fmt.Errorf("бэкап vcfg не записался (%s): %w", p, err)
				}
				log.Printf("сенса: отложен %s", p)
			}
		}
		s.vcfgBacked = true
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return fmt.Errorf("папка cfg не создаётся: %w", err)
	}
	if err := os.WriteFile(t.Path, []byte(execCfgContent(value)), 0o644); err != nil {
		return fmt.Errorf("1448.cfg не записался: %w", err)
	}
	log.Printf("сенса: %s — sensitivity %s записано в %s (игра читает через +exec 1448)", t.Game, value, t.Path)
	return nil
}

// restore — вернуть оригиналы из бэкапов (session_end, старт агента): все vcfg
// CS2 (Т3), Apex; 1448.cfg — пустой. Возвращает число возвращённых файлов.
func (s *sensApplier) restore() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	paths := allCS2Vcfg(s.cfg, s.getenv)
	for _, t := range sensTargets(s.cfg, s.getenv) {
		switch t.Kind {
		case sensKindExec:
			if !s.cfg.dryRun() && s.cfg.Enabled {
				if b, err := os.ReadFile(t.Path); err != nil || string(b) != execCfgContent("") {
					if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err == nil {
						if err := os.WriteFile(t.Path, []byte(execCfgContent("")), 0o644); err != nil {
							log.Printf("сенса: 1448.cfg не опустошился: %v", err)
						}
					}
				}
			}
		default:
			if !slices.Contains(paths, t.Path) {
				paths = append(paths, t.Path)
			}
		}
	}
	for _, p := range paths {
		if restoreSensBackup(p) {
			n++
		}
	}
	s.backed = map[string]bool{}
	s.vcfgBacked = false
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

// ── HTTP для студии сенсы, WS-команда сервера и хук сессии ──

// applyWithRunning — apply + какие из подставленных игр сейчас запущены
// (подхватят сенсу при перезапуске; CS2 — ещё и по exec 1448 в консоли).
func (a *agent) applyWithRunning(p sensProfile) sensResult {
	res := a.sens.apply(p)
	for _, g := range res.Applied {
		if a.track != nil {
			if _, ok := a.track.get(g); ok {
				res.Running = append(res.Running, g)
			}
		}
	}
	return res
}

// POST /sens-apply {dpi, games} — curl-проверка на живом ПК (раньше — кнопка
// «Применить в игре» студии; с С2-и4 студия идёт через сервер).
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
	res := a.applyWithRunning(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"applied": res.Applied, "skipped": res.Skipped, "dry_run": res.DryRun, "running": res.Running,
		"supported": sensGamesSupported, "cs2_mode": a.cfg.Sens.cs2Mode(),
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

// sensOnApply — С2-и4: сервер прислал sens_apply (гость сохранил профиль во
// время сессии) — применяем и отвечаем sens_result, сервер превращает его в
// уведомление гостю.
func (a *agent) sensOnApply(payload json.RawMessage) {
	var p sensProfile
	if json.Unmarshal(payload, &p) != nil || len(p.Games) == 0 {
		log.Printf("sens_apply без games — игнорирую: %s", string(payload))
		return
	}
	res := a.applyWithRunning(p)
	log.Printf("сенса по команде сервера: подставлено %v, пропущено %v, запущено %v (пробный режим: %v)", res.Applied, res.Skipped, res.Running, res.DryRun)
	body, _ := json.Marshal(map[string]any{
		"applied": res.Applied, "skipped": res.Skipped, "dry_run": res.DryRun, "running": res.Running,
	})
	go func() { _ = a.wsSend(wsMessage{Type: "sens_result", Payload: body}) }()
}
