-- Rollback da Migration 0001: Schema inicial do RadioSync (ADR-0006)

DROP TABLE IF EXISTS youtube_cache;
DROP TABLE IF EXISTS suggestions;
DROP TABLE IF EXISTS playlist_items;
DROP TABLE IF EXISTS station_tracks;
DROP TABLE IF EXISTS stations;
DROP TABLE IF EXISTS tracks;
