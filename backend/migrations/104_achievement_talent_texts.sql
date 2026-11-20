-- 104 — К12-и2 (KIOSK.md, этап VII, трек К): тексты ачивок и талантов для гостя по-польски и по-английски.
-- Список «RU → PL → EN» основатель принял 20.11 в 17:24: 1) русский без мужского рода — «да, это важно»;
-- 2) «Градация везения»: «×1.5 за уровень» → «+10% за уровень» во всех трёх языках — «да» (по коду вес Heavy+
-- растёт в 1 + 0.1 × уровень раз, ×1.5 — только на 5-м уровне); 3) PL и EN — «пока что да». И решение 13.11:
-- гостю «монетки», не «coins» (русские описания coin_mint и investor; EN coins не трогаем).
-- Колонки PL/EN — из 103 (К12-и1): пусто → гость видит русский. Тексты — из одного скрипта со списком основателю.
--
-- Русский меняется только там, где он ещё прежний (условие на старое значение): ручную правку не затираем.
-- Откат (down-файлов в репо нет — миграции однократны по журналу schema_migrations):
--   UPDATE achievements SET title_pl = '', title_en = '', description_pl = '', description_en = '';
--   UPDATE talent_definitions SET name_pl = '', name_en = '', description_pl = '', description_en = '';
--   и вернуть русский:
--   UPDATE achievements SET description = 'Забронировал ПК впервые' WHERE id = 'first_booking';
--   UPDATE achievements SET description = 'Провёл 1 час за компьютером' WHERE id = 'hour_1';
--   UPDATE achievements SET description = 'Провёл 10 часов за компьютером' WHERE id = 'hour_10';
--   UPDATE achievements SET description = 'Провёл 100 часов за компьютером' WHERE id = 'hour_100';
--   UPDATE achievements SET description = 'Провёл 1000 часов за компьютером' WHERE id = 'hour_1000';
--   UPDATE achievements SET description = 'Провёл 500 часов за компьютером' WHERE id = 'hour_500';
--   UPDATE achievements SET description = 'Привязал номер телефона' WHERE id = 'phone_verified';
--   UPDATE achievements SET description = 'Указал дату рождения — подарок не промахнётся' WHERE id = 'profile_birth';
--   UPDATE achievements SET description = 'Заполнил профиль целиком' WHERE id = 'profile_complete';
--   UPDATE achievements SET description = 'Указал Discord' WHERE id = 'profile_discord';
--   UPDATE achievements SET description = 'Выбрал любимые игры' WHERE id = 'profile_games';
--   UPDATE achievements SET title = 'Представился' WHERE id = 'profile_name';
--   UPDATE achievements SET description = 'Указал имя и фамилию' WHERE id = 'profile_name';
--   UPDATE achievements SET description = 'Рассказал, откуда узнал о клубе' WHERE id = 'profile_source';
--   UPDATE achievements SET description = 'Указал Telegram' WHERE id = 'profile_telegram';
--   UPDATE achievements SET description = 'Посетил клуб сегодня' WHERE id = 'daily_visit';
--   UPDATE achievements SET title = 'Подкрепился' WHERE id = 'snack_daily';
--   UPDATE achievements SET description = 'Заказал что-то на кухне сегодня' WHERE id = 'snack_daily';
--   UPDATE achievements SET description = 'Начал сессию ночью (22:00–07:59)' WHERE id = 'night_owl';
--   UPDATE achievements SET description = 'Выиграл 3 матча подряд за день' WHERE id = 'win_streak_3';
--   UPDATE achievements SET description = 'Посещал клуб 7 дней подряд' WHERE id = 'week_streak_7';
--   UPDATE achievements SET description = 'Посетил клуб 5 дней за неделю' WHERE id = 'week_5of7';
--   UPDATE talent_definitions SET description = '+5% coins при пополнении баланса' WHERE id = 'coin_mint';
--   UPDATE talent_definitions SET description = 'Coins не сгорают 2 месяца вместо 1' WHERE id = 'investor';
--   UPDATE talent_definitions SET description = 'Шанс Heavy+ кейса ×1.5 за уровень' WHERE id = 'luck_grade';
--
-- Идемпотентна: повтор ставит те же значения.

UPDATE achievements SET title_pl = 'Pierwsza rezerwacja', title_en = 'First booking', description_pl = 'Komputer zarezerwowany po raz pierwszy', description_en = 'Booked a PC for the first time' WHERE id = 'first_booking';
UPDATE achievements SET title_pl = 'Pierwsze doładowanie', title_en = 'First top-up', description_pl = 'Pierwsze doładowanie konta', description_en = 'First balance top-up' WHERE id = 'first_deposit';
UPDATE achievements SET title_pl = '1 godzina w klubie', title_en = '1 hour in the club', description_pl = 'Za tobą 1 godzina przy komputerze', description_en = 'Spent 1 hour at a PC' WHERE id = 'hour_1';
UPDATE achievements SET title_pl = '10 godzin', title_en = '10 hours', description_pl = 'Za tobą 10 godzin przy komputerze', description_en = 'Spent 10 hours at a PC' WHERE id = 'hour_10';
UPDATE achievements SET title_pl = 'Setka godzin', title_en = 'A hundred hours', description_pl = 'Za tobą 100 godzin przy komputerze', description_en = 'Spent 100 hours at a PC' WHERE id = 'hour_100';
UPDATE achievements SET title_pl = 'Tysiąc', title_en = 'One thousand', description_pl = 'Za tobą 1000 godzin przy komputerze', description_en = 'Spent 1000 hours at a PC' WHERE id = 'hour_1000';
UPDATE achievements SET title_pl = 'Pięćset', title_en = 'Five hundred', description_pl = 'Za tobą 500 godzin przy komputerze', description_en = 'Spent 500 hours at a PC' WHERE id = 'hour_500';
UPDATE achievements SET title_pl = 'Pierwsze zamówienie', title_en = 'First order', description_pl = 'Pierwsze zamówienie z kuchni', description_en = 'First order from the kitchen' WHERE id = 'first_kitchen';
UPDATE achievements SET title_pl = 'Witaj w klubie', title_en = 'Welcome', description_pl = 'Pierwsze logowanie', description_en = 'First sign-in' WHERE id = 'first_login';
UPDATE achievements SET title_pl = 'Weryfikacja', title_en = 'Verified', description_pl = 'Numer telefonu powiązany z kontem', description_en = 'Linked a phone number' WHERE id = 'phone_verified';
UPDATE achievements SET title_pl = 'Urodziny w bazie', title_en = 'Birthday on file', description_pl = 'Data urodzenia podana — prezent nie chybi', description_en = 'Added your birthday — no missed gifts' WHERE id = 'profile_birth';
UPDATE achievements SET title_pl = 'Profil kompletny', title_en = 'Profile complete', description_pl = 'Profil wypełniony w całości', description_en = 'Filled in the whole profile' WHERE id = 'profile_complete';
UPDATE achievements SET title_pl = 'Na Discordzie', title_en = 'On Discord', description_pl = 'Discord dodany do profilu', description_en = 'Added Discord to your profile' WHERE id = 'profile_discord';
UPDATE achievements SET title_pl = 'Mój zestaw', title_en = 'My lineup', description_pl = 'Ulubione gry wybrane', description_en = 'Picked your favorite games' WHERE id = 'profile_games';
UPDATE achievements SET title_pl = 'Poznajmy się', title_en = 'Nice to meet you', description_pl = 'Imię i nazwisko podane', description_en = 'Added your first and last name' WHERE id = 'profile_name';
UPDATE achievements SET title_pl = 'Poczta pantoflowa', title_en = 'Word of mouth', description_pl = 'Wiemy już, skąd znasz klub', description_en = 'Told us how you found the club' WHERE id = 'profile_source';
UPDATE achievements SET title_pl = 'Na Telegramie', title_en = 'On Telegram', description_pl = 'Telegram dodany do profilu', description_en = 'Added Telegram to your profile' WHERE id = 'profile_telegram';
UPDATE achievements SET title_pl = 'Na fali', title_en = 'In the flow', description_pl = '4 aktywne godziny w ciągu doby', description_en = '4 active hours in a day' WHERE id = 'flow_240';
UPDATE achievements SET title_pl = 'Maraton', title_en = 'Marathon', description_pl = '8 aktywnych godzin w ciągu doby', description_en = '8 active hours in a day' WHERE id = 'marathon_480';
UPDATE achievements SET title_pl = 'Rozgrzewka', title_en = 'Warm-up', description_pl = 'Godzina aktywnej gry w ciągu doby', description_en = 'An hour of active play in a day' WHERE id = 'warmup_60';
UPDATE achievements SET title_pl = 'Codzienna wizyta', title_en = 'Daily visit', description_pl = 'Dziś w klubie', description_en = 'Visited the club today' WHERE id = 'daily_visit';
UPDATE achievements SET title_pl = 'Przekąska', title_en = 'Snack time', description_pl = 'Zamówienie z kuchni dzisiaj', description_en = 'Ordered from the kitchen today' WHERE id = 'snack_daily';
UPDATE achievements SET title_pl = 'Nocna zmiana', title_en = 'Night shift', description_pl = 'Start sesji w nocy (22:00–07:59)', description_en = 'Started a session at night (22:00–07:59)' WHERE id = 'night_owl';
UPDATE achievements SET title_pl = '3 wygrane z rzędu', title_en = '3 wins in a row', description_pl = '3 wygrane mecze z rzędu w ciągu dnia', description_en = 'Won 3 matches in a row in one day' WHERE id = 'win_streak_3';
UPDATE achievements SET title_pl = 'Dziesiątka', title_en = 'Perfect ten', description_pl = '10 aktywnych godzin w tygodniu', description_en = '10 active hours in a week' WHERE id = 'week_10h';
UPDATE achievements SET title_pl = 'Tydzień bez przerwy', title_en = 'No days off', description_pl = '7 dni z rzędu w klubie', description_en = 'Visited the club 7 days in a row' WHERE id = 'week_streak_7';
UPDATE achievements SET title_pl = 'Pięć z siedmiu', title_en = 'Five out of seven', description_pl = '5 dni w klubie w ciągu tygodnia', description_en = 'Visited the club 5 days in a week' WHERE id = 'week_5of7';
UPDATE achievements SET title_pl = 'Setka', title_en = 'Century', description_pl = '100 aktywnych godzin w miesiącu', description_en = '100 active hours in a month' WHERE id = 'month_100h';
UPDATE achievements SET title_pl = 'Drugi dom', title_en = 'Club resident', description_pl = '20 wizyt w miesiącu', description_en = '20 visits in a month' WHERE id = 'month_regular';
UPDATE talent_definitions SET name_pl = 'Łowca skrzynek', name_en = 'Case hunter', description_pl = '+15% szansy na skrzynkę za poziom', description_en = '+15% case drop chance per level' WHERE id = 'case_hunter';
UPDATE talent_definitions SET name_pl = 'Podwójny drop', name_en = 'Double drop', description_pl = 'Szansa na 2 skrzynki naraz', description_en = 'Chance of 2 cases at once' WHERE id = 'double_drop';
UPDATE talent_definitions SET name_pl = 'Stopnie szczęścia', name_en = 'Luck grade', description_pl = '+10% szansy na skrzynkę Heavy+ za poziom', description_en = '+10% Heavy+ case chance per level' WHERE id = 'luck_grade';
UPDATE talent_definitions SET name_pl = 'Tryb nocny', name_en = 'Night mode', description_pl = '−5% taryfy nocnej za poziom', description_en = '−5% night rate per level' WHERE id = 'night_mode';
UPDATE talent_definitions SET name_pl = 'Priorytet rezerwacji', name_en = 'Priority booking', description_pl = 'Rezerwacja bez przedpłaty', description_en = 'Booking without prepayment' WHERE id = 'priority_booking';
UPDATE talent_definitions SET name_pl = 'Rozpęd XP', name_en = 'XP boost', description_pl = '+10% XP za każdą godzinę gry', description_en = '+10% XP on every hour played' WHERE id = 'xp_boost';
UPDATE talent_definitions SET name_pl = 'Mistrz cashbacku', name_en = 'Cashback master', description_pl = '+2% do bazowego cashbacku za poziom', description_en = '+2% to base cashback per level' WHERE id = 'cashback_master';
UPDATE talent_definitions SET name_pl = 'Mennica', name_en = 'Coin mint', description_pl = '+5% monetek przy doładowaniu konta', description_en = '+5% coins on balance top-up' WHERE id = 'coin_mint';
UPDATE talent_definitions SET name_pl = 'Inwestor', name_en = 'Investor', description_pl = 'Monetki żyją 2 miesiące zamiast 1', description_en = 'Coins last 2 months instead of 1' WHERE id = 'investor';

-- русский: без рода, «монетки», luck_grade
UPDATE achievements SET description = 'ПК забронирован впервые' WHERE id = 'first_booking' AND description = 'Забронировал ПК впервые';
UPDATE achievements SET description = 'Позади 1 час за компьютером' WHERE id = 'hour_1' AND description = 'Провёл 1 час за компьютером';
UPDATE achievements SET description = 'Позади 10 часов за компьютером' WHERE id = 'hour_10' AND description = 'Провёл 10 часов за компьютером';
UPDATE achievements SET description = 'Позади 100 часов за компьютером' WHERE id = 'hour_100' AND description = 'Провёл 100 часов за компьютером';
UPDATE achievements SET description = 'Позади 1000 часов за компьютером' WHERE id = 'hour_1000' AND description = 'Провёл 1000 часов за компьютером';
UPDATE achievements SET description = 'Позади 500 часов за компьютером' WHERE id = 'hour_500' AND description = 'Провёл 500 часов за компьютером';
UPDATE achievements SET description = 'Номер телефона привязан' WHERE id = 'phone_verified' AND description = 'Привязал номер телефона';
UPDATE achievements SET description = 'Дата рождения указана — подарок не промахнётся' WHERE id = 'profile_birth' AND description = 'Указал дату рождения — подарок не промахнётся';
UPDATE achievements SET description = 'Профиль заполнен целиком' WHERE id = 'profile_complete' AND description = 'Заполнил профиль целиком';
UPDATE achievements SET description = 'Discord указан в профиле' WHERE id = 'profile_discord' AND description = 'Указал Discord';
UPDATE achievements SET description = 'Любимые игры выбраны' WHERE id = 'profile_games' AND description = 'Выбрал любимые игры';
UPDATE achievements SET title = 'Знакомство' WHERE id = 'profile_name' AND title = 'Представился';
UPDATE achievements SET description = 'Имя и фамилия указаны' WHERE id = 'profile_name' AND description = 'Указал имя и фамилию';
UPDATE achievements SET description = 'Указано, откуда ты знаешь о клубе' WHERE id = 'profile_source' AND description = 'Рассказал, откуда узнал о клубе';
UPDATE achievements SET description = 'Telegram указан в профиле' WHERE id = 'profile_telegram' AND description = 'Указал Telegram';
UPDATE achievements SET description = 'Сегодня в клубе' WHERE id = 'daily_visit' AND description = 'Посетил клуб сегодня';
UPDATE achievements SET title = 'Перекус' WHERE id = 'snack_daily' AND title = 'Подкрепился';
UPDATE achievements SET description = 'Сегодня был заказ на кухне' WHERE id = 'snack_daily' AND description = 'Заказал что-то на кухне сегодня';
UPDATE achievements SET description = 'Старт сессии ночью (22:00–07:59)' WHERE id = 'night_owl' AND description = 'Начал сессию ночью (22:00–07:59)';
UPDATE achievements SET description = '3 выигранных матча подряд за день' WHERE id = 'win_streak_3' AND description = 'Выиграл 3 матча подряд за день';
UPDATE achievements SET description = '7 дней подряд в клубе' WHERE id = 'week_streak_7' AND description = 'Посещал клуб 7 дней подряд';
UPDATE achievements SET description = '5 дней в клубе за неделю' WHERE id = 'week_5of7' AND description = 'Посетил клуб 5 дней за неделю';
UPDATE talent_definitions SET description = '+5% монеток при пополнении баланса' WHERE id = 'coin_mint' AND description = '+5% coins при пополнении баланса';
UPDATE talent_definitions SET description = 'Монетки не сгорают 2 месяца вместо 1' WHERE id = 'investor' AND description = 'Coins не сгорают 2 месяца вместо 1';
UPDATE talent_definitions SET description = '+10% к шансу Heavy+ кейса за уровень' WHERE id = 'luck_grade' AND description = 'Шанс Heavy+ кейса ×1.5 за уровень';
