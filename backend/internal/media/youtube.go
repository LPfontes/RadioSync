package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrVideoUnavailable indica um vídeo que nunca vai baixar (privado, removido):
// tentar de novo não resolve e não é falha do YouTube.
var ErrVideoUnavailable = errors.New("vídeo indisponível no YouTube")

func isVideoUnavailable(stderr string) bool {
	for _, s := range []string{
		"Private video",
		"This video is private",
		"This video has been removed",
		"account associated with this video has been terminated",
		"Join this channel to get access",
		"Incomplete YouTube ID",
	} {
		if strings.Contains(stderr, s) {
			return true
		}
	}
	return false
}

// copyCookies copia o cookies.txt para dir e devolve o caminho da cópia, ou ""
// se não houver cookies. O yt-dlp regrava o arquivo ao terminar: cada download
// usa a própria cópia, para que downloads em paralelo não corrompam o original.
func copyCookies(dir string) string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "cookies.txt")) // #nosec G304 -- diretório da configuração
	if err != nil || len(data) == 0 {
		return ""
	}
	// Se o arquivo estiver em Base64, decodifica para o formato Netscape
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "IyBOZXRzY2FwZ") {
		if decoded, err := base64.StdEncoding.DecodeString(content); err == nil && len(decoded) > 0 {
			data = decoded
		}
	}
	path := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("[youtube] não foi possível copiar cookies.txt: %v", err)
		return ""
	}
	return path
}

type downloadStrategy struct {
	cookies string
	clients string
}

func cleanEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		// Deno crashes with "Failed to open IPC channel from NODE_CHANNEL_FD (3): fd is not from BiPipe"
		// when spawned as a descendant of PM2/Node if NODE_CHANNEL_FD is present.
		if strings.HasPrefix(e, "NODE_CHANNEL_FD=") {
			continue
		}
		env = append(env, e)
	}
	return env
}

func DownloadYouTubeAudio(ctx context.Context, youtubeURL, outputPath string) (string, float64, error) {
	// Barreira contra injeção de opções no yt-dlp (ver ytsafe.go).
	youtubeURL, err := ValidateYouTubeURL(youtubeURL)
	if err != nil {
		return "", 0, err
	}

	cookiesDir, err := os.MkdirTemp("", "yt-cookies-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(cookiesDir)
	cookiesPath := copyCookies(cookiesDir)
	denoPath, denoErr := exec.LookPath("deno")
	log.Printf("[youtube] PATH='%s'", os.Getenv("PATH"))
	log.Printf("[youtube] denoPath='%s' (err=%v)", denoPath, denoErr)
	log.Printf("[youtube] cookies=%v (DATA_DIR='%s')", cookiesPath != "", os.Getenv("DATA_DIR"))

	// 1. Obter título do vídeo limpo
	titleArgs := []string{
		"--quiet",
		"--no-warnings",
		"--print", "%(title)s",
		"--no-playlist",
	}
	if cookiesPath != "" {
		titleArgs = append(titleArgs, "--cookies", cookiesPath)
	}
	titleArgs = appendURL(titleArgs, youtubeURL)

	titleCmd := command(ctx, "yt-dlp", titleArgs...)
	titleCmd.Env = cleanEnv()
	titleOut, _ := titleCmd.Output()
	title := strings.TrimSpace(string(titleOut))
	if title == "" {
		title = "Vídeo do YouTube"
	}
	log.Printf("[youtube] Título obtido: '%s'", title)

	outputTemplate := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".%(ext)s"

	potURL := os.Getenv("POT_PROVIDER_URL")
	if potURL == "" {
		potURL = "http://bgutil-provider:4416"
	}
	potArg := fmt.Sprintf(";youtubepot-bgutilhttp:base_url=%s", potURL)

	// 2. Definir estratégias em ordem de resiliência
	strategies := []downloadStrategy{}

	if cookiesPath != "" {
		strategies = append(strategies,
			downloadStrategy{cookies: cookiesPath, clients: fmt.Sprintf("youtubepot-bgutilhttp:base_url=%s", potURL)},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=mweb,web" + potArg},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=web_embedded,mweb,web" + potArg},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=tv,mweb"},
			downloadStrategy{cookies: cookiesPath, clients: ""},
		)
	}

	strategies = append(strategies,
		downloadStrategy{cookies: "", clients: fmt.Sprintf("youtubepot-bgutilhttp:base_url=%s", potURL)},
		downloadStrategy{cookies: "", clients: "youtube:player_client=mweb,web" + potArg},
		downloadStrategy{cookies: "", clients: "youtube:player_client=web_embedded,android,ios"},
		downloadStrategy{cookies: "", clients: "youtube:player_client=tv,mweb"},
		downloadStrategy{cookies: "", clients: ""},
	)

	var lastErr error
	for i, st := range strategies {
		args := []string{
			"-v",
			"-f", "ba/b",
			"-x",
			"--audio-format", "opus",
			"--audio-quality", "0",
			"-o", outputTemplate,
			"--no-playlist",
			"--no-warnings",
		}
		if st.clients != "" {
			args = append(args, "--extractor-args", st.clients)
		}
		if st.cookies != "" {
			args = append(args, "--cookies", st.cookies)
		}
		args = appendURL(args, youtubeURL)

		log.Printf("[youtube] Tentando estratégia %d/%d (cookies=%v, clients='%s')", i+1, len(strategies), st.cookies != "", st.clients)
		cmd := command(ctx, "yt-dlp", args...)
		cmd.Env = cleanEnv()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err == nil {
			log.Printf("[youtube] Estratégia %d teve SUCESSO!", i+1)
			duration, _ := GetDuration(ctx, outputPath)
			return title, duration, nil
		} else {
			if ctx.Err() != nil {
				return "", 0, ctx.Err()
			}
			if isVideoUnavailable(stderr.String()) {
				return "", 0, fmt.Errorf("%w: %s", ErrVideoUnavailable, strings.TrimSpace(stderr.String()))
			}
			log.Printf("[youtube] Estratégia %d FALHOU: %v | stderr: %s", i+1, err, strings.TrimSpace(stderr.String()))
			lastErr = fmt.Errorf("%v | log: %s", err, strings.TrimSpace(stderr.String()))
		}
	}

	return "", 0, fmt.Errorf("falha no download do YouTube após testar estratégias: %v", lastErr)
}
