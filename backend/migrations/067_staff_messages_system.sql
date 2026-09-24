-- 067 — З1-и2 (OWNER.md, трек З; №59, решение Д4): служебные сообщения канала
-- персонала и «видит только владелец».
--
-- Сдача кассы писала в канал от имени админа одну строку «в ящике 80, должно
-- быть 100 — расхождение −20. Передал смену: …», и её читал любой админ у
-- стойки: чужая недостача — не его дело, а сумма в ящике — тем более. Теперь
-- деньги пишет система строкой «только владельцу», а заметку «передал смену» —
-- сам админ, и её видят все. На том же механизме встанут напоминания
-- бухгалтерии (З9-и5), приёмка (З10-и4) и алерты (З22).
--
-- kind — text (написал человек) | system (пишет сервер, автора нет);
-- visible_to — all | owner: админ строк owner не видит и не считает.

ALTER TABLE staff_messages ADD COLUMN IF NOT EXISTS kind VARCHAR(8) NOT NULL DEFAULT 'text'
    CHECK (kind IN ('text', 'system'));
ALTER TABLE staff_messages ADD COLUMN IF NOT EXISTS visible_to VARCHAR(8) NOT NULL DEFAULT 'all'
    CHECK (visible_to IN ('all', 'owner'));

-- у служебного сообщения автора нет; у обычного он обязателен, как и был
ALTER TABLE staff_messages ALTER COLUMN author_id DROP NOT NULL;
ALTER TABLE staff_messages DROP CONSTRAINT IF EXISTS staff_messages_author_chk;
ALTER TABLE staff_messages ADD CONSTRAINT staff_messages_author_chk CHECK (kind = 'system' OR author_id IS NOT NULL);

-- Сдачи кассы до З1-и2 — строки «деньги + заметка» от имени админа, видные всем.
-- Деньги в канале — только владельцу (Д4), поэтому такие строки прячем целиком:
-- заметка в них давняя, а сумма остаётся суммой.
UPDATE staff_messages SET visible_to = 'owner' WHERE visible_to = 'all' AND text LIKE '🧮 Сдал смену%';

COMMENT ON COLUMN staff_messages.kind       IS 'З1-и2: text — написал человек, system — пишет сервер (автора нет)';
COMMENT ON COLUMN staff_messages.visible_to IS 'З1-и2: all — все у стойки, owner — только владелец (деньги, люди)';
COMMENT ON COLUMN staff_messages.author_id  IS 'Е6; З1-и2: пусто у служебных сообщений';
