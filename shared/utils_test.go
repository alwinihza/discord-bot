package shared

import "testing"

func TestTimestampToSeconds(t *testing.T) {
	cases := map[string]int{
		"01:04:48": 3888,
		"1:04:48":  3888,
		"04:48":    288,
		"48":       48,
		"":         0,
		"1:2:3:4":  0,
		"ab:cd:ef": 0,
		"-1:00":    0,
	}
	for in, want := range cases {
		if got := TimestampToSeconds(in); got != want {
			t.Errorf("TimestampToSeconds(%q) = %d, want %d", in, got, want)
		}
	}
}
