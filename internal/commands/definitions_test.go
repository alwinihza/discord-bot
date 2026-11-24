package commands

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestPlaylistModIsModsOnlyAndGuildOnly(t *testing.T) {
	for _, def := range Definitions {
		if def.Name != "playlist-mod" {
			continue
		}
		if def.DefaultMemberPermissions == nil || *def.DefaultMemberPermissions != discordgo.PermissionManageMessages {
			t.Errorf("DefaultMemberPermissions = %v, want Manage Messages", def.DefaultMemberPermissions)
		}
		if def.Contexts == nil || len(*def.Contexts) != 1 || (*def.Contexts)[0] != discordgo.InteractionContextGuild {
			t.Errorf("Contexts = %v, want guild only", def.Contexts)
		}
		subs := map[string]bool{}
		for _, sub := range def.Options {
			subs[sub.Name] = len(sub.Options) == 1 && sub.Options[0].Name == "link" && sub.Options[0].Required
		}
		if !subs["info"] || !subs["delete"] || len(subs) != 2 {
			t.Errorf("subcommands = %v, want info and delete each with a required link", subs)
		}
		return
	}
	t.Fatal("playlist-mod is not defined")
}

func TestAnnounceIsModsOnlyGuildOnlyWithRequiredVideoAndOptionalCredit(t *testing.T) {
	for _, def := range Definitions {
		if def.Name != "announce" {
			continue
		}
		if def.DefaultMemberPermissions == nil || *def.DefaultMemberPermissions != discordgo.PermissionManageMessages {
			t.Errorf("DefaultMemberPermissions = %v, want Manage Messages", def.DefaultMemberPermissions)
		}
		if def.Contexts == nil || len(*def.Contexts) != 1 || (*def.Contexts)[0] != discordgo.InteractionContextGuild {
			t.Errorf("Contexts = %v, want guild only", def.Contexts)
		}
		if len(def.Options) != 2 || def.Options[0].Name != "video" || !def.Options[0].Required ||
			def.Options[0].Type != discordgo.ApplicationCommandOptionString {
			t.Errorf("Options = %v, want a required string option video first", def.Options)
			return
		}
		if c := def.Options[1]; c.Name != "credit" || c.Required || c.Type != discordgo.ApplicationCommandOptionString {
			t.Errorf("Options[1] = %v, want an optional string option credit", c)
		}
		return
	}
	t.Fatal("announce is not defined")
}
