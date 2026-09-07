package main

// К4-и3 (KIOSK.md): токен агента. Закрывает известную дыру 26.08 — /ws/shell
// принимал любого, кто знает UUID компьютера (он есть в публичном
// /clubs/:id/computers), и такой «агент» получал команды ПК и мог слать
// session_tick с чужим idle_sec; а публичный /catalog отдавал пути на диске,
// аргументы запуска и корень игр клуба любому, кто открыл адрес сервера.
//
// Секрет один на клуб: AGENT_TOKEN на сервере и тот же токен в agent.json
// (Go-агент), appsettings.json (C#-сервис) и shell.json (C#-киоск: он сам
// тянет каталог для allowlist). Передаётся заголовком X-Agent-Token (или
// Authorization: Bearer); для WS-клиентов, которым заголовок неудобен, —
// ?token= в URL. Пустой AGENT_TOKEN = как раньше (всё открыто), чтобы клуб не
// остался без агентов в момент обновления сервера: сначала раздать токен по
// ПК, потом задать на сервере.

import (
	"crypto/subtle"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var agentToken = "" // AGENT_TOKEN; пусто = проверка выключена

func loadAgentToken() {
	agentToken = strings.TrimSpace(os.Getenv("AGENT_TOKEN"))
	if agentToken == "" {
		log.Println("⚠ AGENT_TOKEN не задан: /ws/shell и полный /catalog открыты всем — задайте токен на сервере и в конфигах агентов (К4-и3)")
		return
	}
	log.Println("Токен агента задан: /ws/shell и /catalog?full=1 только с ним")
}

// presentedAgentToken — токен из запроса: заголовок X-Agent-Token,
// Authorization: Bearer или ?token=.
func presentedAgentToken(c *gin.Context) string {
	if t := strings.TrimSpace(c.GetHeader("X-Agent-Token")); t != "" {
		return t
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return strings.TrimSpace(c.Query("token"))
}

// agentTokenMatches — чистая проверка (тест): пустой серверный токен =
// открыто; иначе сравнение постоянного времени.
func agentTokenMatches(server, presented string) bool {
	if server == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(server), []byte(presented)) == 1
}

// agentAuthorized — прошёл ли запрос проверку токена агента.
func agentAuthorized(c *gin.Context) bool {
	return agentTokenMatches(agentToken, presentedAgentToken(c))
}
