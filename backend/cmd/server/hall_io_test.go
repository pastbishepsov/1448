package main

import (
	"strings"
	"testing"
)

// З25-и1: файл зала — проверка целиком; клетка словами.

func hallOK() hallFile {
	x0, y0, x1 := 0, 0, 2
	return hallFile{Format: hallFormat, Version: hallVersion, Layout: hallLayout{W: 12, H: 8},
		Zones: []hallZone{{Name: "Standard", RatePLN: 20, VatPct: 23, Sort: 1,
			Rules: []hallRule{{DaysMask: 31, StartMin: 1080, EndMin: 1320, RatePLN: 25}, {DaysMask: 96, StartMin: 0, EndMin: 1440, RatePLN: 18, ValidFrom: "2026-12-01"}}}},
		Computers: []hallPC{{Name: "PC-01", Zone: "Standard", PosX: &x0, PosY: &y0, MAC: "aa:bb:cc:dd:ee:01"}, {Name: "PC-02", Zone: "Standard", PosX: &x1, PosY: &y0},
			{Name: "Склад"}}}
}

func TestValidateHall(t *testing.T) {
	if msg := validateHall(hallOK()); msg != "" {
		t.Fatalf("правильный файл: %q", msg)
	}
	big := 30
	cases := []struct {
		name string
		edit func(f *hallFile)
		want string
	}{
		{"не тот формат", func(f *hallFile) { f.Format = "x" }, "Это не файл зала"},
		{"другая версия", func(f *hallFile) { f.Version = 2 }, "Файл версии 2"},
		{"схема 3×3", func(f *hallFile) { f.Layout.W = 3 }, "Схема в файле"},
		{"зона без имени", func(f *hallFile) { f.Zones[0].Name = " " }, "Зона «"},
		{"зона дважды", func(f *hallFile) { f.Zones = append(f.Zones, hallZone{Name: "standard", RatePLN: 1, VatPct: 23}) }, "в файле дважды"},
		{"VAT 7", func(f *hallFile) { f.Zones[0].VatPct = 7 }, "ставка VAT 7"},
		{"правило без дней", func(f *hallFile) { f.Zones[0].Rules[0].DaysMask = 0 }, "правило"},
		{"правила внахлёст", func(f *hallFile) { f.Zones[0].Rules[1].DaysMask = 127 }, "накладываются"},
		{"дата правила", func(f *hallFile) { f.Zones[0].Rules[1].ValidFrom = "01.12.2026" }, "не ГГГГ-ММ-ДД"},
		{"ПК дважды", func(f *hallFile) { f.Computers[1].Name = "pc-01" }, "в файле дважды"},
		{"клетка без y", func(f *hallFile) { f.Computers[1].PosY = nil }, "оба поля"},
		{"клетка за схемой", func(f *hallFile) { f.Computers[1].PosX = &big }, "за схемой 12×8"},
		{"одна клетка", func(f *hallFile) { x := 0; f.Computers[1].PosX = &x }, "в одной клетке 1×1"},
		{"кривой MAC", func(f *hallFile) { f.Computers[0].MAC = "zz" }, "MAC «zz»"},
	}
	for _, tc := range cases {
		f := hallOK()
		f.Zones[0].Rules = append([]hallRule{}, f.Zones[0].Rules...)
		f.Computers = append([]hallPC{}, f.Computers...)
		tc.edit(&f)
		if got := validateHall(f); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, ожидалось с «%s»", tc.name, got, tc.want)
		}
	}
}

func TestHallCells(t *testing.T) {
	x, y := 2, 0
	if cellText(&x, &y) != "клетка 3×1" || cellText(nil, nil) != "вне схемы" {
		t.Error("клетка словами: «клетка 3×1», без клетки — «вне схемы»")
	}
	a, b := 1, 1
	if !samePos(nil, nil, nil, nil) || samePos(&a, &b, nil, nil) || !samePos(&a, &b, &a, &b) || samePos(&a, &b, &x, &y) {
		t.Error("samePos: обе вне схемы — одно и то же, клетка и вне схемы — нет")
	}
	if got := dropCellChange([]string{"зона: A → B", "клетка 1×1 → клетка 3×1", "вне схемы → клетка 2×2", "MAC нет → AA"}); len(got) != 2 {
		t.Errorf("убрать перенос клетки из правок: %q", got)
	}
}
