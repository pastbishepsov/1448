"""Общие настройки живых проверок (e2e_*.py). Всё — через переменные окружения,
по умолчанию — локальный стенд: сервер на :8080, Postgres db1448 на localhost."""
import os, tempfile, urllib.parse

API = os.environ.get("E2E_API", "http://127.0.0.1:8080/api/v1").rstrip("/")
BASE = API[: -len("/api/v1")] if API.endswith("/api/v1") else API
_u = urllib.parse.urlparse(BASE)
HOST, PORT = _u.hostname or "127.0.0.1", _u.port or (443 if _u.scheme == "https" else 80)
DB = os.environ.get("E2E_DB", "postgresql://u1448:p1448@127.0.0.1:5432/db1448?sslmode=disable")
SRV_LOG = os.environ.get("E2E_SRV_LOG", "")  # лог сервера (проверка «токен не светится в логе»); пусто — пропуск
HERE = os.path.dirname(os.path.abspath(__file__))
MOCK_AGENT = os.path.join(HERE, "mock_agent.js")
LOG_DIR = os.environ.get("E2E_OUT", tempfile.gettempdir())
os.makedirs(LOG_DIR, exist_ok=True)
def agent_log(name): return os.path.join(LOG_DIR, f"1448_agent_{name}.log")
