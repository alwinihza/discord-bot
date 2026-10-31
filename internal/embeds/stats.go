package embeds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

const (
	maxFieldValue      = 1024
	recentPerformances = 10
)

var linkText = strings.NewReplacer("[", "(", "]", ")")

func performanceLine(p models.Performance) string {
	line := fmt.Sprintf("`%s` [%s](%s)", p.Date, linkText.Replace(Truncate(p.VideoTitle, 60)), p.URL)
	if p.Version != "" {
		line += " · " + p.Version
	}
	return line
}

func CreateStatsEmbed(st models.SongStats) *discordgo.MessageEmbed {
	first, last := st.Performances[0], st.Performances[len(st.Performances)-1]
	fields := []*discordgo.MessageEmbedField{
		{Name: "Times sung", Value: fmt.Sprint(len(st.Performances)), Inline: true},
		{Name: "Streams", Value: fmt.Sprint(st.StreamCount), Inline: true},
		{Name: "Rank", Value: fmt.Sprintf("#%d of %d songs", st.Rank, st.TotalSongs), Inline: true},
		{Name: "First sung", Value: performanceLine(first)},
	}
	if len(st.Performances) > 1 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Last sung", Value: performanceLine(last)})
	}

	if len(st.Versions) > 1 {
		// "" is a performance with no style tag.
		names := make([]string, 0, len(st.Versions))
		for v := range st.Versions {
			names = append(names, v)
		}
		sort.Slice(names, func(i, j int) bool {
			if st.Versions[names[i]] != st.Versions[names[j]] {
				return st.Versions[names[i]] > st.Versions[names[j]]
			}
			return names[i] < names[j]
		})
		parts := make([]string, len(names))
		for i, v := range names {
			if v == "" {
				v = "regular"
			}
			parts[i] = fmt.Sprintf("%s ×%d", v, st.Versions[names[i]])
		}
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Versions", Value: Truncate(strings.Join(parts, ", "), maxFieldValue)})
	}

	if len(st.Performances) > 2 {
		var b strings.Builder
		for i := len(st.Performances) - 1; i >= 0 && i >= len(st.Performances)-recentPerformances; i-- {
			line := performanceLine(st.Performances[i]) + "\n"
			if b.Len()+len(line) > maxFieldValue {
				break
			}
			b.WriteString(line)
		}
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Recent performances", Value: b.String()})
	}

	embed := &discordgo.MessageEmbed{
		Title:  Truncate("📊 "+models.SearchResult{Title: st.Title, Artist: st.Artist}.Label(), maxTitleLength),
		Color:  embedColor,
		Fields: fields,
	}
	if len(st.Notes) > 0 {
		embed.Description = Truncate(strings.Join(st.Notes, " · "), 4096)
	}
	if len(st.OtherMatches) > 0 {
		others := make([]string, len(st.OtherMatches))
		for i, c := range st.OtherMatches {
			others[i] = c.Label()
		}
		embed.Footer = &discordgo.MessageEmbedFooter{Text: Truncate("Also matched: "+strings.Join(others, " · "), 2048)}
	}
	return embed
}
