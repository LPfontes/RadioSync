package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var supportedFormats = map[string]bool{
	".mp3":  true,
	".mp4":  true,
	".wav":  true,
	".ogg":  true,
	".flac": true,
	".aac":  true,
	".m4a":  true,
}

func IsSupported(ext string) bool {
	return supportedFormats[strings.ToLower(ext)]
}

func ConvertToOpus(inputPath, outputPath string) error {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if !supportedFormats[ext] {
		return fmt.Errorf("formato não suportado: %s", ext)
	}

	args := []string{
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "96k",
		"-vbr", "on",
		"-vn",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("falha na conversão: %v | Log: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type FFProbeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func GetDuration(filePath string) (float64, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	}

	cmd := exec.Command("ffprobe", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("falha ao obter duração: %v", err)
	}

	var out FFProbeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return 0, fmt.Errorf("erro ao parsear ffprobe: %v", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(out.Format.Duration, "%f", &duration); err != nil {
		return 0, fmt.Errorf("erro ao converter duração: %v", err)
	}

	return duration, nil
}
