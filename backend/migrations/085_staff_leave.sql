-- 085 — З14-и2 (OWNER.md, трек З; №25 «а»; З-Р18): отложенное увольнение.
--
-- «Последний рабочий день 30.12» снимал роль в ту же секунду, а в архиве стояло «работал до 30.12». Теперь дата позже
-- сегодняшних клубных суток ложится в кадровую карточку: leave_on — последний рабочий день (доступ — до конца этих
-- клубных суток), leave_by — кто назначил (от его имени строка журнала о завершении), leave_reason — причина для
-- журнала. Роль остаётся admin; со следующего клубного дня adminMiddleware не пускает, фоновый прогон раз в 10 минут
-- завершает увольнение (роль, дата увольнения = leave_on, смены графика после неё, открытая смена) и обнуляет поля.
-- NULL — увольнение не назначено.
--
-- Идемпотентна: колонки — IF NOT EXISTS.

ALTER TABLE staff_profiles ADD COLUMN IF NOT EXISTS leave_on DATE;
ALTER TABLE staff_profiles ADD COLUMN IF NOT EXISTS leave_by UUID REFERENCES users(id);
ALTER TABLE staff_profiles ADD COLUMN IF NOT EXISTS leave_reason TEXT NOT NULL DEFAULT '';
