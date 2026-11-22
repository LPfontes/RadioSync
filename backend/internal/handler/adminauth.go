package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"radio-backend/internal/auth"

	"github.com/go-chi/chi/v5"
)

// bearerToken extrai o token do header "Authorization: Bearer <token>".
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// RequireAdmin protege as rotas de administração com o JWT de admin.
// Devolve 503 se ADMIN_PASSWORD não estiver configurado e 401 se o token
// estiver ausente, expirado ou não for de admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.AdminEnabled() {
			writeJSONError(w, http.StatusServiceUnavailable, "painel admin desabilitado: defina ADMIN_PASSWORD")
			return
		}
		if !auth.ValidateAdminToken(bearerToken(r)) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="radiosync-admin"`)
			writeJSONError(w, http.StatusUnauthorized, "token de admin ausente, inválido ou expirado")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type adminLoginRequest struct {
	Password string `json:"password"`
}

// AdminLogin troca a senha de admin por um JWT de admin com expiração.
// POST /api/v1/admin/login  {"password": "..."}
func AdminLogin(w http.ResponseWriter, r *http.Request) {
	if !auth.AdminEnabled() {
		writeJSONError(w, http.StatusServiceUnavailable, "painel admin desabilitado: defina ADMIN_PASSWORD")
		return
	}

	var req adminLoginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if !auth.CheckAdminPassword(req.Password) {
		log.Printf("admin: falha de login de %s", r.RemoteAddr)
		writeJSONError(w, http.StatusUnauthorized, "senha inválida")
		return
	}

	token, exp, err := auth.GenerateAdminToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}
	log.Printf("admin: login bem-sucedido de %s", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token,
		"expiresAt": exp.UnixMilli(),
	})
}

// AdminVerify responde 204 quando o token de admin é válido. Fica atrás de
// RequireAdmin e é usado pelo ForwardAuth do Traefik (ADR-0005).
func AdminVerify(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// IssueDJTokenAdmin emite, sob demanda, um token de DJ para uma estação.
// Substitui o antigo vazamento do djToken de todas as salas na listagem.
// POST /api/v1/admin/stations/{stationId}/dj-token
func IssueDJTokenAdmin(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	stationsMu.RLock()
	_, ok := stations[stationID]
	stationsMu.RUnlock()
	if !ok {
		writeJSONError(w, http.StatusNotFound, "estação não encontrada")
		return
	}

	token, err := auth.GenerateDJToken(stationID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}
	log.Printf("admin: token de DJ emitido para a estação %s (%s)", stationID, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{
		"stationId": stationID,
		"djToken":   token,
	})
}
