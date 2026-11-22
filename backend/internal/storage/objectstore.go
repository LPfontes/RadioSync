package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

const DefaultMusicBucket = "musicas"

var (
	ErrObjectNotFound = errors.New("objeto não encontrado no storage")
	ErrStationNotFound = errors.New("estação não encontrada")
	ErrTrackNotFound   = errors.New("faixa não encontrada")
)

// ObjectInfo contém os metadados básicos de um objeto armazenado no ObjectStore.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
}

// ObjectStore define o contrato de armazenamento de arquivos de áudio (.opus e originais),
// compartilhado entre o Station Service e o Media Worker (ADR-0002, ADR-0006).
type ObjectStore interface {
	// EnsureBucket garante que o bucket padrão (ex: "musicas") exista.
	EnsureBucket(ctx context.Context) error

	// Put grava um objeto no bucket com a chave e content-type informados.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Get retorna um stream de leitura do objeto e seus metadados.
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)

	// Exists verifica se a chave existe no bucket.
	Exists(ctx context.Context, key string) (bool, error)

	// Delete remove um objeto do bucket.
	Delete(ctx context.Context, key string) error

	// PresignedGetURL gera uma URL temporária pré-assinada para leitura direta do objeto.
	PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// ListKeys lista todas as chaves no bucket que começam com o prefixo informado.
	ListKeys(ctx context.Context, prefix string) ([]string, error)
}
