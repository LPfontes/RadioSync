package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrInvalidMedia indica um arquivo que o FFmpeg não consegue ler: tentar de
// novo não resolve.
var ErrInvalidMedia = errors.New("arquivo de áudio inválido ou corrompido")

// command cria um processo que é encerrado quando ctx termina. O WaitDelay
// evita esperar para sempre por um filho que herdou o stdout/stderr.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// isInvalidMedia reconhece, no log do FFmpeg, um arquivo de entrada ilegível.
func isInvalidMedia(stderr string) bool {
	for _, s := range []string{
		"Invalid data found when processing input",
		"does not contain any stream",
		"matches no streams",
		"moov atom not found",
	} {
		if strings.Contains(stderr, s) {
			return true
		}
	}
	return false
}

var supportedFormats = map[string]bool{
	".mp3":  true,
	".mp4":  true,
	".webm": true,
	".wav":  true,
	".ogg":  true,
	".flac": true,
	".aac":  true,
	".m4a":  true,
}

func IsSupported(ext string) bool {
	return supportedFormats[strings.ToLower(ext)]
}

func ConvertToOpus(ctx context.Context, inputPath, outputPath string) error {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if !supportedFormats[ext] {
		return fmt.Errorf("formato não suportado: %s", ext)
	}

	args := []string{
		"-nostdin",
		// O conteúdo vem do usuário: um playlist renomeado para .mp3 não pode
		// fazer o FFmpeg abrir URLs da rede interna.
		"-protocol_whitelist", "file",
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "96k",
		"-vbr", "on",
		"-vn",
		"-y",
		outputPath,
	}

	cmd := command(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isInvalidMedia(stderr.String()) {
			return fmt.Errorf("%w: %s", ErrInvalidMedia, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("falha na conversão: %v | Log: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type FFProbeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func GetDuration(ctx context.Context, filePath string) (float64, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	}

	cmd := command(ctx, "ffprobe", args...)
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
