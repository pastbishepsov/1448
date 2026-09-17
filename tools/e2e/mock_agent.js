// Мок агента ПК для стенда: подключается к /ws/shell как computer_id и пишет всё,
// что прислал сервер, строками JSON в лог. Запуск: node mock_agent.js <computer_id> <logfile>
// (AGENT_TOKEN на стенде не задан — проверка токена выключена).
const [,, computerId, logFile] = process.argv;
const fs = require('fs');
const log = (o) => fs.appendFileSync(logFile, JSON.stringify(Object.assign({ t: new Date().toISOString() }, o)) + '\n');
const WS_BASE = (process.env.E2E_API || 'http://127.0.0.1:8080/api/v1').replace(/\/$/, '').replace(/^http/, 'ws'); // тот же стенд, что у тестов
const ws = new WebSocket(WS_BASE + '/ws/shell?computer_id=' + encodeURIComponent(computerId));
ws.onopen = () => log({ ev: 'open' });
ws.onmessage = (e) => { let m = null; try { m = JSON.parse(e.data); } catch (_) { m = { raw: String(e.data) }; } log({ ev: 'msg', msg: m }); };
ws.onclose = (e) => { log({ ev: 'close', code: e.code }); process.exit(0); };
ws.onerror = () => log({ ev: 'error' });
setInterval(() => { if (ws.readyState === 1) ws.send(JSON.stringify({ type: 'session_tick', payload: { idle_sec: 0 } })); }, 10000);
