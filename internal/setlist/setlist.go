// Package setlist converts between a stream's songs and the text mods edit
// in /setlist, one song per line:
//
//	HH:MM:SS | Title | Artist | version | note
//
// Only the timestamp and title are required. Titles contain parentheses and
// " - ", so fields are split on "|", which no song name uses.
package setlist

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Entry struct {
	Timestamp string // always HH:MM:SS
	Title     string
	Artist    string
	Version   string
	Note      string
}

// LineError reports every line that failed, so a mod can fix them in one go.
type LineError []string

func (e LineError) Error() string { return strings.Join(e, "\n") }

// Parse reads setlist text. Blank lines are skipped. Any bad line rejects
// the whole text, so a half-understood paste never reaches the database.
func Parse(text string) ([]Entry, error) {
	var entries []Entry
	var bad LineError
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		e, err := parseLine(line)
		if err != nil {
			bad = append(bad, fmt.Sprintf("line %d: %v", n+1, err))
			continue
		}
		entries = append(entries, e)
	}
	if bad != nil {
		return nil, bad
	}
	if len(entries) == 0 {
		return nil, LineError{"no songs found"}
	}
	return entries, nil
}

func parseLine(line string) (Entry, error) {
	fields := strings.Split(line, "|")
	if len(fields) < 2 || len(fields) > 5 {
		return Entry{}, fmt.Errorf("want `time | title | artist`, got %q", line)
	}
	for n := range fields {
		fields[n] = strings.TrimSpace(fields[n])
	}
	fields = append(fields, make([]string, 5-len(fields))...)
	ts, err := NormalizeTimestamp(fields[0])
	if err != nil {
		return Entry{}, err
	}
	if fields[1] == "" {
		return Entry{}, fmt.Errorf("missing title")
	}
	return Entry{Timestamp: ts, Title: fields[1], Artist: fields[2], Version: fields[3], Note: fields[4]}, nil
}

// NormalizeTimestamp turns "SS", "MM:SS" or "H:MM:SS" into "HH:MM:SS".
// Zero-padding keeps a stream's songs in order when sorted as text.
func NormalizeTimestamp(ts string) (string, error) {
	parts := strings.Split(ts, ":")
	if len(parts) > 3 {
		return "", fmt.Errorf("bad time %q", ts)
	}
	total := 0
	for n, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || (n > 0 && v > 59) {
			return "", fmt.Errorf("bad time %q", ts)
		}
		total = total*60 + v
	}
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, total/60%60, total%60), nil
}

// Format renders entries as editable text, dropping empty trailing fields.
func Format(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		fields := []string{e.Timestamp, e.Title, e.Artist, e.Version, e.Note}
		for len(fields) > 2 && fields[len(fields)-1] == "" {
			fields = fields[:len(fields)-1]
		}
		b.WriteString(strings.Join(fields, " | ") + "\n")
	}
	return b.String()
}

var videoIDPattern = regexp.MustCompile(`(?:^|v=|youtu\.be/|/live/|/shorts/|/embed/)([A-Za-z0-9_-]{11})(?:$|[?&#/])`)

// VideoID extracts the 11-character ID from a YouTube URL or a bare ID.
func VideoID(s string) (string, bool) {
	m := videoIDPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	return m[1], true
}
