package embeds

import (
	"testing"
	"time"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

var liveVideo = models.VideoInfo{ID: "LIVE1234567", Title: "【KARAOKE】Fio sings", Thumbnail: "https://i.ytimg.com/vi/LIVE1234567/hqdefault.jpg"}

func TestLiveEmbed(t *testing.T) {
	e := CreateLiveEmbed(liveVideo)
	if e.URL != "https://www.youtube.com/watch?v=LIVE1234567" {
		t.Errorf("url = %q", e.URL)
	}
	if e.Title != "🔴 【KARAOKE】Fio sings" || e.Description != "🔴 **Live now**" {
		t.Errorf("title %q, description %q", e.Title, e.Description)
	}
	if e.Image == nil || e.Image.URL != liveVideo.Thumbnail {
		t.Errorf("image = %+v", e.Image)
	}
}

func TestEndedEmbedLength(t *testing.T) {
	for _, tt := range []struct {
		length time.Duration
		want   string
	}{
		{2*time.Hour + 14*time.Minute + 10*time.Second, "Stream ended · 2h 14m"},
		{59*time.Minute + 50*time.Second, "Stream ended · 1h 0m"},
		{42 * time.Minute, "Stream ended · 42m"},
		{20 * time.Second, "Stream ended · 0m"},
		{-time.Minute, "Stream ended · 0m"},
	} {
		e := CreateEndedEmbed(liveVideo, tt.length)
		if e.Description != tt.want {
			t.Errorf("%s: description = %q, want %q", tt.length, e.Description, tt.want)
		}
		if e.URL != "https://www.youtube.com/watch?v=LIVE1234567" || e.Title != liveVideo.Title {
			t.Errorf("ended embed lost the VOD link or title: %q %q", e.URL, e.Title)
		}
	}
}

func TestParseGoLive(t *testing.T) {
	msg := func(embeds ...*discordgo.MessageEmbed) *discordgo.Message { return &discordgo.Message{Embeds: embeds} }
	tests := []struct {
		name  string
		m     *discordgo.Message
		kind  GoLiveKind
		video models.VideoInfo
	}{
		{"live", msg(CreateLiveEmbed(liveVideo)), Live, liveVideo},
		{"ended", msg(CreateEndedEmbed(liveVideo, time.Hour)), Ended, liveVideo},
		{"announce embed", msg(CreateNewStreamEmbed(liveVideo, nil, "")), NotGoLive, models.VideoInfo{}},
		{"no embeds", msg(), NotGoLive, models.VideoInfo{}},
		{"live footer, foreign link", msg(&discordgo.MessageEmbed{URL: "https://example.com", Footer: &discordgo.MessageEmbedFooter{Text: liveFooter}}), NotGoLive, models.VideoInfo{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, video := ParseGoLive(tt.m)
			if kind != tt.kind || video != tt.video {
				t.Errorf("got %v %+v, want %v %+v", kind, video, tt.kind, tt.video)
			}
		})
	}
}
