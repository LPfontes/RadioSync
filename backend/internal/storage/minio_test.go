package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newFakeS3Server cria um servidor HTTP compatível com S3 REST API (MinIO)
// validando headers de autenticação AWS4-HMAC-SHA256.
func newFakeS3Server(t *testing.T) *httptest.Server {
	t.Helper()
	var (
		mu      sync.RWMutex
		buckets = make(map[string]bool)
		objects = make(map[string]map[string]memoryObject)
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		hasQuerySig := r.URL.Query().Get("X-Amz-Signature") != ""
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") && !hasQuerySig {
			http.Error(w, "assinatura AWS4-HMAC-SHA256 ausente", http.StatusForbidden)
			return
		}

		trimmed := strings.Trim(r.URL.Path, "/")
		parts := strings.SplitN(trimmed, "/", 2)
		bucket := parts[0]
		key := ""
		if len(parts) > 1 {
			key = parts[1]
		}

		// Operações em nível de bucket
		if key == "" {
			switch r.Method {
			case http.MethodHead:
				mu.RLock()
				ok := buckets[bucket]
				mu.RUnlock()
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
			case http.MethodPut:
				mu.Lock()
				buckets[bucket] = true
				if objects[bucket] == nil {
					objects[bucket] = make(map[string]memoryObject)
				}
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				prefix := r.URL.Query().Get("prefix")
				mu.RLock()
				bObjs := objects[bucket]
				var xmlItems strings.Builder
				for k := range bObjs {
					if prefix == "" || strings.HasPrefix(k, prefix) {
						xmlItems.WriteString(fmt.Sprintf("<Contents><Key>%s</Key></Contents>", k))
					}
				}
				mu.RUnlock()
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult>`+xmlItems.String()+`</ListBucketResult>`)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		// Operações em nível de objeto
		switch r.Method {
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			ct := r.Header.Get("Content-Type")
			if ct == "" {
				ct = "audio/ogg"
			}
			mu.Lock()
			if objects[bucket] == nil {
				objects[bucket] = make(map[string]memoryObject)
			}
			objects[bucket][key] = memoryObject{
				data:         data,
				contentType:  ct,
				lastModified: time.Now().UTC(),
			}
			mu.Unlock()
			w.WriteHeader(http.StatusOK)

		case http.MethodHead:
			mu.RLock()
			obj, ok := objects[bucket][key]
			mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", obj.contentType)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj.data)))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			mu.RLock()
			obj, ok := objects[bucket][key]
			mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", obj.contentType)
			w.Header().Set("Last-Modified", obj.lastModified.Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(obj.data)

		case http.MethodDelete:
			mu.Lock()
			if objects[bucket] != nil {
				delete(objects[bucket], key)
			}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}

func TestMinIOObjectStore_S3Protocol(t *testing.T) {
	srv := newFakeS3Server(t)
	defer srv.Close()

	store, err := NewMinIOObjectStore(MinIOConfig{
		Endpoint:  srv.URL,
		PublicURL: "http://localhost:9000",
		Bucket:    DefaultMusicBucket,
		AccessKey: "radiosync",
		SecretKey: "radiosync",
	})
	if err != nil {
		t.Fatalf("NewMinIOObjectStore falhou: %v", err)
	}

	ctx := context.Background()

	// 1. Cria bucket "musicas" (HEAD -> 404, PUT -> 200) e verifica 2ª chamada (HEAD -> 200)
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket (1) falhou: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket (2) falhou: %v", err)
	}

	// 2. Upload de arquivo .opus
	opusData := []byte("OggS\x00\x02opus-audio-content-test")
	objectName := "faixa-01.opus"
	if err := store.Put(ctx, objectName, bytes.NewReader(opusData), int64(len(opusData)), "audio/ogg"); err != nil {
		t.Fatalf("Put falhou: %v", err)
	}

	// 3. Exists
	exists, err := store.Exists(ctx, objectName)
	if err != nil || !exists {
		t.Fatalf("Exists esperava true, obteve %v (err=%v)", exists, err)
	}

	// 4. Get stream
	rc, info, err := store.Get(ctx, objectName)
	if err != nil {
		t.Fatalf("Get falhou: %v", err)
	}
	readBytes, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || !bytes.Equal(readBytes, opusData) {
		t.Fatalf("dados lidos do MinIO diferem do original")
	}
	if info.ContentType != "audio/ogg" {
		t.Errorf("Content-Type esperado audio/ogg, obteve %s", info.ContentType)
	}

	// 5. PresignedGetURL
	presigned, err := store.PresignedGetURL(ctx, objectName, 10*time.Minute)
	if err != nil {
		t.Fatalf("PresignedGetURL falhou: %v", err)
	}
	if !strings.HasPrefix(presigned, "http://localhost:9000/musicas/faixa-01.opus?") ||
		!strings.Contains(presigned, "X-Amz-Algorithm=AWS4-HMAC-SHA256") ||
		!strings.Contains(presigned, "X-Amz-Signature=") {
		t.Errorf("URL pré-assinada inválida: %s", presigned)
	}

	// 6. ListKeys
	keys, err := store.ListKeys(ctx, "faixa-")
	if err != nil || len(keys) != 1 || keys[0] != objectName {
		t.Fatalf("ListKeys inesperado: %v (err=%v)", keys, err)
	}

	// 7. Delete
	if err := store.Delete(ctx, objectName); err != nil {
		t.Fatalf("Delete falhou: %v", err)
	}
	exists, _ = store.Exists(ctx, objectName)
	if exists {
		t.Errorf("objeto ainda existe após Delete")
	}
}
