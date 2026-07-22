package media

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func DownloadYouTubeAudio(youtubeURL, outputPath string) (string, float64, error) {
	extractorArgs := "youtube:player_client=android,web,mweb,ios"

	// 1. Obter título do vídeo limpo
	titleArgs := []string{
		"--quiet",
		"--no-warnings",
		"--print", "%(title)s",
		"--no-playlist",
		"--extractor-args", extractorArgs,
		youtubeURL,
	}
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
		youtubeURL,
	}

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
