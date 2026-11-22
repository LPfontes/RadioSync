// Package security concentra as regras de borda da aplicação (ADR-0005).
package security

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/rs/cors"
)

// AllowedOrigins lê ALLOWED_ORIGINS (lista separada por vírgula, ex.:
// "https://radio.exemplo.com,http://localhost:5173") e normaliza cada item
// para "esquema://host[:porta]" em minúsculas. Itens inválidos e "*" são
// descartados: curinga não é aceito.
//
// Vazio é o padrão e significa "somente a mesma origem", que já cobre o app
// servido pelo Go, pelo gateway e pelo proxy do Vite em desenvolvimento.
func AllowedOrigins() []string {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		o, ok := normalizeOrigin(item)
		if !ok || seen[o] {
			continue
		}
		seen[o] = true
		out = append(out, o)
	}
	return out
}

func normalizeOrigin(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "/"))
	if s == "" || s == "*" {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", false
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), true
}

// IsAllowedOrigin informa se a origem está na allowlist.
func IsAllowedOrigin(origin string, allowed []string) bool {
	o, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == o {
			return true
		}
	}
	return false
}

// CheckWebSocketOrigin é o CheckOrigin do upgrader do WebSocket.
//
// Aceita: requisições sem Origin (clientes que não são navegador, como k6 e
// testes), a mesma origem (host do Origin igual ao Host da requisição; o
// Traefik e o proxy do Vite preservam o Host) e as origens da allowlist.
// Recusa o resto, o que bloqueia Cross-Site WebSocket Hijacking.
func CheckWebSocketOrigin(allowed []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}
		return IsAllowedOrigin(origin, allowed)
	}
}

// CORS devolve o middleware de CORS restrito (ADR-0005).
//
// AllowOriginFunc é usado de propósito: com AllowedOrigins vazio o rs/cors
// liberaria todas as origens. Sem allowlist, nenhuma origem externa recebe
// cabeçalhos CORS (o app é servido na mesma origem e não precisa deles).
// Sem credenciais: a autenticação é por header Bearer, não por cookie.
func CORS(allowed []string) func(http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return IsAllowedOrigin(origin, allowed)
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           600,
	}).Handler
}
