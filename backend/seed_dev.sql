-- seed_dev.sql — демо-данные для локальной разработки.
-- НЕ запускается автоматически (не в migrations/). Применяется вручную:
--   docker compose exec -T postgres psql -U 1448_user -d 1448_db < backend/seed_dev.sql
-- Идемпотентно: повторный запуск ничего не сломает.

-- Один демонстрационный клуб (Варшава, PLN, базовый тариф 23 zł/час)
INSERT INTO clubs (id, name, address, base_rate_pln, rtp_modifier, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    '14:48 Demo Club',
    'Warszawa, ul. Demo 1',
    23.00, 1.00, TRUE
)
ON CONFLICT (id) DO NOTHING;

-- Несколько компьютеров в этом клубе
INSERT INTO computers (id, club_id, name, zone, status)
VALUES
    ('00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-000000000001', 'PC-01',  'Standard', 'available'),
    ('00000000-0000-0000-0000-0000000000a2', '00000000-0000-0000-0000-000000000001', 'PC-02',  'Standard', 'available'),
    ('00000000-0000-0000-0000-0000000000a3', '00000000-0000-0000-0000-000000000001', 'VIP-01', 'VIP',      'available')
ON CONFLICT (id) DO NOTHING;
