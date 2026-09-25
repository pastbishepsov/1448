package main

import (
	"math"
	"testing"
)

// З6-и3: цена — от 0,01 zł, не больше двух знаков и не выше потолка поля.
func TestValidatePLN(t *testing.T) {
	cases := []struct {
		name string
		v    float64
		want bool
	}{
		{"обычная цена", 23, true},
		{"копейка — минимум", 0.01, true},
		{"два знака с хвостом float", 19.99, true},
		{"0,1 + 0,2 из калькулятора", 0.1 + 0.2, true},
		{"ровно потолок", 500, true},
		{"меньше копейки", 0.004, false},
		{"три знака", 1.005, false},
		{"три знака на копейках", 0.015, false},
		{"ноль", 0, false},
		{"минус", -5, false},
		{"выше потолка на копейку", 500.01, false},
		{"3·10⁹", 3e9, false},
		{"NaN", math.NaN(), false},
		{"бесконечность", math.Inf(1), false},
	}
	for _, tc := range cases {
		if got := validatePLN(tc.v, 500); got != tc.want {
			t.Errorf("%s: validatePLN(%v, 500) = %v, ожидалось %v", tc.name, tc.v, got, tc.want)
		}
	}
}

func TestValidSort(t *testing.T) {
	ip := func(v int) *int { return &v }
	cases := []struct {
		name string
		sort *int
		want bool
	}{
		{"не прислан", nil, true},
		{"ноль", ip(0), true},
		{"граница снизу", ip(-maxSort), true},
		{"граница сверху", ip(maxSort), true},
		{"выше границы", ip(maxSort + 1), false},
		{"ниже границы", ip(-maxSort - 1), false},
		{"3·10⁹", ip(3000000000), false},
	}
	for _, tc := range cases {
		if got := validSort(tc.sort); got != tc.want {
			t.Errorf("%s: validSort = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
