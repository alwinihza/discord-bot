CREATE TABLE karaoke_video (
    id           TEXT PRIMARY KEY,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    video_title  TEXT NOT NULL,
    video_url    TEXT NOT NULL,
    membership   INTEGER NOT NULL DEFAULT 0,
    published_at TEXT,
    thumbnail    TEXT
);

CREATE TABLE karaoke_song (
    id            TEXT PRIMARY KEY,
    created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    song_title    TEXT NOT NULL,
    artist        TEXT,
    version       TEXT,
    -- the title as originally typed, kept so search still matches names the
    -- cleanup moved out of title/artist (vocal synths, cover singers)
    original_title TEXT,
    timestamp     TEXT NOT NULL,
    video_id      TEXT NOT NULL REFERENCES karaoke_video(id),
    timestamp_end TEXT
);

CREATE INDEX karaoke_song_video_id ON karaoke_song(video_id);
