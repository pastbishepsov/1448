-- 055_notifications_clients.sql
-- К3-и2 (KIOSK.md, №16/№17): уведомления гостю доставляются каждому клиенту.
--
-- Было: GET /me/notifications отдавал непрочитанные (read_at IS NULL) и
-- помечал их прочитанными. Гость с открытым PWA на телефоне и киоском за ПК
-- получал тост только там, где поллинг успел первым: телефон «съедал»
-- balance_zero у киоска, киоск — kitchen_status у телефона.
--
-- Стало: две отметки — read_kiosk_at и read_app_at. Клиент зовёт ручку с
-- ?client=kiosk|app и получает своё непрочитанное; read_at остаётся
-- «прочитано вообще»: ставится клиентом без параметра (текущая PWA до её
-- итерации в треке М) или когда прочитали оба.
--
-- Идемпотентна: ADD COLUMN IF NOT EXISTS, UPDATE по условию, индексы IF NOT EXISTS.

ALTER TABLE notifications ADD COLUMN IF NOT EXISTS read_kiosk_at TIMESTAMPTZ;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS read_app_at   TIMESTAMPTZ;

COMMENT ON COLUMN notifications.read_kiosk_at IS 'К3-и2: показано на киоске (shell.html, ?client=kiosk)';
COMMENT ON COLUMN notifications.read_app_at   IS 'К3-и2: показано в PWA (app.html, ?client=app)';

-- Всё, что было прочитано до миграции, считаем прочитанным обоими: иначе
-- киоск при первом опросе вывалил бы гостю всю историю тостов.
UPDATE notifications
   SET read_kiosk_at = COALESCE(read_kiosk_at, read_at),
       read_app_at   = COALESCE(read_app_at, read_at)
 WHERE read_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_unread_kiosk
    ON notifications (user_id, created_at) WHERE read_kiosk_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_unread_app
    ON notifications (user_id, created_at) WHERE read_app_at IS NULL;
