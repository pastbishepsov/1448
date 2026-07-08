-- 015_round_economy.sql
-- Правило цифр (DESIGN.md): всё, что видит игрок, кратно 5.
-- Новые начисления округляет сервер (RoundToStep); здесь разово выравниваем
-- накопленное: XP и баланс монет (сдвиг ≤ 2 монет/XP — приемлемо до пилота).
-- Историю сессий (xp_earned/coins_earned) не трогаем — это честный лог прошлого.

UPDATE users SET
    xp_current    = (xp_current + 2) / 5 * 5,
    xp_total      = (xp_total + 2) / 5 * 5,
    coins_balance = (coins_balance + 2) / 5 * 5
WHERE xp_current % 5 <> 0 OR xp_total % 5 <> 0 OR coins_balance % 5 <> 0;
