package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"radio-backend/internal/auth"

	"github.com/go-chi/chi/v5"
)

// Rotas de cookies do YouTube. São servidas pela Media API (ADR-0002): o
// cookies.txt fica no volume que ela compartilha com o Media Worker.

type UploadCookiesRequest struct {
	Content string `json:"content"`
}

func UploadCookiesHandler(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationId")

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader || !auth.ValidateDJToken(token, stationID) {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}

	var req UploadCookiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		http.Error(w, "conteúdo de cookies inválido ou vazio", http.StatusBadRequest)
		return
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	_ = os.MkdirAll(dataDir, 0755)

	cookiesPath := filepath.Join(dataDir, "cookies.txt")
	content := strings.TrimSpace(req.Content)

	if strings.HasPrefix(content, "IyBOZXRzY2FwZ") {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err == nil && len(decoded) > 0 {
			content = string(decoded)
		}
	}

	if err := os.WriteFile(cookiesPath, []byte(content), 0600); err != nil {
		http.Error(w, fmt.Sprintf("erro ao salvar cookies: %v", err), http.StatusInternalServerError)
		return
	}
	// cookies.txt guarda a sessão do YouTube: só o próprio processo lê (0600),
	// inclusive se o arquivo já existia com outra permissão.
	_ = os.Chmod(cookiesPath, 0600)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Cookies do YouTube salvos com sucesso no servidor!",
	})
}

func GetCookiesStatusHandler(w http.ResponseWriter, r *http.Request) {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cookiesPath := filepath.Join(dataDir, "cookies.txt")
	hasCookies := false
	if fi, err := os.Stat(cookiesPath); err == nil && fi.Size() > 0 {
		hasCookies = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hasCookies": hasCookies,
	})
}
