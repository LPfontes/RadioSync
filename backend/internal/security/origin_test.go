package security

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAllowedOrigins(t *testing.T) {
	casos := map[string][]string{
		"":                          nil,
		"   ":                       nil,
		"*":                         nil,
		"https://radio.exemplo.com": {"https://radio.exemplo.com"},
		" https://Radio.Exemplo.com/ , http://localhost:5173": {"https://radio.exemplo.com", "http://localhost:5173"},
		"https://a.com,https://a.com":                         {"https://a.com"},
		// inválidos são descartados: sem esquema, com caminho, outro esquema, curinga
		"radio.com,https://b.com/painel,ftp://c.com,*,https://ok.com": {"https://ok.com"},
	}
	for env, esperado := range casos {
		t.Setenv("ALLOWED_ORIGINS", env)
		if got := AllowedOrigins(); !reflect.DeepEqual(got, esperado) {
			t.Errorf("ALLOWED_ORIGINS=%q: obtido %v, esperado %v", env, got, esperado)
		}
	}
}

func TestIsAllowedOrigin(t *testing.T) {
	allowed := []string{"https://radio.exemplo.com", "http://localhost:5173"}

	for _, o := range []string{"https://radio.exemplo.com", "HTTPS://RADIO.EXEMPLO.COM", "http://localhost:5173"} {
		if !IsAllowedOrigin(o, allowed) {
			t.Errorf("%q deveria ser aceita", o)
		}
	}
	for _, o := range []string{
		"", "null", "*",
		"https://evil.example",
		"http://radio.exemplo.com",          // esquema diferente
		"https://radio.exemplo.com:8443",    // porta diferente
		"https://radio.exemplo.com.evil.io", // sufixo
		"https://sub.radio.exemplo.com",     // subdomínio
	} {
		if IsAllowedOrigin(o, allowed) {
			t.Errorf("%q não deveria ser aceita", o)
		}
	}
	if IsAllowedOrigin("https://radio.exemplo.com", nil) {
		t.Error("allowlist vazia não pode aceitar nenhuma origem")
	}
}

func TestCheckWebSocketOrigin(t *testing.T) {
	check := CheckWebSocketOrigin([]string{"https://radio.exemplo.com"})

	casos := []struct {
		host, origin string
		ok           bool
	}{
		{"localhost", "", true},                                 // cliente que não é navegador
		{"localhost", "http://localhost", true},                 // mesma origem via gateway
		{"localhost:8080", "http://localhost:8080", true},       // mesma origem direto
		{"localhost:5173", "http://localhost:5173", true},       // proxy do Vite preserva o Host
		{"api.interno:8080", "https://radio.exemplo.com", true}, // allowlist
		{"localhost", "https://evil.example", false},
		{"localhost:8080", "http://localhost:5173", false}, // porta diferente, fora da allowlist
		{"localhost", "null", false},
		{"localhost", "lixo", false},
	}
	for _, c := range casos {
		req := httptest.NewRequest("GET", "/ws/stations/ABC123", nil)
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if got := check(req); got != c.ok {
			t.Errorf("Host=%q Origin=%q: obtido %v, esperado %v", c.host, c.origin, got, c.ok)
		}
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://radio.exemplo.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	preflight := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/stations", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := preflight("https://radio.exemplo.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://radio.exemplo.com" {
		t.Fatalf("origem permitida: Allow-Origin=%q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("Allow-Credentials não deveria ser enviado")
	}

	rec = preflight("https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origem não permitida recebeu Allow-Origin=%q", got)
	}

	// Sem allowlist ninguém de fora recebe CORS (o rs/cors liberaria tudo com
	// AllowedOrigins vazio; aqui não pode).
	h = CORS(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/library", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("sem allowlist, Allow-Origin deveria ser vazio, obtido %q", got)
	}
}
