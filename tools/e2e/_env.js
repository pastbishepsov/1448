// Общие настройки браузерных проверок (pw_*.js, smoke_*.js). Всё — через переменные окружения,
// по умолчанию — локальный стенд: сервер на :8080, Postgres db1448 на localhost.
const path = require('path'), os = require('os'), fs = require('fs');
const API = (process.env.E2E_API || 'http://127.0.0.1:8080/api/v1').replace(/\/$/, '');
const BASE = API.endsWith('/api/v1') ? API.slice(0, -'/api/v1'.length) : API;
const DB = process.env.E2E_DB || 'postgresql://u1448:p1448@127.0.0.1:5432/db1448?sslmode=disable';
const CHROME = process.env.E2E_CHROME || undefined; // путь к Chrome/Chromium; пусто — браузер playwright (npx playwright install chromium)
const OUT = process.env.E2E_OUT || os.tmpdir();      // скриншоты и логи мок-агентов
fs.mkdirSync(OUT, { recursive: true });
const MOCK_AGENT = path.join(__dirname, 'mock_agent.js');
const agentLog = name => path.join(OUT, `1448_agent_${name}.log`);
const WEB_DIR = process.env.E2E_WEB_DIR || path.join(__dirname, '..', '..', 'web'); // разметка admin.html для проверок по исходнику
module.exports = { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR };
