# Karaoke Discord bot

Slash commands `/list-video`, `/search`, `/random`, `/stats` and `/setlist` over the karaoke
streams and the songs sung in them. Data lives in a SQLite file the bot creates
and seeds from `database/seed.sql` on first start, then upgrades with
`database/migrations/NNN_*.sql`, each applied once in order.

The bot registers its slash commands from `internal/commands/definitions.go`
on every start, replacing whatever Discord had, so add new commands there.

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

In Discord, run `/setlist video:<YouTube link>`. It opens a form with the
stream's current setlist, empty for a new stream. Write one song per line:

```text
00:13:22 | Halu | Feby Putri
01:04:48 | Kokoronashi | Chouchou-P | short | Hanatan cover
```

The fields are time, title, artist, version and note, split by `|` because
titles contain dashes and parentheses. Only time and title are required.
Submitting replaces that stream's whole setlist. A new stream gets its title
and date from the YouTube Data API, using `YOUTUBE_API_KEY` from `.env`. If any line is wrong, nothing is saved and the bot
sends the text back as a file to fix. The reply lists songs no other stream
has, which catches typos in names the bot already knows. The log keeps every
replaced setlist, so `docker compose logs discord-bot | grep -A60 "saved"`
recovers one.

When `/setlist` adds a stream the bot did not have, it posts the stream and
its songs to the channel in `ANNOUNCE_CHANNEL_ID` (in `.env`; leave it empty
to post nothing). Edits to a known stream post nothing. The bot needs View
Channel, Send Messages and Embed Links there.

Only members with Manage Messages see `/setlist` by default. Change that in
Server Settings, Integrations.

For bulk edits, open a SQL shell inside the running container:

```sh
docker compose exec discord-bot sqlite3 /data/karaoke.db
```

```sql
INSERT INTO karaoke_video (id, video_title, video_url, published_at)
VALUES (lower(hex(randomblob(16))), '【Karaoke】Stream title', 'dQw4w9WgXcQ', '2026-11-01 20:00:00+00');

INSERT INTO karaoke_song (id, song_title, artist, version, note, original_title, timestamp, video_id)
SELECT lower(hex(randomblob(16))), 'Kokoronashi', 'Chouchou-P', 'short', 'Hanatan cover',
       'GUMI - Kokoronashi (short)', '01:04:48', id
FROM karaoke_video WHERE video_url = 'dQw4w9WgXcQ';
```

- `video_url` is the 11-character YouTube video ID, not a full URL.
- `timestamp` is `HH:MM:SS` from the start of the stream.
- `artist` is the original performer.
- `version` is short playing-style tags shown next to the title, such as
  `short`, `acapella`, `duet` or `take 2`. Leave it empty for a normal full
  performance.
- `note` is context kept out of the title, such as the cover it follows
  (`Hanatan cover`) or the series (`Re:Zero ED`). `/stats` shows it.
- Search reads title, artist, version, note and `original_title`, so put any
  extra names viewers might search for in `original_title` (vocal synth).

Writing while the bot runs is fine.

## Re-seed from scratch

The seed only runs when the database is empty. To load a changed `seed.sql`,
delete the volume. **This also deletes every song added by hand since.**

```sh
docker compose down -v && docker compose up -d --build
```

## Backups

Every deploy first snapshots the live database to `~/backups/discord-bot/` on
the devbox, keeping the newest 30. To restore one:

```sh
docker compose cp ~/backups/discord-bot/<file>.db discord-bot:/tmp/restore.db
docker compose exec discord-bot sqlite3 /data/karaoke.db ".restore /tmp/restore.db"
docker compose restart discord-bot
```

These copies live on the devbox itself, so they don't survive losing its disk.
