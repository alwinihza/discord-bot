package shared

import (
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// TimestampToSeconds parses "H:MM:SS", "MM:SS" or "SS". Malformed input
// yields 0 so a bad row links to the start of the video instead of failing.
func TimestampToSeconds(timestamp string) int {
	parts := strings.Split(strings.TrimSpace(timestamp), ":")
	if len(parts) > 3 {
		return 0
	}
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		total = total*60 + n
	}
	return total
}

func RespondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "❌ " + message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error sending error response: %v", err)
	}
}
