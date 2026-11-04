package auth

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndValidateDJToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	token, err := GenerateDJToken("ABC123")
	if err != nil {
		t.Fatalf("erro ao gerar token: %v", err)
	}
	if !ValidateDJToken(token, "ABC123") {
		t.Fatal("token válido foi rejeitado")
	}
}

func TestValidateDJToken_OutraEstacao(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	token, _ := GenerateDJToken("ABC123")
	if ValidateDJToken(token, "XYZ999") {
		t.Fatal("token de uma estação não pode valer para outra")
	}
}

func TestValidateDJToken_SegredoDiferente(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-A")
	token, _ := GenerateDJToken("ABC123")

	t.Setenv("JWT_SECRET", "segredo-B")
	if ValidateDJToken(token, "ABC123") {
		t.Fatal("token assinado com outro segredo deve ser rejeitado")
	}
}

func TestValidateDJToken_SemClaimDJ(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "ABC123", "dj": false})
	s, _ := tok.SignedString([]byte("segredo-de-teste"))
	if ValidateDJToken(s, "ABC123") {
		t.Fatal("token sem dj=true deve ser rejeitado")
	}
}

func TestValidateDJToken_AlgNone(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "ABC123", "dj": true})
	s, _ := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if ValidateDJToken(s, "ABC123") {
		t.Fatal("token com alg=none deve ser rejeitado")
	}
}

func TestValidateDJToken_Lixo(t *testing.T) {
	for _, s := range []string{"", "abc", "a.b.c"} {
		if ValidateDJToken(s, "ABC123") {
			t.Fatalf("string inválida %q foi aceita", s)
		}
	}
}
