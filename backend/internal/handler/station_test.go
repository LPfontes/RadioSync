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

func TestExtractYouTubeID(t *testing.T) {
	casos := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/abc123":       "abc123",
		"https://www.youtube.com/embed/xyz789":        "xyz789",
		"https://vimeo.com/123":                       "",
		"não é url":                                   "",
	}
	for in, want := range casos {
		if got := extractYouTubeID(in); got != want {
			t.Errorf("extractYouTubeID(%q) = %q, esperado %q", in, got, want)
		}
	}
}
