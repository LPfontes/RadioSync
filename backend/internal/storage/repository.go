package storage

import (
	"context"

	"radio-backend/internal/model"
)

// Repository define o contrato de persistência relacional durável (PostgreSQL - ADR-0006)
// para estações, catálogo global de faixas, repositório por estação, itens da playlist,
// sugestões de ouvintes e cache de metadados do YouTube.
type Repository interface {
	// Estações
	CreateStation(ctx context.Context, st *model.Station) error
	GetStation(ctx context.Context, id string) (*model.Station, error)
	ListStations(ctx context.Context) ([]*model.Station, error)
	UpdateStationDJ(ctx context.Context, stationID, djToken string) error
	UpdateStationState(ctx context.Context, stationID string, state *model.PlaybackState, mixer []model.MixerChannel) error
	DeleteStation(ctx context.Context, id string) error

	// Faixas (Catálogo Global e Repositório da Estação)
	UpsertTrack(ctx context.Context, t model.Track) error
	GetTrack(ctx context.Context, trackID string) (model.Track, bool, error)
	ListLibraryTracks(ctx context.Context) ([]model.Track, error)
	AddTrackToStation(ctx context.Context, stationID string, t model.Track) error
	UpdateStationTrack(ctx context.Context, stationID string, t model.Track) error
	RemoveTrackFromStation(ctx context.Context, stationID, trackID string) error

	// Playlist
	ReplacePlaylist(ctx context.Context, stationID string, playlist []model.Track) error
	GetPlaylist(ctx context.Context, stationID string) ([]model.Track, error)

	// Sugestões
	ListSuggestions(ctx context.Context, stationID string) ([]model.SongSuggestion, error)
	SaveSuggestion(ctx context.Context, stationID string, sug model.SongSuggestion) error
	UpdateSuggestionStatus(ctx context.Context, stationID, suggestionID, status string) error
	DeleteSuggestion(ctx context.Context, stationID, suggestionID string) error
	ClearSuggestions(ctx context.Context, stationID string) error

	// Cache do YouTube (migração do yt_cache.json)
	GetYouTubeCache(ctx context.Context, videoID string) (model.Track, bool, error)
	SaveYouTubeCache(ctx context.Context, videoID, trackID string) error

	// Saúde e Ciclo de Vida
	Ping(ctx context.Context) error
	Close() error
}
