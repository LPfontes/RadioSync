package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"radio-backend/internal/storage"
)

func TestServeAudioFromObjectStore_WithoutLocalDisk(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryObjectStore(storage.DefaultMusicBucket)

	opusPayload := []byte("OggS\x00\x02minio-stored-audio-payload")
	if err := store.Put(ctx, "test-song.opus", bytes.NewReader(opusPayload), int64(len(opusPayload)), "audio/ogg"); err != nil {
		t.Fatal(err)
	}

	h := ServeAudioFromObjectStore(store, "proxy")

	// 1. Streaming direto do ObjectStore sem arquivo em disco
	req := httptest.NewRequest(http.MethodGet, "/musicas/test-song.opus", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200, obteve %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/ogg" {
		t.Errorf("esperava Content-Type audio/ogg, obteve %s", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if !bytes.Equal(body, opusPayload) {
		t.Errorf("payload de áudio retornado difere do armazenado no MinIO")
	}

	// 2. Geração de URL pré-assinada via ?presign=1
	reqPresign := httptest.NewRequest(http.MethodGet, "/musicas/test-song.opus?presign=1", nil)
	recPresign := httptest.NewRecorder()
	h.ServeHTTP(recPresign, reqPresign)

	if recPresign.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200 no presign=1, obteve %d", recPresign.Code)
	}
	var presignResp map[string]string
	if err := json.Unmarshal(recPresign.Body.Bytes(), &presignResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(presignResp["url"], "test-song.opus") {
		t.Errorf("URL pré-assinada inválida: %v", presignResp)
	}

	// 3. Redirect 302 para URL pré-assinada no modo "presigned"
	hRedirect := ServeAudioFromObjectStore(store, "presigned")
	reqRedir := httptest.NewRequest(http.MethodGet, "/musicas/test-song.opus", nil)
	recRedir := httptest.NewRecorder()
	hRedirect.ServeHTTP(recRedir, reqRedir)

	if recRedir.Code != http.StatusFound {
		t.Fatalf("esperava HTTP 302 Found no modo presigned, obteve %d", recRedir.Code)
	}
	if loc := recRedir.Header().Get("Location"); !strings.Contains(loc, "test-song.opus") {
		t.Errorf("header Location inesperado: %s", loc)
	}

	// 4. Arquivo inexistente -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/musicas/nao-existe.opus", nil)
	rec404 := httptest.NewRecorder()
	h.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("esperava HTTP 404 para arquivo inexistente, obteve %d", rec404.Code)
	}
}
