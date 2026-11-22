package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestValidateDJToken_Expirado(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "ABC123", "dj": true, "exp": time.Now().Add(-time.Minute).Unix(),
	})
	s, _ := tok.SignedString([]byte("segredo-de-teste"))
	if ValidateDJToken(s, "ABC123") {
		t.Fatal("token de DJ expirado deve ser rejeitado")
	}
}

// Tokens emitidos antes desta mudança não tinham exp e deixam de valer.
func TestValidateDJToken_LegadoSemExp(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "ABC123", "dj": true})
	s, _ := tok.SignedString([]byte("segredo-de-teste"))
	if ValidateDJToken(s, "ABC123") {
		t.Fatal("token de DJ sem exp deve ser rejeitado")
	}
}

func TestGenerateDJToken_TemExp(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")
	t.Setenv("DJ_TOKEN_TTL", "2h")

	s, _ := GenerateDJToken("ABC123")
	tok, _, err := jwt.NewParser().ParseUnverified(s, jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	exp, err := tok.Claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatal("token de DJ sem exp")
	}
	if d := time.Until(exp.Time); d < 119*time.Minute || d > 2*time.Hour {
		t.Fatalf("exp deveria ser ~2h, obtido %v", d)
	}
}

func TestDJTokenTTL(t *testing.T) {
	t.Setenv("DJ_TOKEN_TTL", "")
	if DJTokenTTL() != 24*time.Hour {
		t.Fatalf("padrão deveria ser 24h, obtido %v", DJTokenTTL())
	}
	for _, v := range []string{"abc", "-1h", "0"} {
		t.Setenv("DJ_TOKEN_TTL", v)
		if DJTokenTTL() != 24*time.Hour {
			t.Fatalf("valor inválido %q deveria cair no padrão", v)
		}
	}
}

func TestCheckSecret(t *testing.T) {
	forte := strings.Repeat("a", MinSecretLen)

	t.Setenv("JWT_SECRET", forte)
	if w, err := CheckSecret(); err != nil || w != "" {
		t.Fatalf("segredo forte: warning=%q err=%v", w, err)
	}

	t.Setenv("JWT_SECRET", "")
	if w, err := CheckSecret(); err != nil || w == "" {
		t.Fatalf("sem segredo deveria subir com aviso: warning=%q err=%v", w, err)
	}

	for _, fraco := range []string{
		"dev-secret-change-in-production",
		"coloque_uma_chave_secreta_longa_e_aleatoria_aqui",
		"ci",
		forte[:MinSecretLen-1],
	} {
		t.Setenv("JWT_SECRET", fraco)
		if _, err := CheckSecret(); !errors.Is(err, ErrWeakSecret) {
			t.Errorf("segredo %q deveria ser recusado, err=%v", fraco, err)
		}
	}
}

// Sem JWT_SECRET o segredo é aleatório (não o antigo valor fixo) e estável
// durante o processo.
func TestSecret_SemVariavel(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	tok, _ := GenerateDJToken("ABC123")
	if !ValidateDJToken(tok, "ABC123") {
		t.Fatal("token gerado com o segredo temporário deveria valer no mesmo processo")
	}

	forjado := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "ABC123", "dj": true, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := forjado.SignedString([]byte("dev-secret-change-in-production"))
	if ValidateDJToken(s, "ABC123") {
		t.Fatal("token assinado com o antigo segredo padrão não pode ser aceito")
	}
}
