package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"radio-backend/internal/auth"
	"radio-backend/internal/model"

	"github.com/go-chi/chi/v5"
)

// GET /stations/{id} renova o token de quem já é DJ e não entrega token a
// ouvintes nem a quem manda um token inválido.
func TestGetStation_RenovaTokenDoDJ(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-com-32-caracteres!")
	const id = "TSTTOK"
	stationsMu.Lock()
	stations[id] = model.NewStation(id, "")
	stationsMu.Unlock()
	t.Cleanup(func() {
		stationsMu.Lock()
		delete(stations, id)
		stationsMu.Unlock()
	})

	r := chi.NewRouter()
	r.Get("/api/v1/stations/{stationId}", GetStation)

	get := func(token string) (map[string]interface{}, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stations/"+id, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &body)
		return body, rec
	}

	dj, _ := auth.GenerateDJToken(id)
	body, rec := get(dj)
	if body["role"] != "dj" {
		t.Fatalf("role esperado dj, obtido %v", body["role"])
	}
	novo, _ := body["djToken"].(string)
	if !auth.ValidateDJToken(novo, id) {
		t.Fatal("token renovado ausente ou inválido")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("resposta com token deveria ter Cache-Control: no-store")
	}

	for nome, tok := range map[string]string{"ouvinte": "", "token inválido": "lixo"} {
		body, _ := get(tok)
		if body["role"] != "listener" {
			t.Errorf("%s: role esperado listener, obtido %v", nome, body["role"])
		}
		if _, tem := body["djToken"]; tem {
			t.Errorf("%s: não deveria receber djToken", nome)
		}
	}

	outra, _ := auth.GenerateDJToken("OUTRA1")
	if body, _ := get(outra); body["role"] != "listener" || body["djToken"] != nil {
		t.Error("token de outra estação não pode virar DJ nem receber token")
	}
}
