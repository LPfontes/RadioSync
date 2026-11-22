package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Suporte ao deploy (ADR-0007).

var startedAt = time.Now()

// AppVersion é a versão em execução (tag da imagem, normalmente o SHA do
// commit), vinda de APP_VERSION. O script de deploy usa esse campo para saber
// se a versão nova já está no ar.
func AppVersion() string {
	if v := os.Getenv("APP_VERSION"); v != "" {
		return v
	}
	return "dev"
}

// Healthz responde 200 quando o servidor está pronto: estado carregado e
// diretórios de dados acessíveis. Usado pelo HEALTHCHECK da imagem e pelo
// scripts/deploy.sh. Não expõe nada sensível (só versão, contagem e uptime).
func Healthz(w http.ResponseWriter, r *http.Request) {
	status, code := "ok", http.StatusOK
	if _, err := os.Stat(dataDir()); err != nil {
		status, code = "data_dir indisponível", http.StatusServiceUnavailable
	} else if _, err := os.Stat(getMusicDir()); err != nil {
		status, code = "music_dir indisponível", http.StatusServiceUnavailable
	}

	stationsMu.RLock()
	n := len(stations)
	stationsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        status,
		"version":       AppVersion(),
		"stations":      n,
		"uptimeSeconds": int(time.Since(startedAt).Seconds()),
	})
}

// SaveOnShutdown faz um último salvamento do estado ao receber SIGTERM ou
// SIGINT (docker stop, deploy, Ctrl+C) e encerra o processo.
//
// As mudanças vindas do WebSocket já são salvas na hora; o que só sai no ciclo
// de 30 s do PeriodicSave são as mudanças feitas pelo próprio servidor (por
// exemplo, o avanço automático de faixa). O salvamento final cobre essa janela.
func SaveOnShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)
	go func() {
		sig := <-ch
		log.Printf("sinal %v recebido: salvando o estado antes de encerrar (versão %s)", sig, AppVersion())
		SaveStations()
		SaveTracksCatalog()
		log.Printf("estado salvo; encerrando")
		os.Exit(0)
	}()
}
