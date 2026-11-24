package commands

import "github.com/bwmarrin/discordgo"

var (
	guildOnly = &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild}
	// Server admins can change who may use /setlist under Integrations.
	modsOnly = int64(discordgo.PermissionManageMessages)
)

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
	{
		Name: "setlist", Description: "Add or edit a stream's setlist (mods)", Contexts: guildOnly,
		DefaultMemberPermissions: &modsOnly,
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "video", Description: "YouTube link or video ID",
			Required: true,
		}},
	},
	{
		Name: "announce", Description: "Post the new-stream announcement for a saved setlist (mods)", Contexts: guildOnly,
		DefaultMemberPermissions: &modsOnly,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type: discordgo.ApplicationCommandOptionString, Name: "video", Description: "YouTube link or video ID",
				Required: true,
			},
			{
				Type: discordgo.ApplicationCommandOptionString, Name: "credit", Description: "Who posted the timestamps, a name or @mention",
				MaxLength: 100,
			},
		},
	},
	{
		Name: "playlist", Description: "Manage your shared playlists", Contexts: guildOnly,
		Options: []*discordgo.ApplicationCommandOption{
			fanSubcommand("mine", "List your playlists"),
			fanSubcommand("create", "Start a playlist with one song",
				fanOption("name", "Playlist name", false), fanOption("song", "First song", true)),
			fanSubcommand("add", "Add a song to a playlist",
				fanOption("playlist", "Your playlist", true), fanOption("song", "Song to add", true)),
			fanSubcommand("remove", "Remove a song from a playlist",
				fanOption("playlist", "Your playlist", true), fanOption("song", "Song to remove", true)),
			fanSubcommand("rename", "Rename a playlist",
				fanOption("playlist", "Your playlist", true), fanOption("name", "New name", false)),
			fanSubcommand("delete", "Delete a playlist after confirmation",
				fanOption("playlist", "Your playlist", true)),
			fanSubcommand("share", "Post a playlist's link in this channel",
				fanOption("playlist", "Your playlist", true)),
		},
	},
	{
		Name: "playlist-mod", Description: "Inspect or delete a shared playlist (mods)", Contexts: guildOnly,
		DefaultMemberPermissions: &modsOnly,
		Options: []*discordgo.ApplicationCommandOption{
			playlistSubcommand("info", "Show a playlist's name and song count"),
			playlistSubcommand("delete", "Delete a playlist after confirmation"),
		},
	},
}

func fanSubcommand(name, description string, options ...*discordgo.ApplicationCommandOption) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionSubCommand, Name: name, Description: description, Options: options,
	}
}

func fanOption(name, description string, autocomplete bool) *discordgo.ApplicationCommandOption {
	opt := &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionString, Name: name, Description: description,
		Required: true, Autocomplete: autocomplete,
	}
	if !autocomplete {
		opt.MaxLength = 100
	}
	return opt
}

func playlistSubcommand(name, description string) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionSubCommand, Name: name, Description: description,
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "link", Description: "Playlist link or id",
			Required: true,
		}},
	}
}
