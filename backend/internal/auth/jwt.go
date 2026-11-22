package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------- segredo

// MinSecretLen é o tamanho mínimo do JWT_SECRET: 32 bytes (256 bits), o
// tamanho da saída do SHA-256 usado pelo HS256.
const MinSecretLen = 32

// Valores que já apareceram no código ou no .env.example: nunca são aceitos.
var knownInsecureSecrets = map[string]bool{
	"dev-secret-change-in-production":                  true,
	"coloque_uma_chave_secreta_longa_e_aleatoria_aqui": true,
}

var (
	ephemeralOnce   sync.Once
	ephemeralSecret []byte
)

// secret devolve o JWT_SECRET. Sem a variável, usa um segredo aleatório
// gerado uma vez por processo: nunca um valor fixo conhecido, que permitiria
// forjar tokens de DJ de qualquer estação.
func secret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	ephemeralOnce.Do(func() {
		b := make([]byte, MinSecretLen)
		if _, err := rand.Read(b); err != nil {
			panic("auth: falha ao gerar segredo aleatório: " + err.Error())
		}
		ephemeralSecret = b
	})
	return ephemeralSecret
}

// ErrWeakSecret indica um JWT_SECRET inseguro.
var ErrWeakSecret = errors.New("JWT_SECRET inseguro")

// CheckSecret valida o JWT_SECRET na subida do servidor.
//
//   - não definido: aceito com aviso (segredo aleatório temporário; os tokens
//     deixam de valer a cada reinício). Bom para desenvolvimento local.
//   - valor de exemplo conhecido ou menor que MinSecretLen: erro, o servidor
//     não deve subir.
func CheckSecret() (warning string, err error) {
	s := os.Getenv("JWT_SECRET")
	switch {
	case s == "":
		return "JWT_SECRET não definido: usando um segredo aleatório temporário, os tokens deixam de valer a cada reinício. Defina JWT_SECRET fora do ambiente local (openssl rand -hex 32).", nil
	case knownInsecureSecrets[s]:
		return "", fmt.Errorf("%w: está com um valor de exemplo; gere um com: openssl rand -hex 32", ErrWeakSecret)
	case len(s) < MinSecretLen:
		return "", fmt.Errorf("%w: tem %d caracteres, o mínimo é %d; gere um com: openssl rand -hex 32", ErrWeakSecret, len(s), MinSecretLen)
	}
	return "", nil
}

func parseHS256(tokenString string) (jwt.MapClaims, bool) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inválido: %v", token.Header["alg"])
		}
		return secret(), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	return claims, ok
}

// ---------------------------------------------------------------- token de DJ

const defaultDJTokenTTL = 24 * time.Hour

// DJTokenTTL lê DJ_TOKEN_TTL (ex.: "24h", "12h"); valores inválidos caem no padrão.
// A expiração é deslizante: GET /stations/{id} devolve um token novo para
// quem já é DJ, então só expira quem fica esse tempo sem abrir a sala.
func DJTokenTTL() time.Duration {
	if v := os.Getenv("DJ_TOKEN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultDJTokenTTL
}

func GenerateDJToken(stationID string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": stationID,
		"dj":  true,
		"id":  uuid.New().String(),
		"iat": now.Unix(),
		"exp": now.Add(DJTokenTTL()).Unix(),
	})
	return token.SignedString(secret())
}

// ValidateDJToken exige HS256, exp válido, dj=true e sub igual à estação.
// Tokens antigos, emitidos sem exp, deixam de valer.
func ValidateDJToken(tokenString, stationID string) bool {
	claims, ok := parseHS256(tokenString)
	if !ok {
		return false
	}

	dj, ok := claims["dj"].(bool)
	if !ok || !dj {
		return false
	}

	sub, ok := claims["sub"].(string)
	return ok && sub == stationID
}
