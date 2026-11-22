package handler

import (
	"net/http"
	"strings"
)

// Autenticação do WebSocket sem token na URL (ADR-0005).
//
// O navegador não permite headers customizados no WebSocket, então o token
// de DJ vai no header Sec-WebSocket-Protocol, como um subprotocolo:
//
//	new WebSocket(url, ["radiosync.v1", "radiosync.auth.<djToken>"])
//
// O servidor responde escolhendo só "radiosync.v1"; o token nunca é ecoado.
// Diferente da query string, esse header não aparece nos logs de acesso do
// Go (chi Logger) nem do Traefik.
const (
	wsProtocol           = "radiosync.v1"
	wsAuthProtocolPrefix = "radiosync.auth."
	WSProtocol           = wsProtocol
	WSAuthProtocolPrefix = wsAuthProtocolPrefix
)

// WSOfferedProtocols lista os subprotocolos oferecidos pelo cliente.
func WSOfferedProtocols(r *http.Request) []string {
	var out []string
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func wsOfferedProtocols(r *http.Request) []string {
	return WSOfferedProtocols(r)
}

// WSTokenFromProtocols extrai o token de DJ de "radiosync.auth.<token>" e
// informa se o cliente ofereceu "radiosync.v1".
func WSTokenFromProtocols(r *http.Request) (token string, offersV1 bool) {
	for _, p := range WSOfferedProtocols(r) {
		switch {
		case p == wsProtocol:
			offersV1 = true
		case strings.HasPrefix(p, wsAuthProtocolPrefix) && token == "":
			token = strings.TrimPrefix(p, wsAuthProtocolPrefix)
		}
	}
	return token, offersV1
}

func wsTokenFromProtocols(r *http.Request) (token string, offersV1 bool) {
	return WSTokenFromProtocols(r)
}
