-- 060_mousepad_cm.sql
-- С4-и3 (SENS.md, №13): ширина коврика клуба в сантиметрах — эталон длины
-- для замера DPI свайпом в студии сенсы (гость со своей мышью, DPI которой
-- не знает: DPI = счётчики мыши / см × 2,54 при raw input). 0 — не задана:
-- студия предлагает лист A4 (21 см) или свою длину.
INSERT INTO settings (key, value) VALUES ('mousepad_cm', '0')
ON CONFLICT (key) DO NOTHING;
