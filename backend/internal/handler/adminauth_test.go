package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
)

const testStationID = "TSTADM"

// adminRouter monta as rotas de admin como em cmd/server/main.go.
func adminRouter() http.Handler {
	r := chi.NewRouter()
	r.With(RequireAdmin).Get("/api/v1/debug", DebugHandler)
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Post("/login", AdminLogin)
		r.Group(func(r chi.Router) {
			r.Use(RequireAdmin)
			r.Get("/verify", AdminVerify)
			r.Get("/stations", GetAdminStations)
			r.Post("/stations/{stationId}/dj-token", IssueDJTokenAdmin)
			r.Delete("/stations/{stationId}", DeleteStationAdmin)
			r.Post("/purge-orphans", PurgeOrphanTracksAdmin)
			r.Delete("/stations/{stationId}/tracks/{trackId}", RemoveTrackFromStationAdmin)
			r.Put("/tracks/{trackId}", RenameTrackAdmin)
		})
	})
	return r
}

func setupAdminTest(t *testing.T, password string) http.Handler {
	t.Helper()
	t.Setenv("JWT_SECRET", "segredo-de-teste")
	t.Setenv("ADMIN_PASSWORD", password)
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("MUSIC_DIR", t.TempDir())

	djToken, _ := auth.GenerateDJToken(testStationID)
	stationsMu.Lock()
	stations[testStationID] = model.NewStation(testStationID, djToken)
	stationsMu.Unlock()
	t.Cleanup(func() {
		stationsMu.Lock()
		delete(stations, testStationID)
		stationsMu.Unlock()
	})
	return adminRouter()
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func login(t *testing.T, h http.Handler, password string) string {
	t.Helper()
	rec := do(h, http.MethodPost, "/api/v1/admin/login", "", `{"password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: esperado 200, obtido %d (%s)", rec.Code, rec.Body)
	}
	var resp struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Token == "" || resp.ExpiresAt == 0 {
		t.Fatalf("login sem token/expiresAt: %s", rec.Body)
	}
	return resp.Token
}

var rotasProtegidas = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/admin/verify"},
	{http.MethodGet, "/api/v1/admin/stations"},
	{http.MethodPost, "/api/v1/admin/stations/" + testStationID + "/dj-token"},
	{http.MethodDelete, "/api/v1/admin/stations/" + testStationID},
	{http.MethodPost, "/api/v1/admin/purge-orphans"},
	{http.MethodDelete, "/api/v1/admin/stations/" + testStationID + "/tracks/t1"},
	{http.MethodPut, "/api/v1/admin/tracks/t1"},
	{http.MethodGet, "/api/v1/debug"},
}

func TestRotasAdmin_SemToken401(t *testing.T) {
	h := setupAdminTest(t, "senha-admin")
	for _, rt := range rotasProtegidas {
		if rec := do(h, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s sem token: esperado 401, obtido %d", rt.method, rt.path, rec.Code)
		}
	}
}

func TestRotasAdmin_TokenDeDJ401(t *testing.T) {
	h := setupAdminTest(t, "senha-admin")
	dj, _ := auth.GenerateDJToken(testStationID)
	for _, rt := range rotasProtegidas {
		if rec := do(h, rt.method, rt.path, dj, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s com token de DJ: esperado 401, obtido %d", rt.method, rt.path, rec.Code)
		}
	}
}

func TestRotasAdmin_Desabilitado503(t *testing.T) {
	h := setupAdminTest(t, "")
	if rec := do(h, http.MethodPost, "/api/v1/admin/login", "", `{"password":""}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("login com admin desabilitado: esperado 503, obtido %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/api/v1/admin/stations", "qualquer", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("rota admin desabilitada: esperado 503, obtido %d", rec.Code)
	}
}

func TestAdminLogin_SenhaErrada(t *testing.T) {
	h := setupAdminTest(t, "senha-admin")
	if rec := do(h, http.MethodPost, "/api/v1/admin/login", "", `{"password":"errada"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("senha errada: esperado 401, obtido %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/v1/admin/login", "", `lixo`); rec.Code != http.StatusBadRequest {
		t.Fatalf("corpo inválido: esperado 400, obtido %d", rec.Code)
	}
}

func TestAdminFluxoCompleto(t *testing.T) {
	h := setupAdminTest(t, "senha-admin")
	token := login(t, h, "senha-admin")

	if rec := do(h, http.MethodGet, "/api/v1/admin/verify", token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("verify: esperado 204, obtido %d", rec.Code)
	}

	// A listagem não pode mais expor o djToken de nenhuma estação.
	rec := do(h, http.MethodGet, "/api/v1/admin/stations", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listagem: esperado 200, obtido %d", rec.Code)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "djtoken") {
		t.Fatalf("listagem admin ainda expõe djToken: %s", rec.Body)
	}

	// Token de DJ sob demanda vale para a estação pedida.
	rec = do(h, http.MethodPost, "/api/v1/admin/stations/"+testStationID+"/dj-token", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("dj-token: esperado 200, obtido %d", rec.Code)
	}
	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !auth.ValidateDJToken(resp["djToken"], testStationID) {
		t.Fatal("token de DJ emitido pelo admin não é válido para a estação")
	}

	if rec := do(h, http.MethodPost, "/api/v1/admin/stations/NAOEXI/dj-token", token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("dj-token de estação inexistente: esperado 404, obtido %d", rec.Code)
	}
}

func TestBearerToken(t *testing.T) {
	casos := map[string]string{
		"Bearer abc":    "abc",
		"bearer abc":    "abc",
		"Bearer   abc ": "abc",
		"abc":           "",
		"Basic abc":     "",
		"":              "",
	}
	for header, esperado := range casos {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := bearerToken(req); got != esperado {
			t.Errorf("bearerToken(%q) = %q, esperado %q", header, got, esperado)
		}
	}
}
