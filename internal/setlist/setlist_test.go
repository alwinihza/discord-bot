package setlist

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse(`
13:22 | Runtuh | Feby Putri feat. Fiersa Besari
1:02:03|Samurai Heart (Some Like It Hot!!)|SPYAIR|short|Gintama ED

  0:05 | Aladdin Ngawur - A Whole New World versi Arab |
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Timestamp: "00:13:22", Title: "Runtuh", Artist: "Feby Putri feat. Fiersa Besari"},
		{Timestamp: "01:02:03", Title: "Samurai Heart (Some Like It Hot!!)", Artist: "SPYAIR", Version: "short", Note: "Gintama ED"},
		{Timestamp: "00:00:05", Title: "Aladdin Ngawur - A Whole New World versi Arab"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseRejectsEveryBadLine(t *testing.T) {
	_, err := Parse("13:22 Feby Putri - Halu\n20:42 | Runtuh\n1:75:00 | Halu\n00:01 |  | Raisa\n1|2|3|4|5|6")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, line := range []string{"line 1", "line 3", "line 4", "line 5"} {
		if !strings.Contains(err.Error(), line) {
			t.Errorf("error %q does not mention %s", err, line)
		}
	}
	if strings.Contains(err.Error(), "line 2") {
		t.Errorf("error %q blames the valid line 2", err)
	}
	if _, err := Parse("\n  \n"); err == nil {
		t.Error("empty text: want an error")
	}
}

func TestFormatRoundTrips(t *testing.T) {
	in := []Entry{
		{Timestamp: "00:00:00", Title: "Hirari, Hirari"},
		{Timestamp: "00:13:22", Title: "Halu", Artist: "Feby Putri", Note: "Esme cover"},
	}
	text := Format(in)
	if want := "00:00:00 | Hirari, Hirari\n00:13:22 | Halu | Feby Putri |  | Esme cover\n"; text != want {
		t.Fatalf("Format = %q, want %q", text, want)
	}
	out, err := Parse(text)
	if err != nil || !reflect.DeepEqual(out, in) {
		t.Fatalf("Parse(Format) = %+v, %v", out, err)
	}
}

func TestVideoID(t *testing.T) {
	for in, want := range map[string]string{
		"KANYG45JUlo":                                     "KANYG45JUlo",
		"https://youtu.be/KANYG45JUlo?t=30":               "KANYG45JUlo",
		"https://www.youtube.com/watch?v=KANYG45JUlo":     "KANYG45JUlo",
		"https://m.youtube.com/watch?app=1&v=a-b_c1234XY": "a-b_c1234XY",
		"https://www.youtube.com/live/KANYG45JUlo?si=x":   "KANYG45JUlo",
		"https://youtube.com/shorts/KANYG45JUlo":          "KANYG45JUlo",
	} {
		if got, ok := VideoID(in); !ok || got != want {
			t.Errorf("VideoID(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "Halu", "https://youtu.be/short", "KANYG45JUloX"} {
		if got, ok := VideoID(in); ok {
			t.Errorf("VideoID(%q) = %q, want no match", in, got)
		}
	}
}
