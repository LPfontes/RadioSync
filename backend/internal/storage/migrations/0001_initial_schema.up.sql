-- Migration 0001: Schema inicial do RadioSync (ADR-0006)
-- Tabelas: tracks, stations, station_tracks, playlist_items, suggestions, youtube_cache

CREATE TABLE IF NOT EXISTS tracks (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    filename    TEXT NOT NULL,
    url         TEXT NOT NULL,
    duration    DOUBLE PRECISION NOT NULL DEFAULT 0,
    category    TEXT NOT NULL DEFAULT '',
    theme       TEXT NOT NULL DEFAULT '',
    cover_url   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tracks_filename ON tracks(filename);

CREATE TABLE IF NOT EXISTS stations (
    id           VARCHAR(16) PRIMARY KEY,
    dj_token     TEXT NOT NULL,
    is_playing   BOOLEAN NOT NULL DEFAULT FALSE,
    started_at   BIGINT NOT NULL DEFAULT 0,
    seek_offset  DOUBLE PRECISION NOT NULL DEFAULT 0,
    current_song TEXT NOT NULL DEFAULT '',
    duration     DOUBLE PRECISION NOT NULL DEFAULT 0,
    mixer_json   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS station_tracks (
    station_id  VARCHAR(16) NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INT NOT NULL DEFAULT 0,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (station_id, track_id)
);

CREATE INDEX IF NOT EXISTS idx_station_tracks_station ON station_tracks(station_id, position);

CREATE TABLE IF NOT EXISTS playlist_items (
    id          BIGSERIAL PRIMARY KEY,
    station_id  VARCHAR(16) NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INT NOT NULL,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_playlist_items_station_pos ON playlist_items(station_id, position);

CREATE TABLE IF NOT EXISTS suggestions (
    id           TEXT PRIMARY KEY,
    station_id   VARCHAR(16) NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    track_id     TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL,
    url          TEXT NOT NULL DEFAULT '',
    suggested_by TEXT NOT NULL,
    created_at   BIGINT NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending'
);

CREATE INDEX IF NOT EXISTS idx_suggestions_station ON suggestions(station_id, created_at);

CREATE TABLE IF NOT EXISTS youtube_cache (
    video_id    TEXT PRIMARY KEY,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
