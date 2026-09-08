package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetMusicDir() string {
	dir := os.Getenv("MUSIC_DIR")
	if dir == "" {
		dir = "../musicas"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

func getCookiesFile() string {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	_ = os.MkdirAll(dir, 0755)
	cookiesPath := filepath.Join(dir, "cookies.txt")

	// Checagem do arquivo físico no disco
	if data, err := os.ReadFile(cookiesPath); err == nil && len(data) > 0 {
		strContent := strings.TrimSpace(string(data))
		// Se o arquivo no disco estiver em formato Base64, decodifica automaticamente para o formato Netscape
		if strings.HasPrefix(strContent, "IyBOZXRzY2FwZ") {
			decoded, err := base64.StdEncoding.DecodeString(strContent)
			if err == nil && len(decoded) > 0 {
				_ = os.WriteFile(cookiesPath, decoded, 0644)
				return cookiesPath
			}
		}
		return cookiesPath
	}
	return ""
}

type downloadStrategy struct {
	cookies string
	clients string
}

func DownloadYouTubeAudio(youtubeURL, outputPath string) (string, float64, error) {
	cookiesPath := getCookiesFile()
	denoPath, denoErr := exec.LookPath("deno")
	log.Printf("[youtube] PATH='%s'", os.Getenv("PATH"))
	log.Printf("[youtube] denoPath='%s' (err=%v)", denoPath, denoErr)
	log.Printf("[youtube] cookiesPath detectado: '%s' (DATA_DIR='%s')", cookiesPath, os.Getenv("DATA_DIR"))

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
	titleArgs = append(titleArgs, youtubeURL)

	titleCmd := exec.Command("yt-dlp", titleArgs...)
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
			downloadStrategy{cookies: cookiesPath, clients: ""},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=mweb,web" + potArg},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=web_embedded,mweb,web" + potArg},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=tv,mweb"},
		)
	}

	strategies = append(strategies,
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
		args = append(args, youtubeURL)

		log.Printf("[youtube] Tentando estratégia %d/%d (cookies=%v, clients='%s')", i+1, len(strategies), st.cookies != "", st.clients)
		cmd := exec.Command("yt-dlp", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err == nil {
			log.Printf("[youtube] Estratégia %d teve SUCESSO!", i+1)
			duration, _ := GetDuration(outputPath)
			return title, duration, nil
		} else {
			log.Printf("[youtube] Estratégia %d FALHOU: %v | stderr: %s", i+1, err, strings.TrimSpace(stderr.String()))
			lastErr = fmt.Errorf("%v | log: %s", err, strings.TrimSpace(stderr.String()))
		}
	}

	return "", 0, fmt.Errorf("falha no download do YouTube após testar estratégias: %v", lastErr)
}
