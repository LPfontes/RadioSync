package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Autenticação do painel administrativo (ADR-0005).
//
// O admin entra com a senha definida em ADMIN_PASSWORD e recebe um JWT com
// role=admin e expiração (ADMIN_TOKEN_TTL, padrão 8h). Sem ADMIN_PASSWORD as
// rotas de admin ficam desligadas: seguro por padrão.

const defaultAdminTokenTTL = 8 * time.Hour

// AdminEnabled informa se o painel admin está configurado.
func AdminEnabled() bool {
	return os.Getenv("ADMIN_PASSWORD") != ""
}

// CheckAdminPassword compara a senha em tempo constante. Os dois lados passam
// por SHA-256 antes da comparação para não vazar o tamanho da senha.
func CheckAdminPassword(password string) bool {
	expected := os.Getenv("ADMIN_PASSWORD")
	if expected == "" || password == "" {
		return false
	}
	a := sha256.Sum256([]byte(password))
	b := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// AdminTokenTTL lê ADMIN_TOKEN_TTL (ex.: "8h", "30m"); valores inválidos caem no padrão.
func AdminTokenTTL() time.Duration {
	if v := os.Getenv("ADMIN_TOKEN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultAdminTokenTTL
}

// GenerateAdminToken emite um JWT de admin e devolve também o instante de expiração.
func GenerateAdminToken() (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(AdminTokenTTL())
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "admin",
		"role": "admin",
		"jti":  uuid.New().String(),
		"iat":  now.Unix(),
		"exp":  exp.Unix(),
	})
	signed, err := token.SignedString(secret())
	return signed, exp, err
}

// ValidateAdminToken exige HS256, exp presente e válido e role=admin.
// Um token de DJ (sem role e sem exp) nunca passa aqui.
func ValidateAdminToken(tokenString string) bool {
	if tokenString == "" {
		return false
	}
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inválido: %v", token.Header["alg"])
		}
		return secret(), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}
	role, _ := claims["role"].(string)
	return role == "admin"
}
