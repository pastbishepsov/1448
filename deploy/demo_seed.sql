-- demo_seed.sql — демо-учётки для ОБЛАЧНОГО демо (попадает только в
-- Dockerfile.railway как migrations/901_demo_seed.sql; в боевых установках
-- этого файла нет). Идемпотентно: повторный старт ничего не ломает.
--
-- Пароли (только для демо-стенда, в клубе не использовать):
--   owner1448 / Klub1448!    — роль owner, «кабинет владельца»
--   admin1448 / Admin1448!   — роль admin, стойка
--   neo, sova, pixel / Gracz1448! — гости с прогрессом

INSERT INTO users (id, nickname, email, password_hash, role, level,
                   xp_current, xp_total, coins_balance, wallet_grosz,
                   first_name, last_name, avatar_id)
VALUES
  ('00000000-0000-0000-0000-00000000d001', 'owner1448', 'owner@1448.demo',
   '$2a$10$aiJFCIRYCro.QD7CqwEazO.nlVrwFxMtGMp6955aGQEY1B7RgvtPS',
   'owner', 1, 0, 0, 0, 0, 'Пан', 'Владелец', 1),
  ('00000000-0000-0000-0000-00000000d002', 'admin1448', 'admin@1448.demo',
   '$2a$10$Jb79fqPbSYUtkJSwLcLY2.abpeW9LfQmuEHkifmYL5rx77R9KgbFO',
   'admin', 1, 0, 0, 0, 0, 'Ася', 'Администратор', 2),
  ('00000000-0000-0000-0000-00000000d003', 'neo', 'neo@1448.demo',
   '$2a$10$DtyOSVeFr/ecOGYIIFzede3b9eC4nT4zPENyIA0C0J4CtppBzuJQK',
   'player', 7, 350, 21350, 940, 4500, 'Кирилл', 'Соколов', 3),
  ('00000000-0000-0000-0000-00000000d004', 'sova', 'sova@1448.demo',
   '$2a$10$DtyOSVeFr/ecOGYIIFzede3b9eC4nT4zPENyIA0C0J4CtppBzuJQK',
   'player', 4, 120, 6120, 260, 1500, 'Оля', 'Новак', 4),
  ('00000000-0000-0000-0000-00000000d005', 'pixel', 'pixel@1448.demo',
   '$2a$10$DtyOSVeFr/ecOGYIIFzede3b9eC4nT4zPENyIA0C0J4CtppBzuJQK',
   'player', 2, 40, 1040, 55, 0, 'Макс', 'Ковальчик', 5)
ON CONFLICT DO NOTHING;
