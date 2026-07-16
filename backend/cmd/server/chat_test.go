package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateChatMessage(t *testing.T) {
	long := strings.Repeat("а", 501)
	cases := []struct {
		name string
		kind string
		text string
		ok   bool
		code string
	}{
		{"вызов без текста — ок", "call", "", true, ""},
		{"вызов с текстом — ок", "call", "подойдите к PC-3", true, ""},
		{"текст — ок", "text", "привет", true, ""},
		{"пустой текст нельзя", "text", "   ", false, "empty_text"},
		{"неизвестный вид", "ping", "hi", false, "bad_kind"},
		{"длиннее 500 нельзя", "text", long, false, "too_long"},
	}
	for _, tc := range cases {
		ok, code := validateChatMessage(tc.kind, tc.text)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: validateChatMessage(%q, …) = (%v, %q), ожидалось (%v, %q)",
				tc.name, tc.kind, ok, code, tc.ok, tc.code)
		}
	}
}

func TestChatCooldown(t *testing.T) {
	if chatCooldown("call") != 30*time.Second {
		t.Errorf("cooldown вызова должен быть 30с")
	}
	if chatCooldown("text") != 2*time.Second {
		t.Errorf("cooldown текста должен быть 2с")
	}
	now := time.Now()
	if cooldownPassed(now.Add(-10*time.Second), now, 30*time.Second) {
		t.Errorf("10с из 30с — пауза не прошла")
	}
	if !cooldownPassed(now.Add(-31*time.Second), now, 30*time.Second) {
		t.Errorf("31с из 30с — пауза прошла")
	}
}
