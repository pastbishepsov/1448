package main

import (
	"reflect"
	"testing"
)

func TestAccelFromParams(t *testing.T) {
	if accelFromParams([3]int32{6, 10, 1}) != true {
		t.Error("accel=1 должен читаться как включённый")
	}
	if accelFromParams([3]int32{0, 0, 0}) != false {
		t.Error("accel=0 должен читаться как выключенный")
	}
	// пороги не важны — важен только третий параметр
	if accelFromParams([3]int32{6, 10, 0}) != false {
		t.Error("при accel=0 пороги не должны включать акселерацию")
	}
}

func TestPointerSpeedIdeal(t *testing.T) {
	if !pointerSpeedIdeal(10) {
		t.Error("10/20 — идеальная скорость (1:1)")
	}
	for _, s := range []int{1, 6, 9, 11, 20} {
		if pointerSpeedIdeal(s) {
			t.Errorf("%d не должна считаться идеальной", s)
		}
	}
}

func TestDetectMouseVendors(t *testing.T) {
	procs := []string{
		"chrome.exe", "LGHUB.exe", "lghub_agent.exe",
		"RzSynapse.exe", "steam.exe", "SteelSeriesGG.exe",
	}
	got := detectMouseVendors(procs)
	want := []string{"Logitech", "Razer", "SteelSeries"} // отсортировано, без дублей
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detectMouseVendors = %v, ожидалось %v", got, want)
	}

	// нет знакомого софта — пустой список
	if v := detectMouseVendors([]string{"chrome.exe", "notepad.exe"}); len(v) != 0 {
		t.Errorf("посторонние процессы дали вендоров: %v", v)
	}
	// регистр не важен
	if v := detectMouseVendors([]string{"RAZER CENTRAL.EXE"}); len(v) != 1 || v[0] != "Razer" {
		t.Errorf("регистр процесса сломал детект: %v", v)
	}
}
