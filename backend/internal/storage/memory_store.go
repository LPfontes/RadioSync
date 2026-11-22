package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"radio-backend/internal/model"
)

// ---------------------------------------------------------------- MemoryObjectStore

type memoryObject struct {
	data         []byte
	contentType  string
	lastModified time.Time
}

// MemoryObjectStore implementa ObjectStore em memória para testes unitários e de integração rápidos.
type MemoryObjectStore struct {
	mu          sync.RWMutex
	bucket      string
	bucketReady bool
	baseURL     string
	objects     map[string]memoryObject
}

func NewMemoryObjectStore(bucket string) *MemoryObjectStore {
	if bucket == "" {
		bucket = DefaultMusicBucket
	}
	return &MemoryObjectStore{
		bucket:  bucket,
		baseURL: "http://minio.local:9000",
		objects: make(map[string]memoryObject),
	}
}

func (m *MemoryObjectStore) EnsureBucket(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bucketReady = true
	return nil
}

func (m *MemoryObjectStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "audio/ogg"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memoryObject{
		data:         data,
		contentType:  contentType,
		lastModified: time.Now().UTC(),
	}
	return nil
}

func (m *MemoryObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	m.mu.RLock()
	obj, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return nil, ObjectInfo{}, ErrObjectNotFound
	}
	info := ObjectInfo{
		Key:          key,
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		LastModified: obj.lastModified,
	}
	return io.NopCloser(bytes.NewReader(obj.data)), info, nil
}

func (m *MemoryObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	_, ok := m.objects[key]
	m.mu.RUnlock()
	return ok, nil
}

func (m *MemoryObjectStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	delete(m.objects, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryObjectStore) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	m.mu.RLock()
	_, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return "", ErrObjectNotFound
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	u := fmt.Sprintf("%s/%s/%s?X-Amz-Expires=%d", m.baseURL, m.bucket, url.PathEscape(key), int(expiry.Seconds()))
	return u, nil
}

func (m *MemoryObjectStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// ---------------------------------------------------------------- MemoryRepository

// MemoryRepository implementa Repository em memória com semântica idêntica ao PostgresRepository.
type MemoryRepository struct {
	mu       sync.RWMutex
	down     bool
	stations map[string]*model.Station
	tracks   map[string]model.Track
	ytCache  map[string]string // videoID -> trackID
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		stations: make(map[string]*model.Station),
		tracks:   make(map[string]model.Track),
		ytCache:  make(map[string]string),
	}
}

// SetAvailable permite simular queda do banco de dados nos testes de resiliência (RES-05).
func (r *MemoryRepository) SetAvailable(available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.down = !available
}

func (r *MemoryRepository) checkAvailable() error {
	if r.down {
		return fmt.Errorf("postgres indisponível: connection refused")
	}
	return nil
}

func cloneStation(st *model.Station) *model.Station {
	st.RLock()
	defer st.RUnlock()
	var stateCopy model.PlaybackState
	if st.State != nil {
		stateCopy = *st.State
	}
	cp := &model.Station{
		ID:          st.ID,
		DJ:          st.DJ,
		State:       &stateCopy,
		Repository:  append([]model.Track{}, st.Repository...),
		Playlist:    append([]model.Track{}, st.Playlist...),
		Suggestions: append([]model.SongSuggestion{}, st.Suggestions...),
		Mixer:       append([]model.MixerChannel{}, st.Mixer...),
	}
	return cp
}

func normalizeTrack(t model.Track) model.Track {
	if t.ID == "" && t.Filename != "" {
		t.ID = strings.TrimSuffix(t.Filename, filepath.Ext(t.Filename))
	}
	if t.Filename == "" && t.ID != "" {
		if strings.HasSuffix(t.ID, ".opus") {
			t.Filename = t.ID
			t.ID = strings.TrimSuffix(t.ID, ".opus")
		} else {
			t.Filename = t.ID + ".opus"
		}
	}
	if t.URL == "" && t.Filename != "" {
		t.URL = "/musicas/" + t.Filename
	}
	return t
}

func (r *MemoryRepository) CreateStation(ctx context.Context, st *model.Station) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	cp := cloneStation(st)
	for i, t := range cp.Repository {
		nt := normalizeTrack(t)
		cp.Repository[i] = nt
		r.tracks[nt.ID] = nt
	}
	for i, t := range cp.Playlist {
		nt := normalizeTrack(t)
		cp.Playlist[i] = nt
		if _, exists := r.tracks[nt.ID]; !exists {
			r.tracks[nt.ID] = nt
		}
	}
	r.stations[cp.ID] = cp
	return nil
}

func (r *MemoryRepository) GetStation(ctx context.Context, id string) (*model.Station, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	st, ok := r.stations[id]
	if !ok {
		return nil, ErrStationNotFound
	}
	return cloneStation(st), nil
}

func (r *MemoryRepository) ListStations(ctx context.Context) ([]*model.Station, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.stations))
	for id := range r.stations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]*model.Station, 0, len(ids))
	for _, id := range ids {
		list = append(list, cloneStation(r.stations[id]))
	}
	return list, nil
}

func (r *MemoryRepository) UpdateStationDJ(ctx context.Context, stationID, djToken string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	st.DJ = djToken
	return nil
}

func (r *MemoryRepository) UpdateStationState(ctx context.Context, stationID string, state *model.PlaybackState, mixer []model.MixerChannel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	if state != nil {
		sc := *state
		st.State = &sc
	}
	if mixer != nil {
		st.Mixer = append([]model.MixerChannel{}, mixer...)
	}
	return nil
}

func (r *MemoryRepository) DeleteStation(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	if _, ok := r.stations[id]; !ok {
		return ErrStationNotFound
	}
	delete(r.stations, id)
	return nil
}

func (r *MemoryRepository) UpsertTrack(ctx context.Context, t model.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	t = normalizeTrack(t)
	if t.ID == "" {
		return fmt.Errorf("track id vazio")
	}
	r.tracks[t.ID] = t
	for _, st := range r.stations {
		for i := range st.Repository {
			if st.Repository[i].ID == t.ID {
				st.Repository[i] = t
			}
		}
		for i := range st.Playlist {
			if st.Playlist[i].ID == t.ID {
				st.Playlist[i] = t
			}
		}
	}
	return nil
}

func (r *MemoryRepository) GetTrack(ctx context.Context, trackID string) (model.Track, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return model.Track{}, false, err
	}
	cleanID := strings.TrimSuffix(trackID, ".opus")
	if t, ok := r.tracks[cleanID]; ok {
		return t, true, nil
	}
	if t, ok := r.tracks[trackID]; ok {
		return t, true, nil
	}
	return model.Track{}, false, nil
}

func (r *MemoryRepository) ListLibraryTracks(ctx context.Context) ([]model.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.tracks))
	for id := range r.tracks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]model.Track, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.tracks[id])
	}
	return out, nil
}

func (r *MemoryRepository) AddTrackToStation(ctx context.Context, stationID string, t model.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	t = normalizeTrack(t)
	r.tracks[t.ID] = t
	for i, existing := range st.Repository {
		if existing.ID == t.ID {
			st.Repository[i] = t
			return nil
		}
	}
	st.Repository = append(st.Repository, t)
	return nil
}

func (r *MemoryRepository) UpdateStationTrack(ctx context.Context, stationID string, t model.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	t = normalizeTrack(t)
	found := false
	for i, existing := range st.Repository {
		if existing.ID == t.ID {
			st.Repository[i] = t
			found = true
			break
		}
	}
	if !found {
		return ErrTrackNotFound
	}
	for i, existing := range st.Playlist {
		if existing.ID == t.ID {
			st.Playlist[i] = t
		}
	}
	r.tracks[t.ID] = t
	return nil
}

func (r *MemoryRepository) RemoveTrackFromStation(ctx context.Context, stationID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	cleanID := strings.TrimSuffix(trackID, ".opus")
	repo := make([]model.Track, 0, len(st.Repository))
	for _, t := range st.Repository {
		if t.ID != trackID && t.ID != cleanID {
			repo = append(repo, t)
		}
	}
	st.Repository = repo

	pl := make([]model.Track, 0, len(st.Playlist))
	for _, t := range st.Playlist {
		if t.ID != trackID && t.ID != cleanID {
			pl = append(pl, t)
		}
	}
	st.Playlist = pl
	return nil
}

func (r *MemoryRepository) ReplacePlaylist(ctx context.Context, stationID string, playlist []model.Track) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	copied := make([]model.Track, 0, len(playlist))
	for _, t := range playlist {
		nt := normalizeTrack(t)
		if _, exists := r.tracks[nt.ID]; !exists && nt.ID != "" {
			r.tracks[nt.ID] = nt
		}
		copied = append(copied, nt)
	}
	st.Playlist = copied
	return nil
}

func (r *MemoryRepository) GetPlaylist(ctx context.Context, stationID string) ([]model.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return nil, ErrStationNotFound
	}
	return append([]model.Track{}, st.Playlist...), nil
}

func (r *MemoryRepository) ListSuggestions(ctx context.Context, stationID string) ([]model.SongSuggestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return nil, err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return nil, ErrStationNotFound
	}
	return append([]model.SongSuggestion{}, st.Suggestions...), nil
}

func (r *MemoryRepository) SaveSuggestion(ctx context.Context, stationID string, sug model.SongSuggestion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	for i, existing := range st.Suggestions {
		if existing.ID == sug.ID {
			st.Suggestions[i] = sug
			return nil
		}
	}
	st.Suggestions = append(st.Suggestions, sug)
	return nil
}

func (r *MemoryRepository) UpdateSuggestionStatus(ctx context.Context, stationID, suggestionID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	for i, existing := range st.Suggestions {
		if existing.ID == suggestionID {
			st.Suggestions[i].Status = status
			return nil
		}
	}
	return nil
}

func (r *MemoryRepository) DeleteSuggestion(ctx context.Context, stationID, suggestionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	filtered := make([]model.SongSuggestion, 0, len(st.Suggestions))
	for _, sug := range st.Suggestions {
		if sug.ID != suggestionID {
			filtered = append(filtered, sug)
		}
	}
	st.Suggestions = filtered
	return nil
}

func (r *MemoryRepository) ClearSuggestions(ctx context.Context, stationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	st, ok := r.stations[stationID]
	if !ok {
		return ErrStationNotFound
	}
	st.Suggestions = make([]model.SongSuggestion, 0)
	return nil
}

func (r *MemoryRepository) GetYouTubeCache(ctx context.Context, videoID string) (model.Track, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := r.checkAvailable(); err != nil {
		return model.Track{}, false, err
	}
	trackID, ok := r.ytCache[videoID]
	if !ok {
		return model.Track{}, false, nil
	}
	t, ok := r.tracks[trackID]
	return t, ok, nil
}

func (r *MemoryRepository) SaveYouTubeCache(ctx context.Context, videoID, trackID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkAvailable(); err != nil {
		return err
	}
	r.ytCache[videoID] = strings.TrimSuffix(trackID, ".opus")
	return nil
}

func (r *MemoryRepository) Ping(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.checkAvailable()
}

func (r *MemoryRepository) Close() error {
	return nil
}
