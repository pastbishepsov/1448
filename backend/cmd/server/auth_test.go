package main

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Логика токенов: подпись, проверка, разделение access/refresh.
func TestTokens(t *testing.T) {
	jwtSecret = []byte("test_secret")

	// access подписывается и парсится, тип и sub на месте
	access, err := signToken("user-1", "player", tokenTypeAccess, time.Minute)
	if err != nil {
		t.Fatalf("signToken(access): %v", err)
	}
	claims, err := parseToken(access)
	if err != nil {
		t.Fatalf("parseToken(access): %v", err)
	}
	if tokenType(claims) != tokenTypeAccess {
		t.Errorf("typ = %q, ожидался access", tokenType(claims))
	}
	if sub, _ := claims["sub"].(string); sub != "user-1" {
		t.Errorf("sub = %q, ожидался user-1", sub)
	}

	// refresh валиден, но его тип НЕ access → middleware обязан отклонить
	refresh, _ := signToken("user-1", "player", tokenTypeRefresh, time.Hour)
	claims, err = parseToken(refresh)
	if err != nil {
		t.Fatalf("parseToken(refresh): %v", err)
	}
	if tokenType(claims) == tokenTypeAccess {
		t.Error("refresh-токен прошёл бы как access — дыра в middleware")
	}
	if tokenType(claims) != tokenTypeRefresh {
		t.Errorf("typ = %q, ожидался refresh", tokenType(claims))
	}

	// роль доезжает в claims
	adm, _ := signToken("user-1", "admin", tokenTypeAccess, time.Minute)
	claims, _ = parseToken(adm)
	if r, _ := claims["role"].(string); r != "admin" {
		t.Errorf("role = %q, ожидался admin", r)
	}

	// истёкший токен не проходит
	expired, _ := signToken("user-1", "player", tokenTypeAccess, -time.Minute)
	if _, err := parseToken(expired); err == nil {
		t.Error("истёкший токен прошёл проверку")
	}

	// подпись чужим секретом не проходит
	foreign, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "typ": tokenTypeAccess,
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("wrong_secret"))
	if _, err := parseToken(foreign); err == nil {
		t.Error("токен с чужой подписью прошёл проверку")
	}

	// старый токен без typ (до этого релиза) — не access и не refresh
	legacy, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwtSecret)
	claims, err = parseToken(legacy)
	if err != nil {
		t.Fatalf("parseToken(legacy): %v", err)
	}
	if tokenType(claims) == tokenTypeAccess || tokenType(claims) == tokenTypeRefresh {
		t.Errorf("legacy-токен получил тип %q, ожидался пустой", tokenType(claims))
	}
}
