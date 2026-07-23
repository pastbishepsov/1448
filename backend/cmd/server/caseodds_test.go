package main

import (
	"math"
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Таблица шансов обязана быть арифметически честной: проценты внутри тира
// в сумме дают 100, распределение тиров бонусного кейса — тоже 100.
func TestCaseOddsSumTo100(t *testing.T) {
	for _, rtp := range []float64{0.8, 1.0, 1.2} {
		tiers := caseOddsTiers(rtp)
		if len(tiers) != 5 {
			t.Fatalf("rtp=%v: тиров %d, хотим 5", rtp, len(tiers))
		}
		bonusSum := 0.0
		for _, o := range tiers {
			sum := o.CoinsPct + o.BusterPct + o.JackpotPct
			if math.Abs(sum-100) > 0.03 {
				t.Errorf("rtp=%v тир %s: сумма процентов %.3f, хотим 100", rtp, o.Tier, sum)
			}
			if o.CoinsPct < 0 || o.BusterPct < 0 || o.JackpotPct < 0 {
				t.Errorf("rtp=%v тир %s: отрицательный процент: %+v", rtp, o.Tier, o)
			}
			bonusSum += o.BonusRollPct
		}
		if math.Abs(bonusSum-100) > 0.03 {
			t.Errorf("rtp=%v: сумма bonus_roll_pct %.3f, хотим 100", rtp, bonusSum)
		}
	}
}

// Пороги зеркалят Roll: при экстремальном RTP монеты уходят в 0, но проценты
// не ломаются (клэмп 100000 — как в models/case.go).
func TestCaseOddsExtremeRTP(t *testing.T) {
	for _, o := range caseOddsTiers(2.0) {
		sum := o.CoinsPct + o.BusterPct + o.JackpotPct
		if math.Abs(sum-100) > 0.03 {
			t.Errorf("rtp=2.0 тир %s: сумма %.3f, хотим 100", o.Tier, sum)
		}
		if o.CoinsPct < 0 {
			t.Errorf("rtp=2.0 тир %s: coins_pct < 0 (%v)", o.Tier, o.CoinsPct)
		}
	}
}

// Базовые шансы (rtp=1) обязаны совпадать с dropConfigs — защита от
// рассинхрона «код нарисовал одно, роллит другое».
func TestCaseOddsMatchDropConfig(t *testing.T) {
	for _, o := range caseOddsTiers(1.0) {
		cfg := models.DropConfigFor(o.Tier)
		if want := float64(cfg.JackpotChance) / 1000; math.Abs(o.JackpotPct-want) > 0.001 {
			t.Errorf("тир %s: jackpot_pct %v, хотим %v", o.Tier, o.JackpotPct, want)
		}
		if want := float64(cfg.BusterChance) / 1000; math.Abs(o.BusterPct-want) > 0.001 {
			t.Errorf("тир %s: buster_pct %v, хотим %v", o.Tier, o.BusterPct, want)
		}
		if o.CoinsMin != cfg.CoinsMin || o.CoinsMax != cfg.CoinsMax {
			t.Errorf("тир %s: диапазон монет %d–%d, хотим %d–%d",
				o.Tier, o.CoinsMin, o.CoinsMax, cfg.CoinsMin, cfg.CoinsMax)
		}
	}
}

// Версия и дата таблицы объявлены — UI показывает их игроку.
func TestCaseOddsVersionDeclared(t *testing.T) {
	if models.CaseOddsVersion < 1 {
		t.Errorf("CaseOddsVersion = %d, хотим >= 1", models.CaseOddsVersion)
	}
	if len(models.CaseOddsDate) != 10 {
		t.Errorf("CaseOddsDate = %q, хотим YYYY-MM-DD", models.CaseOddsDate)
	}
}
