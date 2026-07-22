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
	cookiesPath := filepath.Join(dir, "cookies.txt")

	// 1. Suporte para variável de ambiente YOUTUBE_COOKIES_BASE64 (Ideal no Railway)
	if envBase64 := strings.TrimSpace(os.Getenv("YOUTUBE_COOKIES_BASE64")); envBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(envBase64)
		if err == nil && len(data) > 0 {
			_ = os.MkdirAll(dir, 0755)
			_ = os.WriteFile(cookiesPath, data, 0644)
			return cookiesPath
		}
	}

	// 2. Suporte para variável de ambiente YOUTUBE_COOKIES (Texto bruto)
	if envRaw := strings.TrimSpace(os.Getenv("YOUTUBE_COOKIES")); envRaw != "" {
		_ = os.MkdirAll(dir, 0755)
		_ = os.WriteFile(cookiesPath, []byte(envRaw), 0644)
		return cookiesPath
	}

	// 3. Checagem física do arquivo no disco
	if _, err := os.Stat(cookiesPath); err == nil {
		return cookiesPath
	}
	return ""
}

func DownloadYouTubeAudio(youtubeURL, outputPath string) (string, float64, error) {
	extractorArgs := "youtube:player_client=mweb,ios,tv_embedded,android"
	cookiesPath := getCookiesFile()

	// 1. Obter título do vídeo limpo
	titleArgs := []string{
		"--quiet",
		"--no-warnings",
		"--print", "%(title)s",
		"--no-playlist",
		"--extractor-args", extractorArgs,
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

	// 2. Definir template de saída adequado para conversão Opus
	outputTemplate := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".%(ext)s"

	// 3. Executar o download e conversão para .opus
	downloadArgs := []string{
		"-f", "ba/b",
		"-x",
		"--audio-format", "opus",
		"--audio-quality", "0",
		"--extractor-args", extractorArgs,
		"-o", outputTemplate,
		"--no-playlist",
	}
	if cookiesPath != "" {
		downloadArgs = append(downloadArgs, "--cookies", cookiesPath)
	}
	downloadArgs = append(downloadArgs, youtubeURL)

	cmd := exec.Command("yt-dlp", downloadArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("falha ao baixar do YouTube com yt-dlp: %v | Log: %s", err, strings.TrimSpace(stderr.String()))
	}

	// 4. Obter duração do arquivo .opus gerado
	duration, err := GetDuration(outputPath)
	if err != nil {
		duration = 0
	}

	return title, duration, nil
}
