package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	s3Region  = "us-east-1"
	s3Service = "s3"
)

// MinIOConfig agrupa os parâmetros de conexão com o MinIO / S3 (ADR-0006).
type MinIOConfig struct {
	Endpoint   string // ex: http://minio:9000
	PublicURL  string // opcional: ex: http://localhost:9000 para URLs pré-assinadas externas
	Bucket     string // padrão: musicas
	AccessKey  string
	SecretKey  string
	HTTPClient *http.Client
}

// MinIOConfigFromEnv lê a configuração do MinIO das variáveis de ambiente do docker-compose.yml.
func MinIOConfigFromEnv() MinIOConfig {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = DefaultMusicBucket
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	if accessKey == "" {
		accessKey = os.Getenv("MINIO_ROOT_USER")
	}
	if accessKey == "" {
		accessKey = "radiosync"
	}
	secretKey := os.Getenv("S3_SECRET_KEY")
	if secretKey == "" {
		secretKey = os.Getenv("MINIO_ROOT_PASSWORD")
	}
	if secretKey == "" {
		secretKey = "radiosync123"
	}
	return MinIOConfig{
		Endpoint:  endpoint,
		PublicURL: os.Getenv("S3_PUBLIC_URL"),
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
}

// MinIOObjectStore implementa ObjectStore falando o protocolo S3 REST (AWS Signature V4)
// diretamente com o MinIO, sem dependências externas vulneráveis.
type MinIOObjectStore struct {
	endpoint  *url.URL
	publicURL *url.URL
	bucket    string
	accessKey string
	secretKey string
	client    *http.Client
}

func NewMinIOObjectStore(cfg MinIOConfig) (*MinIOObjectStore, error) {
	rawEndpoint := strings.TrimSpace(cfg.Endpoint)
	if rawEndpoint == "" {
		return nil, fmt.Errorf("endpoint S3/MinIO não informado")
	}
	if !strings.HasPrefix(rawEndpoint, "http://") && !strings.HasPrefix(rawEndpoint, "https://") {
		rawEndpoint = "http://" + rawEndpoint
	}
	u, err := url.Parse(strings.TrimRight(rawEndpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("endpoint S3 inválido (%s): %w", cfg.Endpoint, err)
	}

	var pubURL *url.URL
	if strings.TrimSpace(cfg.PublicURL) != "" {
		rawPub := strings.TrimSpace(cfg.PublicURL)
		if !strings.HasPrefix(rawPub, "http://") && !strings.HasPrefix(rawPub, "https://") {
			rawPub = "http://" + rawPub
		}
		if parsedPub, err := url.Parse(strings.TrimRight(rawPub, "/")); err == nil {
			pubURL = parsedPub
		}
	}

	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		bucket = DefaultMusicBucket
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	return &MinIOObjectStore{
		endpoint:  u,
		publicURL: pubURL,
		bucket:    bucket,
		accessKey: cfg.AccessKey,
		secretKey: cfg.SecretKey,
		client:    client,
	}, nil
}

func (s *MinIOObjectStore) objectPath(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	if cleanKey == "" {
		return "/" + s.bucket
	}
	parts := strings.Split(cleanKey, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/" + s.bucket + "/" + strings.Join(parts, "/")
}

func (s *MinIOObjectStore) EnsureBucket(ctx context.Context) error {
	headReq, err := s.newSignedRequest(ctx, http.MethodHead, s.objectPath(""), nil, nil, 0, "")
	if err != nil {
		return err
	}
	resp, err := s.client.Do(headReq)
	if err != nil {
		return fmt.Errorf("erro ao verificar bucket %s no MinIO: %w", s.bucket, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	putReq, err := s.newSignedRequest(ctx, http.MethodPut, s.objectPath(""), nil, nil, 0, "")
	if err != nil {
		return err
	}
	putResp, err := s.client.Do(putReq)
	if err != nil {
		return fmt.Errorf("erro ao criar bucket %s no MinIO: %w", s.bucket, err)
	}
	body, _ := io.ReadAll(putResp.Body)
	putResp.Body.Close()

	if putResp.StatusCode == http.StatusOK || putResp.StatusCode == http.StatusCreated || putResp.StatusCode == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("falha ao criar bucket %s: HTTP %d (%s)", s.bucket, putResp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *MinIOObjectStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("chave do objeto vazia")
	}
	if contentType == "" {
		contentType = "audio/ogg"
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("erro ao ler payload para upload no MinIO: %w", err)
	}

	req, err := s.newSignedRequest(ctx, http.MethodPut, s.objectPath(key), nil, bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("erro no Put(%s) no MinIO: %w", key, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("MinIO Put(%s) retornou HTTP %d: %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
}

func (s *MinIOObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	req, err := s.newSignedRequest(ctx, http.MethodGet, s.objectPath(key), nil, nil, 0, "")
	if err != nil {
		return nil, ObjectInfo{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("erro no Get(%s) no MinIO: %w", key, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, ObjectInfo{}, ErrObjectNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, ObjectInfo{}, fmt.Errorf("MinIO Get(%s) retornou HTTP %d: %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" {
		if strings.HasSuffix(strings.ToLower(key), ".opus") {
			ct = "audio/ogg"
		}
	}
	var lastMod time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if parsed, err := http.ParseTime(lm); err == nil {
			lastMod = parsed
		}
	}

	info := ObjectInfo{
		Key:          key,
		Size:         resp.ContentLength,
		ContentType:  ct,
		LastModified: lastMod,
	}
	return resp.Body, info, nil
}

func (s *MinIOObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	req, err := s.newSignedRequest(ctx, http.MethodHead, s.objectPath(key), nil, nil, 0, "")
	if err != nil {
		return false, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("MinIO Head(%s) retornou HTTP %d", key, resp.StatusCode)
}

func (s *MinIOObjectStore) Delete(ctx context.Context, key string) error {
	req, err := s.newSignedRequest(ctx, http.MethodDelete, s.objectPath(key), nil, nil, 0, "")
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("MinIO Delete(%s) retornou HTTP %d: %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
}

type listBucketResult struct {
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func (s *MinIOObjectStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	q := url.Values{}
	q.Set("list-type", "2")
	if prefix != "" {
		q.Set("prefix", prefix)
	}

	req, err := s.newSignedRequest(ctx, http.MethodGet, s.objectPath(""), q, nil, 0, "")
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MinIO ListKeys retornou HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed listBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("erro ao decodificar XML do MinIO: %w", err)
	}

	keys := make([]string, 0, len(parsed.Contents))
	for _, c := range parsed.Contents {
		if c.Key != "" {
			keys = append(keys, c.Key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// PresignedGetURL gera uma URL pré-assinada compatível com S3 Signature V4.
func (s *MinIOObjectStore) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("chave vazia para PresignedGetURL")
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	expSec := int(expiry.Seconds())
	if expSec < 1 {
		expSec = 60
	}
	if expSec > 604800 {
		expSec = 604800
	}

	base := s.endpoint
	if s.publicURL != nil {
		base = s.publicURL
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, s3Region, s3Service)

	canonicalURI := s.objectPath(key)
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", fmt.Sprintf("%s/%s", s.accessKey, credentialScope))
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(expSec))
	q.Set("X-Amz-SignedHeaders", "host")

	canonicalQuery := canonicalQueryString(q)
	canonicalHeaders := fmt.Sprintf("host:%s\n", base.Host)
	canonicalRequest := strings.Join([]string{
		http.MethodGet,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(s.secretKey, dateStamp, s3Region, s3Service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	finalURL := fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s", base.String(), canonicalURI, canonicalQuery, signature)
	return finalURL, nil
}

// ---------------------------------------------------------------- AWS Signature V4 helpers

func (s *MinIOObjectStore) newSignedRequest(
	ctx context.Context,
	method string,
	canonicalURI string,
	query url.Values,
	body io.Reader,
	contentLen int64,
	contentType string,
) (*http.Request, error) {
	const payloadHash = "UNSIGNED-PAYLOAD"

	fullURL := s.endpoint.String() + canonicalURI
	canonicalQuery := canonicalQueryString(query)
	if canonicalQuery != "" {
		fullURL += "?" + canonicalQuery
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}
	if contentLen > 0 {
		req.ContentLength = contentLen
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Host = s.endpoint.Host
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", s.endpoint.Host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, s3Region, s3Service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(s.secretKey, dateStamp, s3Region, s3Service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authHeader)
	return req, nil
}

func canonicalQueryString(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := append([]string{}, q[k]...)
		sort.Strings(vals)
		escapedKey := s3Escape(k)
		for _, v := range vals {
			parts = append(parts, escapedKey+"="+s3Escape(v))
		}
	}
	return strings.Join(parts, "&")
}

func s3Escape(str string) string {
	escaped := url.QueryEscape(str)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
