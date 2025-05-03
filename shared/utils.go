package shared

import (
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func TimestampToSeconds(timestamp string) int {
	parts := strings.Split(timestamp, ":")
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	totalSeconds := hours*3600 + minutes*60 + seconds
	return totalSeconds
}

func RespondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "❌ " + message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
