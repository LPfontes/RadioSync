package media

import "testing"

func TestIsSupported(t *testing.T) {
	aceitos := []string{".mp3", ".MP3", ".wav", ".ogg", ".flac", ".aac", ".m4a"}
	for _, ext := range aceitos {
		if !IsSupported(ext) {
			t.Errorf("%s deveria ser suportado", ext)
		}
	}
	recusados := []string{".exe", ".txt", ".zip", ""}
	for _, ext := range recusados {
		if IsSupported(ext) {
			t.Errorf("%s não deveria ser suportado", ext)
		}
	}
}

func TestConvertToOpus_FormatoInvalido(t *testing.T) {
	if err := ConvertToOpus("arquivo.exe", "saida.opus"); err == nil {
		t.Fatal("esperado erro para formato não suportado")
	}
}
