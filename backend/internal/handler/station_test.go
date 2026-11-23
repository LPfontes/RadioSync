package handler

import (
	"strings"
	"testing"
)

func TestGenerateCode(t *testing.T) {
	vistos := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		c := generateCode(6)
		if len(c) != 6 {
			t.Fatalf("tamanho esperado 6, obtido %d (%q)", len(c), c)
		}
		for _, r := range c {
			if !strings.ContainsRune(codeChars, r) {
				t.Fatalf("caractere fora do alfabeto: %q em %q", r, c)
			}
		}
		vistos[c] = true
	}
	// 32^6 ≈ 1 bilhão de combinações: 1000 códigos não devem colidir na prática.
	if len(vistos) < 995 {
		t.Fatalf("colisões demais: apenas %d códigos únicos em 1000", len(vistos))
	}
}
