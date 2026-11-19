package handlers

import (
	"strings"
	"testing"
)

func TestCSVCellEscapesFormulaAndNewlines(t *testing.T) {
	cases := map[string]string{
		"=1+1":        "'=1+1",
		"+1":          "'+1",
		"-1":          "'-1",
		"@cmd":        "'@cmd",
		"normal":      "normal",
		"line\nbreak": "line break",
	}
	for in, want := range cases {
		if got := csvCell(in); got != want {
			t.Fatalf("csvCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateBaseURLAllowlist(t *testing.T) {
	s := &Server{}
	ok := []string{
		"https://api.deepseek.com",
		"https://api.deepseek.com/v1",
		"http://127.0.0.1:18080",
		"http://localhost:9999",
	}
	for _, raw := range ok {
		if err := s.validateBaseURL(raw); err != nil {
			t.Fatalf("expected %q to be allowed: %v", raw, err)
		}
	}
	bad := []string{
		"http://api.deepseek.com",
		"https://evil.example",
		"https://api.deepseek.com.evil.example",
		"ftp://api.deepseek.com",
	}
	for _, raw := range bad {
		if err := s.validateBaseURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestAllowedBaseURLPrefixBoundary(t *testing.T) {
	t.Setenv("ALLOWED_API_BASE_URLS", "https://proxy.example.com")
	s := &Server{}
	if err := s.validateBaseURL("https://proxy.example.com/v1"); err != nil {
		t.Fatalf("expected path under allowed base to pass: %v", err)
	}
	for _, raw := range []string{
		"https://proxy.example.com.evil.example",
		"https://proxy.example.com@evil.example",
	} {
		if err := s.validateBaseURL(raw); err == nil {
			t.Fatalf("expected prefix bypass %q to be rejected", raw)
		}
	}
}

func TestTokenRoundTripAndExpiryShape(t *testing.T) {
	s := NewServer(nil)
	tok := s.signToken()
	if tok == "" || !strings.Contains(tok, ".") {
		t.Fatalf("unexpected token: %q", tok)
	}
	if !s.verifyToken(tok) {
		t.Fatal("fresh token should verify")
	}
	if s.verifyToken(tok + "x") {
		t.Fatal("tampered token must not verify")
	}
}
