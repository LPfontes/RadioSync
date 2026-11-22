package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"radio-backend/internal/model"

	_ "github.com/lib/pq"
)

// PostgresRepository implementa Repository usando PostgreSQL 16 (ADR-0006).
type PostgresRepository struct {
	db *sql.DB
}

// DatabaseURLFromEnv retorna a string de conexão do Postgres configurada no ambiente.
func DatabaseURLFromEnv() string {
	u := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if u != "" {
		return u
	}
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "radiosync"
	}
	pass := os.Getenv("POSTGRES_PASSWORD")
	if pass == "" {
		pass = "radiosync"
	}
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = "radiosync"
	}
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost:5432"
	}
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, pass, host, dbName)
}

// NewPostgresRepository abre o pool de conexões com o PostgreSQL, valida conectividade
// e executa automaticamente as migrations versionadas embutidas em MigrationsFS.
func NewPostgresRepository(ctx context.Context, databaseURL string) (*PostgresRepository, error) {
	if strings.TrimSpace(databaseURL) == "" {
		databaseURL = DatabaseURLFromEnv()
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("falha no ping do postgres: %w", err)
	}

	if err := RunMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &PostgresRepository{db: db}, nil
}

// NewPostgresRepositoryFromDB cria um PostgresRepository a partir de um *sql.DB existente.
func NewPostgresRepositoryFromDB(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) DB() *sql.DB {
	return r.db
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *PostgresRepository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// ---------------------------------------------------------------- Estações

func (r *PostgresRepository) CreateStation(ctx context.Context, st *model.Station) error {
	st.RLock()
	id := st.ID
	dj := st.DJ
	var state model.PlaybackState
	if st.State != nil {
		state = *st.State
	}
	repoTracks := append([]model.Track{}, st.Repository...)
	playlistTracks := append([]model.Track{}, st.Playlist...)
	suggestions := append([]model.SongSuggestion{}, st.Suggestions...)
	mixer := append([]model.MixerChannel{}, st.Mixer...)
	st.RUnlock()

	if mixer == nil {
		mixer = make([]model.MixerChannel, 0)
	}
	mixerBytes, err := json.Marshal(mixer)
	if err != nil {
		mixerBytes = []byte("[]")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO stations (id, dj_token, is_playing, started_at, seek_offset, current_song, duration, mixer_json, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW())
		ON CONFLICT (id) DO UPDATE SET
			dj_token     = EXCLUDED.dj_token,
			is_playing   = EXCLUDED.is_playing,
			started_at   = EXCLUDED.started_at,
			seek_offset  = EXCLUDED.seek_offset,
			current_song = EXCLUDED.current_song,
			duration     = EXCLUDED.duration,
			mixer_json   = EXCLUDED.mixer_json,
			updated_at   = NOW()
	`, id, dj, state.IsPlaying, state.StartedAt, state.SeekOffset, state.CurrentSong, state.Duration, string(mixerBytes))
	if err != nil {
		return fmt.Errorf("erro ao salvar estação %s: %w", id, err)
	}

	// Garante faixas no catálogo global e vincula ao repositório da estação
	if _, err := tx.ExecContext(ctx, `DELETE FROM station_tracks WHERE station_id = $1`, id); err != nil {
		return err
	}
	for pos, t := range repoTracks {
		nt := normalizeTrack(t)
		if nt.ID == "" {
			continue
		}
		if err := upsertTrackTx(ctx, tx, nt); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO station_tracks (station_id, track_id, position)
			VALUES ($1, $2, $3)
			ON CONFLICT (station_id, track_id) DO UPDATE SET position = EXCLUDED.position
		`, id, nt.ID, pos)
		if err != nil {
			return err
		}
	}

	// Substitui itens da playlist mantendo a ordem exata
	if _, err := tx.ExecContext(ctx, `DELETE FROM playlist_items WHERE station_id = $1`, id); err != nil {
		return err
	}
	for pos, t := range playlistTracks {
		nt := normalizeTrack(t)
		if nt.ID == "" {
			continue
		}
		if err := upsertTrackTx(ctx, tx, nt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO playlist_items (station_id, track_id, position)
			VALUES ($1, $2, $3)
		`, id, nt.ID, pos); err != nil {
			return err
		}
	}

	// Substitui sugestões se fornecidas na criação/importação
	if _, err := tx.ExecContext(ctx, `DELETE FROM suggestions WHERE station_id = $1`, id); err != nil {
		return err
	}
	for _, sug := range suggestions {
		if sug.ID == "" {
			continue
		}
		status := sug.Status
		if status == "" {
			status = "pending"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO suggestions (id, station_id, track_id, title, url, suggested_by, created_at, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (id) DO UPDATE SET
				station_id   = EXCLUDED.station_id,
				track_id     = EXCLUDED.track_id,
				title        = EXCLUDED.title,
				url          = EXCLUDED.url,
				suggested_by = EXCLUDED.suggested_by,
				status       = EXCLUDED.status
		`, sug.ID, id, sug.TrackID, sug.Title, sug.URL, sug.SuggestedBy, sug.CreatedAt, status); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresRepository) GetStation(ctx context.Context, id string) (*model.Station, error) {
	var (
		stID        string
		djToken     string
		state       model.PlaybackState
		mixerRaw    []byte
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, dj_token, is_playing, started_at, seek_offset, current_song, duration, mixer_json
		FROM stations
		WHERE id = $1
	`, id).Scan(
		&stID,
		&djToken,
		&state.IsPlaying,
		&state.StartedAt,
		&state.SeekOffset,
		&state.CurrentSong,
		&state.Duration,
		&mixerRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrStationNotFound
		}
		return nil, err
	}

	mixer := make([]model.MixerChannel, 0)
	if len(mixerRaw) > 0 {
		_ = json.Unmarshal(mixerRaw, &mixer)
	}

	repoTracks, err := r.listStationRepositoryTracks(ctx, id)
	if err != nil {
		return nil, err
	}
	playlist, err := r.GetPlaylist(ctx, id)
	if err != nil {
		return nil, err
	}
	suggestions, err := r.ListSuggestions(ctx, id)
	if err != nil {
		return nil, err
	}

	return &model.Station{
		ID:          stID,
		DJ:          djToken,
		State:       &state,
		Repository:  repoTracks,
		Playlist:    playlist,
		Suggestions: suggestions,
		Mixer:       mixer,
	}, nil
}

func (r *PostgresRepository) ListStations(ctx context.Context) ([]*model.Station, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM stations ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]*model.Station, 0, len(ids))
	for _, id := range ids {
		st, err := r.GetStation(ctx, id)
		if err == nil && st != nil {
			result = append(result, st)
		}
	}
	return result, nil
}

func (r *PostgresRepository) UpdateStationDJ(ctx context.Context, stationID, djToken string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE stations SET dj_token = $2, updated_at = NOW() WHERE id = $1
	`, stationID, djToken)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrStationNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateStationState(ctx context.Context, stationID string, state *model.PlaybackState, mixer []model.MixerChannel) error {
	if state == nil {
		state = &model.PlaybackState{}
	}
	if mixer == nil {
		mixer = make([]model.MixerChannel, 0)
	}
	mixerBytes, err := json.Marshal(mixer)
	if err != nil {
		mixerBytes = []byte("[]")
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE stations
		SET is_playing = $2,
		    started_at = $3,
		    seek_offset = $4,
		    current_song = $5,
		    duration = $6,
		    mixer_json = $7::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, stationID, state.IsPlaying, state.StartedAt, state.SeekOffset, state.CurrentSong, state.Duration, string(mixerBytes))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrStationNotFound
	}
	return nil
}

func (r *PostgresRepository) DeleteStation(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM stations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrStationNotFound
	}
	return nil
}

// ---------------------------------------------------------------- Faixas (Catálogo e Repositório)

func upsertTrackTx(ctx context.Context, tx *sql.Tx, t model.Track) error {
	t = normalizeTrack(t)
	if t.ID == "" {
		return fmt.Errorf("track id vazio")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO tracks (id, title, filename, url, duration, category, theme, cover_url, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET
			title      = CASE WHEN EXCLUDED.title <> '' AND NOT (EXCLUDED.title LIKE 'Música %' AND tracks.title <> '') THEN EXCLUDED.title ELSE tracks.title END,
			filename   = CASE WHEN EXCLUDED.filename <> '' THEN EXCLUDED.filename ELSE tracks.filename END,
			url        = CASE WHEN EXCLUDED.url <> '' THEN EXCLUDED.url ELSE tracks.url END,
			duration   = CASE WHEN EXCLUDED.duration > 0 THEN EXCLUDED.duration ELSE tracks.duration END,
			category   = CASE WHEN EXCLUDED.category <> '' THEN EXCLUDED.category ELSE tracks.category END,
			theme      = CASE WHEN EXCLUDED.theme <> '' THEN EXCLUDED.theme ELSE tracks.theme END,
			cover_url  = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE tracks.cover_url END,
			updated_at = NOW()
	`, t.ID, t.Title, t.Filename, t.URL, t.Duration, t.Category, t.Theme, t.CoverURL)
	return err
}

func (r *PostgresRepository) UpsertTrack(ctx context.Context, t model.Track) error {
	t = normalizeTrack(t)
	if t.ID == "" {
		return fmt.Errorf("track id vazio")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tracks (id, title, filename, url, duration, category, theme, cover_url, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET
			title      = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE tracks.title END,
			filename   = CASE WHEN EXCLUDED.filename <> '' THEN EXCLUDED.filename ELSE tracks.filename END,
			url        = CASE WHEN EXCLUDED.url <> '' THEN EXCLUDED.url ELSE tracks.url END,
			duration   = CASE WHEN EXCLUDED.duration > 0 THEN EXCLUDED.duration ELSE tracks.duration END,
			category   = EXCLUDED.category,
			theme      = EXCLUDED.theme,
			cover_url  = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE tracks.cover_url END,
			updated_at = NOW()
	`, t.ID, t.Title, t.Filename, t.URL, t.Duration, t.Category, t.Theme, t.CoverURL)
	return err
}

func (r *PostgresRepository) GetTrack(ctx context.Context, trackID string) (model.Track, bool, error) {
	cleanID := strings.TrimSuffix(trackID, ".opus")
	var t model.Track
	err := r.db.QueryRowContext(ctx, `
		SELECT id, title, filename, url, duration, category, theme, cover_url
		FROM tracks
		WHERE id = $1 OR filename = $2 OR filename = $3
		LIMIT 1
	`, cleanID, trackID, cleanID+".opus").Scan(
		&t.ID, &t.Title, &t.Filename, &t.URL, &t.Duration, &t.Category, &t.Theme, &t.CoverURL,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Track{}, false, nil
		}
		return model.Track{}, false, err
	}
	return t, true, nil
}

func (r *PostgresRepository) ListLibraryTracks(ctx context.Context) ([]model.Track, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, filename, url, duration, category, theme, cover_url
		FROM tracks
		ORDER BY created_at DESC, title ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tracks := make([]model.Track, 0)
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Filename, &t.URL, &t.Duration, &t.Category, &t.Theme, &t.CoverURL); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

func (r *PostgresRepository) listStationRepositoryTracks(ctx context.Context, stationID string) ([]model.Track, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.title, t.filename, t.url, t.duration, t.category, t.theme, t.cover_url
		FROM station_tracks st
		JOIN tracks t ON t.id = st.track_id
		WHERE st.station_id = $1
		ORDER BY st.position ASC, st.added_at ASC
	`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tracks := make([]model.Track, 0)
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Filename, &t.URL, &t.Duration, &t.Category, &t.Theme, &t.CoverURL); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

func (r *PostgresRepository) AddTrackToStation(ctx context.Context, stationID string, t model.Track) error {
	t = normalizeTrack(t)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE id = $1)`, stationID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrStationNotFound
	}

	if err := upsertTrackTx(ctx, tx, t); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO station_tracks (station_id, track_id, position)
		VALUES ($1, $2, COALESCE((SELECT MAX(position) + 1 FROM station_tracks WHERE station_id = $1), 0))
		ON CONFLICT (station_id, track_id) DO NOTHING
	`, stationID, t.ID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PostgresRepository) UpdateStationTrack(ctx context.Context, stationID string, t model.Track) error {
	t = normalizeTrack(t)
	var linked bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM station_tracks WHERE station_id = $1 AND track_id = $2)
	`, stationID, t.ID).Scan(&linked)
	if err != nil {
		return err
	}
	if !linked {
		var stExists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE id = $1)`, stationID).Scan(&stExists); err == nil && !stExists {
			return ErrStationNotFound
		}
		return ErrTrackNotFound
	}
	return r.UpsertTrack(ctx, t)
}

func (r *PostgresRepository) RemoveTrackFromStation(ctx context.Context, stationID, trackID string) error {
	cleanID := strings.TrimSuffix(trackID, ".opus")
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var stExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE id = $1)`, stationID).Scan(&stExists); err != nil {
		return err
	}
	if !stExists {
		return ErrStationNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM station_tracks WHERE station_id = $1 AND (track_id = $2 OR track_id = $3)`, stationID, trackID, cleanID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM playlist_items WHERE station_id = $1 AND (track_id = $2 OR track_id = $3)`, stationID, trackID, cleanID); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- Playlist

func (r *PostgresRepository) ReplacePlaylist(ctx context.Context, stationID string, playlist []model.Track) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE id = $1)`, stationID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrStationNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM playlist_items WHERE station_id = $1`, stationID); err != nil {
		return err
	}

	for pos, t := range playlist {
		nt := normalizeTrack(t)
		if nt.ID == "" {
			continue
		}
		if err := upsertTrackTx(ctx, tx, nt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO playlist_items (station_id, track_id, position)
			VALUES ($1, $2, $3)
		`, stationID, nt.ID, pos); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresRepository) GetPlaylist(ctx context.Context, stationID string) ([]model.Track, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.title, t.filename, t.url, t.duration, t.category, t.theme, t.cover_url
		FROM playlist_items pi
		JOIN tracks t ON t.id = pi.track_id
		WHERE pi.station_id = $1
		ORDER BY pi.position ASC, pi.id ASC
	`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	playlist := make([]model.Track, 0)
	for rows.Next() {
		var t model.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Filename, &t.URL, &t.Duration, &t.Category, &t.Theme, &t.CoverURL); err != nil {
			return nil, err
		}
		playlist = append(playlist, t)
	}
	return playlist, rows.Err()
}

// ---------------------------------------------------------------- Sugestões

func (r *PostgresRepository) ListSuggestions(ctx context.Context, stationID string) ([]model.SongSuggestion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, track_id, title, url, suggested_by, created_at, status
		FROM suggestions
		WHERE station_id = $1
		ORDER BY created_at ASC
	`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sugs := make([]model.SongSuggestion, 0)
	for rows.Next() {
		var s model.SongSuggestion
		if err := rows.Scan(&s.ID, &s.TrackID, &s.Title, &s.URL, &s.SuggestedBy, &s.CreatedAt, &s.Status); err != nil {
			return nil, err
		}
		sugs = append(sugs, s)
	}
	return sugs, rows.Err()
}

func (r *PostgresRepository) SaveSuggestion(ctx context.Context, stationID string, sug model.SongSuggestion) error {
	status := sug.Status
	if status == "" {
		status = "pending"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO suggestions (id, station_id, track_id, title, url, suggested_by, created_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			track_id     = EXCLUDED.track_id,
			title        = EXCLUDED.title,
			url          = EXCLUDED.url,
			suggested_by = EXCLUDED.suggested_by,
			status       = EXCLUDED.status
	`, sug.ID, stationID, sug.TrackID, sug.Title, sug.URL, sug.SuggestedBy, sug.CreatedAt, status)
	return err
}

func (r *PostgresRepository) UpdateSuggestionStatus(ctx context.Context, stationID, suggestionID, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE suggestions SET status = $3 WHERE station_id = $1 AND id = $2
	`, stationID, suggestionID, status)
	return err
}

func (r *PostgresRepository) DeleteSuggestion(ctx context.Context, stationID, suggestionID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM suggestions WHERE station_id = $1 AND id = $2
	`, stationID, suggestionID)
	return err
}

func (r *PostgresRepository) ClearSuggestions(ctx context.Context, stationID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM suggestions WHERE station_id = $1
	`, stationID)
	return err
}

// ---------------------------------------------------------------- Cache do YouTube

func (r *PostgresRepository) GetYouTubeCache(ctx context.Context, videoID string) (model.Track, bool, error) {
	var t model.Track
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.title, t.filename, t.url, t.duration, t.category, t.theme, t.cover_url
		FROM youtube_cache yc
		JOIN tracks t ON t.id = yc.track_id
		WHERE yc.video_id = $1
	`, videoID).Scan(&t.ID, &t.Title, &t.Filename, &t.URL, &t.Duration, &t.Category, &t.Theme, &t.CoverURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Track{}, false, nil
		}
		return model.Track{}, false, err
	}
	return t, true, nil
}

func (r *PostgresRepository) SaveYouTubeCache(ctx context.Context, videoID, trackID string) error {
	cleanID := strings.TrimSuffix(trackID, ".opus")
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO youtube_cache (video_id, track_id)
		VALUES ($1, $2)
		ON CONFLICT (video_id) DO UPDATE SET track_id = EXCLUDED.track_id
	`, videoID, cleanID)
	return err
}
