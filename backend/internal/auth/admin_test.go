package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAdminEnabled(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "")
	if AdminEnabled() {
		t.Fatal("sem ADMIN_PASSWORD o admin deve ficar desabilitado")
	}
	t.Setenv("ADMIN_PASSWORD", "s3nha")
	if !AdminEnabled() {
		t.Fatal("com ADMIN_PASSWORD o admin deve ficar habilitado")
	}
}

func TestCheckAdminPassword(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "senha-correta")

	if !CheckAdminPassword("senha-correta") {
		t.Fatal("senha correta foi rejeitada")
	}
	for _, p := range []string{"", "senha-errada", "senha-corret", "senha-correta "} {
		if CheckAdminPassword(p) {
			t.Fatalf("senha %q não deveria ser aceita", p)
		}
	}
}

func TestCheckAdminPassword_Desabilitado(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "")
	if CheckAdminPassword("") {
		t.Fatal("com admin desabilitado nenhuma senha (nem vazia) pode passar")
	}
}

func TestGenerateAndValidateAdminToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")
	t.Setenv("ADMIN_TOKEN_TTL", "")

	token, exp, err := GenerateAdminToken()
	if err != nil {
		t.Fatalf("erro ao gerar token: %v", err)
	}
	if !ValidateAdminToken(token) {
		t.Fatal("token de admin válido foi rejeitado")
	}
	if d := time.Until(exp); d < 7*time.Hour || d > 8*time.Hour+time.Minute {
		t.Fatalf("expiração padrão deveria ser ~8h, obtido %v", d)
	}
}

func TestAdminTokenTTL(t *testing.T) {
	t.Setenv("ADMIN_TOKEN_TTL", "30m")
	if AdminTokenTTL() != 30*time.Minute {
		t.Fatalf("esperado 30m, obtido %v", AdminTokenTTL())
	}
	for _, v := range []string{"abc", "-1h", "0"} {
		t.Setenv("ADMIN_TOKEN_TTL", v)
		if AdminTokenTTL() != defaultAdminTokenTTL {
			t.Fatalf("valor inválido %q deveria cair no padrão", v)
		}
	}
}

func TestValidateAdminToken_Expirado(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "admin",
		"exp":  time.Now().Add(-time.Minute).Unix(),
	})
	signed, _ := token.SignedString([]byte("segredo-de-teste"))
	if ValidateAdminToken(signed) {
		t.Fatal("token de admin expirado não pode ser aceito")
	}
}

func TestValidateAdminToken_SemExp(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"role": "admin"})
	signed, _ := token.SignedString([]byte("segredo-de-teste"))
	if ValidateAdminToken(signed) {
		t.Fatal("token de admin sem exp não pode ser aceito")
	}
}

func TestValidateAdminToken_SegredoDiferente(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-A")
	token, _, _ := GenerateAdminToken()

	t.Setenv("JWT_SECRET", "segredo-B")
	if ValidateAdminToken(token) {
		t.Fatal("token assinado com outro segredo não pode ser aceito")
	}
}

// Separação de papéis: token de DJ não vira admin e token de admin não vira DJ.
func TestSeparacaoDePapeis(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	dj, _ := GenerateDJToken("ABC123")
	if ValidateAdminToken(dj) {
		t.Fatal("token de DJ não pode ser aceito como admin")
	}

	admin, _, _ := GenerateAdminToken()
	if ValidateDJToken(admin, "ABC123") {
		t.Fatal("token de admin não pode ser aceito como DJ")
	}
}

func TestValidateAdminToken_AlgNone(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste")

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if ValidateAdminToken(signed) {
		t.Fatal("token com alg=none não pode ser aceito")
	}
}
