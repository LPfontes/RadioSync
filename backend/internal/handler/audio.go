package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"radio-backend/internal/storage"

	"github.com/go-chi/chi/v5"
)

// ServeAudioFromObjectStore cria um http.HandlerFunc que serve arquivos .opus
// diretamente do MinIO / ObjectStore (bucket "musicas"), sem depender do disco local (ADR-0006).
//
// Suporta dois modos de entrega:
//   - Stream / Proxy (padrão): lê do ObjectStore e transmite ao cliente com Content-Type audio/ogg e suporte a Range.
//   - URL Pré-assinada: quando deliveryMode == "presigned", ?redirect=1 (302 Found) ou ?presign=1 (JSON {"url": "..."}).
func ServeAudioFromObjectStore(store storage.ObjectStore, deliveryMode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "object store não configurado", http.StatusServiceUnavailable)
			return
		}

		filename := chi.URLParam(r, "filename")
		if filename == "" {
			trimmed := strings.TrimPrefix(r.URL.Path, "/musicas/")
			filename = path.Base(trimmed)
		}
		filename = strings.TrimSpace(filename)
		if filename == "" || filename == "." || filename == "/" || strings.Contains(filename, "..") {
			http.Error(w, "nome de arquivo inválido", http.StatusBadRequest)
			return
		}

		// Endpoint JSON para obter a URL pré-assinada diretamente
		if r.URL.Query().Get("presign") == "1" || r.URL.Query().Get("format") == "json" {
			presignedURL, err := store.PresignedGetURL(r.Context(), filename, 15*time.Minute)
			if err != nil {
				if errors.Is(err, storage.ErrObjectNotFound) {
					http.Error(w, "áudio não encontrado", http.StatusNotFound)
					return
				}
				http.Error(w, "erro ao gerar URL pré-assinada", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"filename": filename,
				"url":      presignedURL,
			})
			return
		}

		// Modo redirect (302 Found) para a URL pré-assinada do MinIO
		if strings.EqualFold(deliveryMode, "presigned") || r.URL.Query().Get("redirect") == "1" {
			presignedURL, err := store.PresignedGetURL(r.Context(), filename, 15*time.Minute)
			if err != nil {
				if errors.Is(err, storage.ErrObjectNotFound) {
					http.Error(w, "áudio não encontrado", http.StatusNotFound)
					return
				}
				http.Error(w, "erro ao gerar URL pré-assinada", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, presignedURL, http.StatusFound)
			return
		}

		// Modo stream direto do MinIO (funciona transparentemente mesmo quando o container MinIO não é exposto ao navegador)
		rc, info, err := store.Get(r.Context(), filename)
		if err != nil {
			if errors.Is(err, storage.ErrObjectNotFound) {
				http.Error(w, "áudio não encontrado", http.StatusNotFound)
				return
			}
			http.Error(w, "erro ao buscar áudio no MinIO", http.StatusBadGateway)
			return
		}
		defer rc.Close()

		contentType := info.ContentType
		if contentType == "" {
			contentType = "audio/ogg"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if info.Size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		}
		if !info.LastModified.IsZero() {
			w.Header().Set("Last-Modified", info.LastModified.UTC().Format(http.TimeFormat))
		}

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}
}
