-- 102 — К11-и5 (KIOSK.md, этап VII, трек К; №9 осмотра 09.11, основатель: «окей»): ачивка «3 победы подряд» выключена.
--
-- win_streak_3 (условие win_streak, 005) сервер не считает — conditionValue в achievements.go: тип не поддержан, у гостя
-- прогресс всегда 0, получить её нельзя. Выключаем, а не удаляем: определение и выданные строки user_achievements на
-- месте (выданных нет — получить было нельзя). Выключенную ачивку не видит гость (GET /me/achievements берёт только
-- is_active) и не выдаёт сервер. Механика побед появится — включить обратно отдельной миграцией.
--
-- Откат (down-файлов в репо нет — миграции однократны по журналу schema_migrations):
--   UPDATE achievements SET is_active = TRUE WHERE id = 'win_streak_3';
--
-- Идемпотентна: повтор ничего не меняет.

UPDATE achievements SET is_active = FALSE WHERE id = 'win_streak_3' AND is_active;
