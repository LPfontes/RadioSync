package media

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Validação da URL antes de chamar o yt-dlp.
//
// A URL chega de quem é DJ (ou de uma sugestão aprovada) e vira argumento do
// yt-dlp. Sem validação, um valor que começa com "-" é interpretado como opção
// do yt-dlp, e algumas opções leem arquivos de configuração ou executam
// comandos. Por isso há duas barreiras:
//  1. ValidateYouTubeURL só aceita http(s) em domínios do YouTube;
//  2. appendURL coloca "--" antes da URL, encerrando as opções.

// ErrInvalidYouTubeURL indica uma URL que não é do YouTube.
var ErrInvalidYouTubeURL = errors.New("URL do YouTube inválida")

var youtubeHosts = map[string]bool{
	"youtube.com":       true,
	"www.youtube.com":   true,
	"m.youtube.com":     true,
	"music.youtube.com": true,
	"youtu.be":          true,
	"www.youtu.be":      true,
}

// ValidateYouTubeURL aceita só http(s), sem usuário/senha nem porta, em um
// host do YouTube, e devolve a URL normalizada.
func ValidateYouTubeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, " \t\r\n") {
		return "", ErrInvalidYouTubeURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrInvalidYouTubeURL
	}
	if u.User != nil || u.Port() != "" || !youtubeHosts[strings.ToLower(u.Hostname())] {
		return "", ErrInvalidYouTubeURL
	}
	return u.String(), nil
}

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// YouTubeID extrai o ID do vídeo de uma URL do YouTube, ou "" se não houver
// (playlist, canal, outro site). O ID identifica o vídeo no cache.
func YouTubeID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !youtubeHosts[strings.ToLower(u.Hostname())] {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	id := u.Query().Get("v")
	switch {
	case strings.HasSuffix(strings.ToLower(u.Hostname()), "youtu.be"):
		id = parts[0]
	case id == "" && len(parts) >= 2:
		switch parts[0] {
		case "shorts", "embed", "v", "live":
			id = parts[1]
		}
	}
	if !videoIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// WatchURL devolve a URL canônica de um vídeo, sem playlist nem parâmetros.
func WatchURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + videoID
}

// appendURL termina a lista de opções do yt-dlp com "--" e só então a URL,
// para que ela nunca seja lida como opção.
func appendURL(args []string, videoURL string) []string {
	return append(args, "--", videoURL)
}
