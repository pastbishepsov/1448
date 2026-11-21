-- 105 — К12-и5 (KIOSK.md, этап VII, трек К): открытие тайного седьмого ранга по буквам — один раз на гостя.
-- Основатель 21.11 в 12:13: «он видит эту анимацию один раз, когда ее получает. один раз за компьютером и
-- соответственно получения - на телефоне»; в 12:43 — «да»: сервер помнит отдельно киоск и PWA.
-- NULL — ещё не видел; время — когда клиент показал открытие (POST /me/rank-reveal?client=kiosk|app).
ALTER TABLE users ADD COLUMN IF NOT EXISTS rank_reveal_kiosk_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS rank_reveal_app_at TIMESTAMPTZ;
