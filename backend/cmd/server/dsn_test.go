package main

import (
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// З2-и1 (№4): пояс клуба в строке подключения — и в URL, и в key=value; уже
// заданный пояс не трогаем. Строку читают двое: pgx (разбирает URL и
// раскодирует) и gorm — тот берёт значение timezone= из строки сырым и зовёт
// time.LoadLocation, поэтому «Europe%2FWarsaw» уронил бы подключение.
func TestWithClubTimeZone(t *testing.T) {
	raw := regexp.MustCompile("(time_zone|TimeZone|timezone)=(.*?)($|&| )") // как в gorm.io/driver/postgres
	const tz = "Europe/Warsaw"
	cases := []struct {
		name, dsn, want string // want — пояс сессии после разбора драйвером
	}{
		{"URL без параметров (Railway)", "postgresql://u:p@db.internal:5432/railway", tz},
		{"URL с sslmode (клуб, стенд)", "postgres://u:p@postgres:5432/1448_db?sslmode=disable", tz},
		{"URL с «?» в конце", "postgres://u:p@localhost/db?", tz},
		{"URL — пояс уже задан, не трогаем", "postgres://u:p@localhost/db?sslmode=disable&timezone=UTC", "UTC"},
		{"key=value без пояса", "host=localhost port=5432 user=u password=p dbname=db sslmode=disable", tz},
		{"key=value с TimeZone", "host=localhost user=u password=p dbname=db TimeZone=Asia/Tokyo", "Asia/Tokyo"},
	}
	for _, c := range cases {
		got := withClubTimeZone(c.dsn, tz)
		cfg, err := pgx.ParseConfig(got)
		if err != nil {
			t.Fatalf("%s: драйвер не разобрал %q: %v", c.name, got, err)
		}
		zone := cfg.RuntimeParams["timezone"]
		if zone == "" {
			zone = cfg.RuntimeParams["TimeZone"]
		}
		if zone != c.want {
			t.Errorf("%s: %q → пояс сессии %q, ждали %q", c.name, got, zone, c.want)
		}
		if m := raw.FindStringSubmatch(got); len(m) < 3 || m[2] != c.want {
			t.Errorf("%s: gorm прочтёт пояс из %q как %q, ждали %q", c.name, got, m, c.want)
		} else if _, err := time.LoadLocation(m[2]); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.want != tz && got != c.dsn {
			t.Errorf("%s: строку с поясом переписали: %q", c.name, got)
		}
	}
	if got := withClubTimeZone(buildDSN(), tz); !contains(got, " timezone=Europe/Warsaw") {
		t.Errorf("buildDSN без пояса клуба: %q", got)
	}
}
