package commands

import "github.com/bwmarrin/discordgo"

var guildOnly = &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild}

// Definitions is the full slash-command set the bot registers on startup.
// Discord replaces whatever was registered before with exactly this list.
var Definitions = []*discordgo.ApplicationCommand{
	{Name: "list-video", Description: "List Karaoke Video", Contexts: guildOnly},
	{Name: "random", Description: "Get a random song by Fiona", Contexts: guildOnly},
	{
		Name: "search", Description: "Search the song", Contexts: guildOnly,
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "search", Description: "Title", Required: true,
		}},
	},
	{
		Name: "stats", Description: "Show how often a song was sung", Contexts: guildOnly,
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "song", Description: "Song title or artist",
			Required: true, Autocomplete: true,
		}},
	},
}
