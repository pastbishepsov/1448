-- 021: MAC-адрес ПК для Wake-on-LAN (спринт Б8 админ-трека, ADMIN.md).
-- Вводит owner в редакторе зала. NULL — включение по сети недоступно.
-- Magic-пакет шлёт живой агент-сосед в LAN клуба (бэкенд в докере
-- до LAN-broadcast не достаёт).

ALTER TABLE computers ADD COLUMN IF NOT EXISTS mac VARCHAR(17);
