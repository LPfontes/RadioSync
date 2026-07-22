package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getCookiesFile() string {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	_ = os.MkdirAll(dir, 0755)
	cookiesPath := filepath.Join(dir, "cookies.txt")

	// 1. Suporte para variável de ambiente YOUTUBE_COOKIES_BASE64 (Ideal no Railway)
	if envBase64 := strings.TrimSpace(os.Getenv("YOUTUBE_COOKIES_BASE64")); envBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(envBase64)
		if err == nil && len(data) > 0 {
			_ = os.WriteFile(cookiesPath, data, 0644)
			return cookiesPath
		}
	}

	// 2. Suporte para variável de ambiente YOUTUBE_COOKIES (Texto bruto ou Base64)
	if envRaw := strings.TrimSpace(os.Getenv("YOUTUBE_COOKIES")); envRaw != "" {
		if strings.HasPrefix(envRaw, "IyBOZXRzY2FwZ") {
			decoded, err := base64.StdEncoding.DecodeString(envRaw)
			if err == nil && len(decoded) > 0 {
				_ = os.WriteFile(cookiesPath, decoded, 0644)
				return cookiesPath
			}
		}
		_ = os.WriteFile(cookiesPath, []byte(envRaw), 0644)
		return cookiesPath
	}

	// 3. Checagem do arquivo físico no disco
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

	outputTemplate := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".%(ext)s"

	// 2. Definir estratégias em ordem de resiliência no Railway / IPs de DataCenter
	strategies := []downloadStrategy{}

	if cookiesPath != "" {
		strategies = append(strategies,
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=web_embedded,mweb,web"},
			downloadStrategy{cookies: cookiesPath, clients: "youtube:player_client=tv,mweb"},
			downloadStrategy{cookies: cookiesPath, clients: ""},
		)
	}

	strategies = append(strategies,
		downloadStrategy{cookies: "", clients: "youtube:player_client=web_embedded,android,ios"},
		downloadStrategy{cookies: "", clients: "youtube:player_client=tv,mweb"},
		downloadStrategy{cookies: "", clients: ""},
	)

	var lastErr error
	for _, st := range strategies {
		args := []string{
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

		cmd := exec.Command("yt-dlp", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err == nil {
			// Download realizado com sucesso!
			duration, _ := GetDuration(outputPath)
			return title, duration, nil
		} else {
			lastErr = fmt.Errorf("%v | log: %s", err, strings.TrimSpace(stderr.String()))
		}
	}

	return "", 0, fmt.Errorf("falha no download do YouTube no Railway após testar estratégias: %v", lastErr)
}
