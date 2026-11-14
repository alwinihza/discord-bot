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
