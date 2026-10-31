# Karaoke Discord bot

Slash commands `/list-video`, `/search` and `/random` over the karaoke streams
and the songs sung in them. Data lives in a SQLite file the bot creates and
seeds from `database/seed.sql` on first start.

## Run

```sh
cp .env.example .env   # fill in TOKEN
docker compose up -d --build
docker compose logs -f discord-bot   # "Bot is ready as ..."
```

Without Docker: `TOKEN=... DATABASE_PATH=./karaoke.db go run .`

Tests: `go test ./...` (they seed a temporary database from the real data).

Pushing to `main` runs the tests on GitHub, then deploys on the devbox through
a self-hosted runner labelled `discord-bot`. The deploy reads
`/home/alwinihza/project/discord-bot/.env`, and the compose project is pinned to
`discord-bot`, so a manual `docker compose up` in this checkout manages the same
container and volume.

## Add a stream and its songs

Open a SQL shell inside the running container:

```sh
docker compose exec discord-bot sqlite3 /data/karaoke.db
```

```sql
INSERT INTO karaoke_video (id, video_title, video_url, published_at)
VALUES (lower(hex(randomblob(16))), '【Karaoke】Stream title', 'dQw4w9WgXcQ', '2026-11-01 20:00:00+00');

INSERT INTO karaoke_song (id, song_title, artist, version, original_title, timestamp, video_id)
SELECT lower(hex(randomblob(16))), 'Kokoronashi', 'Chouchou-P', 'short ver.',
       'GUMI - Kokoronashi (short ver.)', '1:04:48', id
FROM karaoke_video WHERE video_url = 'dQw4w9WgXcQ';
```

- `video_url` is the 11-character YouTube video ID, not a full URL.
- `timestamp` is `H:MM:SS` from the start of the stream.
- `artist` is the original performer, `version` holds notes such as
  `short ver.` or `Hanatan cover`. Search also reads `original_title`, so put
  any extra names viewers might search for there (vocal synth, cover singer).

Writing while the bot runs is fine.

## Re-seed from scratch

The seed only runs when the database is empty. To load a changed `seed.sql`,
delete the volume. **This also deletes every song added by hand since.**

```sh
docker compose down -v && docker compose up -d --build
```

`database/seed.sql` is generated from the last Supabase dump (not in the
repo) and `scripts/song-cleanup.tsv`, which maps every original song title to
its artist, title and version:

```sh
python3 scripts/extract-seed.py <dump.sql.gz> scripts/song-cleanup.tsv > database/seed.sql
```

Fix a song's artist or title by editing its row in `song-cleanup.tsv` and
regenerating. Rows with `confidence` = `unsure` are the ones worth checking.
