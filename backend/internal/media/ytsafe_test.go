package media

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateYouTubeURL(t *testing.T) {
	validas := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"  https://youtu.be/dQw4w9WgXcQ  ":            "https://youtu.be/dQw4w9WgXcQ",
		"http://youtube.com/watch?v=abc&list=x":       "http://youtube.com/watch?v=abc&list=x",
		"https://music.youtube.com/watch?v=abc":       "https://music.youtube.com/watch?v=abc",
		"https://m.youtube.com/watch?v=abc":           "https://m.youtube.com/watch?v=abc",
		"https://WWW.YouTube.com/watch?v=abc":         "https://WWW.YouTube.com/watch?v=abc",
	}
	for in, esperado := range validas {
		got, err := ValidateYouTubeURL(in)
		if err != nil || got != esperado {
			t.Errorf("%q: obtido (%q, %v), esperado %q", in, got, err, esperado)
		}
	}

	invalidas := []string{
		"",
		"--version", // vira opção do yt-dlp
		"-o/tmp/x",  // idem, forma curta
		"--ignore-config",
		"https://www.youtube.com/watch?v=a --verbose", // espaço: mais de um argumento
		"https://www.youtube.com/watch?v=a\n--version",
		"ftp://youtube.com/x",
		"file:///etc/passwd",
		"youtube.com/watch?v=abc", // sem esquema
		"https://evil.example/watch?v=abc",
		"https://youtube.com.evil.example/x", // sufixo
		"https://evilyoutube.com/x",
		"https://user:pass@youtube.com/x", // credenciais
		"https://youtube.com:8443/x",      // porta
	}
	for _, in := range invalidas {
		if got, err := ValidateYouTubeURL(in); !errors.Is(err, ErrInvalidYouTubeURL) {
			t.Errorf("%q deveria ser recusada, obtido (%q, %v)", in, got, err)
		}
	}
}

func TestYouTubeID(t *testing.T) {
	casos := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":          "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?list=PL1&v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ":        "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=10":                    "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":           "dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ":            "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ":             "dQw4w9WgXcQ",
		"https://www.youtube.com/playlist?list=PL123":          "",
		"https://www.youtube.com/watch?v=../../../etc":         "",
		"https://www.youtube.com/@canal":                       "",
		"https://youtu.be/":                                    "",
		"https://vimeo.com/123":                                "",
		"https://evil.example/watch?v=dQw4w9WgXcQ":             "",
		"não é url": "",
	}
	for in, want := range casos {
		if got := YouTubeID(in); got != want {
			t.Errorf("YouTubeID(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestIsVideoUnavailable(t *testing.T) {
	if !isVideoUnavailable("ERROR: [youtube] abc: Private video. Sign in if you've been granted access") {
		t.Error("vídeo privado é definitivo")
	}
	// Bloqueio anti-bot e rede são temporários: precisam de retry.
	for _, s := range []string{"Sign in to confirm you're not a bot", "HTTP Error 429", "Unable to download webpage", ""} {
		if isVideoUnavailable(s) {
			t.Errorf("%q é temporário", s)
		}
	}
}

// Nenhum download usa o cookies.txt original: o yt-dlp regrava o arquivo.
func TestCopyCookies(t *testing.T) {
	data := t.TempDir()
	t.Setenv("DATA_DIR", data)
	if got := copyCookies(t.TempDir()); got != "" {
		t.Fatalf("sem cookies.txt deveria devolver vazio, obtido %q", got)
	}
	const netscape = "# Netscape HTTP Cookie File\n"
	base64Netscape := base64.StdEncoding.EncodeToString([]byte(netscape))
	for _, content := range []string{netscape, base64Netscape} {
		if err := os.WriteFile(filepath.Join(data, "cookies.txt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		dst := t.TempDir()
		path := copyCookies(dst)
		if filepath.Dir(path) != dst {
			t.Fatalf("a cópia deveria ficar em %s, obtido %s", dst, path)
		}
		if got, _ := os.ReadFile(path); string(got) != netscape {
			t.Fatalf("cookies copiados: %q", got)
		}
		if original, _ := os.ReadFile(filepath.Join(data, "cookies.txt")); string(original) != content {
			t.Fatal("o original não pode mudar")
		}
	}
}

func TestAppendURL(t *testing.T) {
	args := appendURL([]string{"--quiet", "--no-playlist"}, "https://youtu.be/x")
	n := len(args)
	if n < 2 || args[n-2] != "--" || args[n-1] != "https://youtu.be/x" {
		t.Fatalf("a URL deve vir depois de \"--\": %q", args)
	}
}

// Com um yt-dlp falso no PATH que registra os argumentos: uma URL inválida
// nunca chega a executar o yt-dlp, e uma válida sempre vem depois de "--".
func TestDownloadYouTubeAudio_ArgumentosDoYtDlp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("yt-dlp falso usa shell script")
	}
	dir := t.TempDir()
	registro := filepath.Join(dir, "chamadas.log")
	fake := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done >> \"" + registro + "\"\necho '---' >> \"" + registro + "\"\necho 'titulo de teste'\n"
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DATA_DIR", t.TempDir())

	// 1) Valor malicioso: recusado antes de qualquer execução.
	if _, _, err := DownloadYouTubeAudio(context.Background(), "--version", filepath.Join(dir, "a.opus")); !errors.Is(err, ErrInvalidYouTubeURL) {
		t.Fatalf("esperado ErrInvalidYouTubeURL, obtido %v", err)
	}
	if _, err := os.Stat(registro); err == nil {
		t.Fatal("o yt-dlp não deveria ter sido executado para uma URL inválida")
	}

	// 2) URL válida: toda chamada termina com "--" e a URL.
	const videoURL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	title, _, err := DownloadYouTubeAudio(context.Background(), videoURL, filepath.Join(dir, "b.opus"))
	if err != nil {
		t.Fatalf("download com yt-dlp falso: %v", err)
	}
	if title != "titulo de teste" {
		t.Fatalf("título %q", title)
	}

	conteudo, err := os.ReadFile(registro)
	if err != nil {
		t.Fatal(err)
	}
	chamadas := strings.Split(strings.TrimSuffix(string(conteudo), "---\n"), "---\n")
	if len(chamadas) < 2 {
		t.Fatalf("esperado ao menos 2 chamadas (título e download), obtido %d", len(chamadas))
	}
	for i, c := range chamadas {
		args := strings.Split(strings.TrimSuffix(c, "\n"), "\n")
		n := len(args)
		if n < 2 || args[n-2] != "--" || args[n-1] != videoURL {
			t.Errorf("chamada %d: a URL deve ser o último argumento, depois de \"--\": %q", i+1, args)
		}
	}
}
