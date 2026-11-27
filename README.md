# Karaoke Discord bot

Slash commands `/list-video`, `/search`, `/random`, `/stats`, `/setlist` and `/playlist` over the karaoke
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

## Playlists

`/playlist` lets any member manage their own shared playlists: `mine`, `create`, `add`,
`remove`, `rename`, `delete` and `share`. A member owns the playlists they create from
Discord, plus the ones their connected Discord account claimed on the site. The bot calls the
site's admin listener (`SITE_ADMIN_URL`, `SITE_ADMIN_SECRET`) and acts for the member who ran
the command. `SITE_PUBLIC_URL` is the address in the `/p/<id>` links the bot posts (default
`https://fionastation.vercel.app`). Without the first two the command answers "not set up".
`/playlist-mod` is the mods-only inspect and delete command.

## Commissions

Commission and inquiry forms on the site (voice licensing, Paopoa Studio covers, brand
endorsements) reach Discord through the bot. Every 30 seconds the bot asks the site's admin
listener for new inquiries. It posts each one as an embed in `COMMISSIONS_CHANNEL_ID`, starts a
thread on it and tells the site where it went. Leave `COMMISSIONS_CHANNEL_ID` empty to turn this
off. It also needs `SITE_ADMIN_URL` and `SITE_ADMIN_SECRET`.

The buttons under an embed are the statuses the site allows next. Only the Discord user ids in
`COMMISSION_MANAGER_IDS` (comma-separated) can press them. Everyone else gets a private refusal.
"Quoted" and "Declined" ask for an optional note first. After each change the bot updates the
embed and writes a line in the thread. The site decides which changes are legal and the client
sees progress on the tracking page.

If the bot restarts between posting a message and telling the site, it reads the last 100
messages in the channel on startup and adopts its own commission messages (the embed footer
reads `Commission <id>`), so it tells the site about the existing message instead of posting
again. If that history read fails, the bot logs it and carries on, and a duplicate is possible
until the next restart. Messages posted before the footer changed to this format are not adopted.

## Go-live webhook (OBS)

An OBS script POSTs to the bot when Fiona presses Start or Stop Streaming.

- On `started`, the bot asks YouTube for the live video on `YOUTUBE_CHANNEL_ID`. It asks at
  once, then every 15 seconds for up to 5 minutes, because YouTube can take a while to mark a
  broadcast live. When it finds the video, it posts a "Live now" embed in `LIVE_CHANNEL_ID`
  that links to the stream, with a ping for `LIVE_PING_ROLE_ID` when that is set. If nothing is
  found in 5 minutes, it posts nothing and logs `no live broadcast found for channel <id>`.
  This is the expected result for a private or unlisted test stream.
- On `stopped`, the bot stops looking and edits the live message to "Stream ended · 2h 14m",
  timed from when the message was posted. The link now leads to the VOD and the ping is
  removed.

The bot posts one message per stream. It keeps no state of its own: on each event it reads the
last 50 messages in the live channel and finds its newest live or ended embed by the embed
footer. A second `started` while a live message is open does nothing, a restart mid-stream
still lets `stopped` edit the right message, and starting again into a broadcast whose message
already ended posts nothing. The bot needs View Channel, Send Messages, Embed Links and Read
Message History in the live channel.

Each lookup is one `search.list` call. A stream that takes the full 5 minutes to appear costs
at most 21 calls.

The webhook turns on when `WEBHOOK_SECRET`, `LIVE_CHANNEL_ID` and `YOUTUBE_CHANNEL_ID` are all
set. Otherwise the bot logs that it is disabled and runs as before. `YOUTUBE_CHANNEL_ID` is
Fiona's `UC...` channel id, and `YOUTUBE_API_KEY` must be set for the lookup to work. It
listens on `WEBHOOK_ADDR` (default `:8080`). `WEBHOOK_BIND` is the host address compose
publishes the port on (default `127.0.0.1`).

```sh
curl -i -X POST http://127.0.0.1:8080/hooks/obs \
  -H "Authorization: Bearer $WEBHOOK_SECRET" \
  -d '{"event":"started"}'
```

A wrong or missing secret gets `401`, an unknown event or bad JSON gets `400`, and a body over
1 KiB gets `413`. `GET /healthz` answers `ok` without auth.

### Install the OBS script

The script is `scripts/obs/fiona-live.lua`, and it needs nothing else installed.

1. In OBS, open **Tools > Scripts**, press **+**, and pick `fiona-live.lua`.
2. Leave **Webhook URL** at `https://hooks.alwinihza.com/hooks/obs`, and paste the
   `WEBHOOK_SECRET` into **Secret**.
3. Start streaming. The **Script Log** button on the same window shows `sent started`.

On Windows the script starts `curl.exe` with no console window and does not wait for it, so OBS
never stalls. If the request fails, the bot posts nothing, and the bot's log is the place to look.

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
