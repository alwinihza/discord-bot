"""Convert the karaoke tables of a Postgres plain-SQL dump into SQLite seed SQL.

Usage: python3 scripts/extract-seed.py <dump.sql[.gz]> scripts/song-cleanup.tsv > database/seed.sql

song-cleanup.tsv maps each original song_title, as escaped in the dump's COPY
text (a tab is written \t), to its cleaned
artist, title and version. Every song in the dump must have a mapping.
"""
import csv
import gzip
import re
import sys

TABLES = ["karaoke_video", "karaoke_song"]  # parents before children
ESCAPES = {"t": "\t", "n": "\n", "r": "\r", "\\": "\\", "b": "\b", "f": "\f", "v": "\v"}


def decode(field):
    if field == r"\N":
        return None
    return re.sub(r"\\(.)", lambda m: ESCAPES.get(m.group(1), m.group(1)), field)


BOOL_COLUMNS = {"membership"}


def literal(col, value):
    if value is None:
        return "NULL"
    if col in BOOL_COLUMNS:
        return "1" if value == "t" else "0"
    return "'" + value.replace("'", "''") + "'"


def read_copy_blocks(lines):
    blocks, current = {}, None
    for line in lines:
        line = line.rstrip("\n")
        if current is None:
            m = re.match(r"^COPY public\.(\w+) \((.*)\) FROM stdin;$", line)
            if m and m.group(1) in TABLES:
                current = m.group(1)
                cols = [c.strip().strip('"') for c in m.group(2).split(",")]
                blocks[current] = (cols, [])
        elif line == r"\.":
            current = None
        else:
            blocks[current][1].append([decode(f) for f in line.split("\t")])
    return blocks


def read_cleanup(path):
    with open(path, encoding="utf-8", newline="") as f:
        rows = csv.DictReader(f, delimiter="\t", quoting=csv.QUOTE_NONE)
        return {r["original"]: r for r in rows}


def copy_escape(value):
    return value.replace("\\", "\\\\").replace("\t", "\\t").replace("\n", "\\n").replace("\r", "\\r")


def apply_cleanup(blocks, cleanup):
    cols, rows = blocks["karaoke_song"]
    i = cols.index("song_title")
    originals = [r[i] for r in rows]
    rows = [r[:i] + [copy_escape(r[i])] + r[i + 1:] for r in rows]
    missing = sorted({r[i] for r in rows if r[i] not in cleanup})
    if missing:
        sys.exit("no cleanup mapping for: " + ", ".join(repr(m) for m in missing))
    blocks["karaoke_song"] = (
        cols + ["artist", "version", "original_title"],
        [r[:i] + [cleanup[r[i]]["title"]] + r[i + 1:]
         + [cleanup[r[i]]["artist"] or None, cleanup[r[i]]["version"] or None, orig]
         for r, orig in zip(rows, originals)],
    )


def main(path, cleanup_path):
    opener = gzip.open if path.endswith(".gz") else open
    with opener(path, "rt", encoding="utf-8") as f:
        blocks = read_copy_blocks(f)
    apply_cleanup(blocks, read_cleanup(cleanup_path))
    out = sys.stdout
    for table in TABLES:
        cols, rows = blocks[table]
        for row in rows:
            out.write(f"INSERT INTO {table} ({', '.join(cols)}) VALUES "
                      f"({', '.join(literal(c, v) for c, v in zip(cols, row))});\n")


main(sys.argv[1], sys.argv[2])
