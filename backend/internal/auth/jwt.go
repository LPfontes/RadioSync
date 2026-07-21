package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func secret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-in-production"
	}
	return []byte(s)
}

func GenerateDJToken(stationID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": stationID,
		"dj":  true,
		"id":  uuid.New().String(),
		"iat": time.Now().Unix(),
	})
	return token.SignedString(secret())
}

func ValidateDJToken(tokenString, stationID string) bool {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inválido: %v", token.Header["alg"])
		}
		return secret(), nil
	})
	if err != nil {
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return false
	}

	dj, ok := claims["dj"].(bool)
	if !ok || !dj {
		return false
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub != stationID {
		return false
	}

	return true
}
