package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	fanConfirmPrefix = "playlist:confirm:"
	fanCancelID      = "playlist:cancel"
	maxMineLines     = 10
	mineBudget       = 1800
)

type escapedError string

func (e escapedError) Error() string { return string(e) }

const notYourPlaylist = escapedError("That is not one of your playlists, pick one from the list")

type playlistArgs struct{ playlist, name, song string }

type playlistReply struct {
	text            string
	components      []discordgo.MessageComponent
	channelFollowup string
}

type playlistAction func(h *CommandHandler, ctx context.Context, user string, a playlistArgs) (playlistReply, error)

var playlistActions = map[string]playlistAction{
	"mine":   (*CommandHandler).playlistMine,
	"create": (*CommandHandler).playlistCreate,
	"add":    (*CommandHandler).playlistAdd,
	"remove": (*CommandHandler).playlistRemove,
	"rename": (*CommandHandler).playlistRename,
	"delete": (*CommandHandler).playlistDelete,
	"share":  (*CommandHandler).playlistShare,
}

func actingUser(i *discordgo.InteractionCreate) string {
	switch {
	case i.Member != nil && i.Member.User != nil:
		return i.Member.User.ID
	case i.User != nil:
		return i.User.ID
	}
	return ""
}

func (h *CommandHandler) Playlist(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if h.site == nil {
		respondPlaylist(s, i, "❌ Playlists are not set up on this bot", nil)
		return
	}
	user := actingUser(i)
	subs := i.ApplicationCommandData().Options
	if user == "" || len(subs) == 0 {
		respondPlaylist(s, i, "❌ Could not read that command", nil)
		return
	}
	action, ok := playlistActions[subs[0].Name]
	if !ok {
		respondPlaylist(s, i, "❌ Unknown playlist command", nil)
		return
	}
	var args playlistArgs
	for _, opt := range subs[0].Options {
		switch opt.Name {
		case "playlist":
			args.playlist = opt.StringValue()
		case "name":
			args.name = opt.StringValue()
		case "song":
			args.song = opt.StringValue()
		}
	}

	if !deferEphemeral(s, i, discordgo.InteractionResponseDeferredChannelMessageWithSource) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout)
	defer cancel()
	reply, err := action(h, ctx, user, args)
	if err != nil {
		editPlaylist(s, i, "❌ "+h.siteError(err), nil)
		return
	}
	if reply.channelFollowup == "" {
		editPlaylist(s, i, reply.text, reply.components)
		return
	}
	editPlaylist(s, i, "Sharing in this channel…", nil)
	if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: reply.channelFollowup, AllowedMentions: noMentions}); err != nil {
		log.Printf("Error posting shared playlist: %v", err)
		editPlaylist(s, i, "❌ Could not post in this channel, please try again", nil)
		return
	}
	editPlaylist(s, i, reply.text, nil)
}

func deferEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, kind discordgo.InteractionResponseType) bool {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: kind,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions},
	})
	if err != nil {
		log.Printf("Error acknowledging playlist command: %v", err)
	}
	return err == nil
}

func (h *CommandHandler) handleFanPlaylistButton(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if customID == fanCancelID {
		cancelPlaylistDelete(s, i)
		return
	}
	user := actingUser(i)
	id, ok := site.PlaylistID(strings.TrimPrefix(customID, fanConfirmPrefix))
	if h.site == nil || user == "" || !ok {
		respondPlaylist(s, i, "❌ Could not delete that playlist", nil)
		return
	}
	if !deferEphemeral(s, i, discordgo.InteractionResponseDeferredMessageUpdate) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout)
	defer cancel()
	content := "Deleted. The link no longer works."
	if err := h.site.Remove(ctx, user, id); err != nil {
		content = "❌ " + h.siteError(err)
	}
	editPlaylist(s, i, content, []discordgo.MessageComponent{})
}

func ownedID(a playlistArgs) (string, error) {
	id, ok := site.PlaylistID(a.playlist)
	if !ok {
		return "", notYourPlaylist
	}
	return id, nil
}

func (h *CommandHandler) resolveSong(ctx context.Context, query string) (songID, label string, err error) {
	choice, err := h.db.ResolveSong(ctx, strings.TrimSpace(query))
	if errors.Is(err, database.ErrSongNotFound) {
		return "", "", escapedError(fmt.Sprintf(`No song matches "%s"`, escapeMarkdown(embeds.Truncate(query, 100))))
	}
	if err != nil {
		log.Printf("Error resolving song %q: %v", query, err)
		return "", "", escapedError("Failed to look up the song, please try again")
	}
	return choice.ID, choice.Label(), nil
}

func (h *CommandHandler) playlistMine(ctx context.Context, user string, _ playlistArgs) (playlistReply, error) {
	list, err := h.site.Mine(ctx, user)
	if err != nil {
		return playlistReply{}, err
	}
	if len(list) == 0 {
		return playlistReply{text: "You have no playlists yet. Make one with `/playlist create`."}, nil
	}
	var b strings.Builder
	for n, p := range list {
		line := fmt.Sprintf("**%s**, %s\n<%s>\n", displayName(p.Name), songCount(p.EntryCount), h.site.ViewLink(p.ID))
		more := fmt.Sprintf("…and %d more", len(list)-n)
		if n == maxMineLines || utf8.RuneCountInString(b.String()+line+more) > mineBudget {
			b.WriteString(more)
			break
		}
		b.WriteString(line)
	}
	return playlistReply{text: strings.TrimSpace(b.String())}, nil
}

func (h *CommandHandler) playlistCreate(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	songID, label, err := h.resolveSong(ctx, a.song)
	if err != nil {
		return playlistReply{}, err
	}
	p, err := h.site.Create(ctx, user, a.name, songID)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{text: fmt.Sprintf("Created **%s** with %s.\n%s", displayName(p.Name), escapeMarkdown(label), h.site.ViewLink(p.ID))}, nil
}

func (h *CommandHandler) playlistAdd(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	id, err := ownedID(a)
	if err != nil {
		return playlistReply{}, err
	}
	songID, label, err := h.resolveSong(ctx, a.song)
	if err != nil {
		return playlistReply{}, err
	}
	p, err := h.site.AddSong(ctx, user, id, songID)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{text: fmt.Sprintf("Added %s to **%s**, now %s.\n%s", escapeMarkdown(label), displayName(p.Name), songCount(len(p.Entries)), h.site.ViewLink(p.ID))}, nil
}

func (h *CommandHandler) playlistRemove(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	id, err := ownedID(a)
	if err != nil {
		return playlistReply{}, err
	}
	current, err := h.site.Get(ctx, user, id)
	if err != nil {
		return playlistReply{}, err
	}
	labels, err := h.entryLabels(ctx, current.Entries)
	if err != nil {
		log.Printf("Error labelling playlist entries: %v", err)
		return playlistReply{}, escapedError("Failed to look up the song, please try again")
	}
	entry, ok := findEntry(current.Entries, labels, a.song)
	if !ok {
		return playlistReply{}, escapedError("That song is not in this playlist, pick one from the list")
	}
	p, err := h.site.RemoveSong(ctx, user, id, entry.SongID)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{text: fmt.Sprintf("Removed %s from **%s**, now %s.\n%s", escapeMarkdown(labels[entry.SongID]), displayName(p.Name), songCount(len(p.Entries)), h.site.ViewLink(p.ID))}, nil
}

func (h *CommandHandler) entryLabels(ctx context.Context, entries []site.Entry) (map[string]string, error) {
	ids := make([]string, len(entries))
	for n, e := range entries {
		ids[n] = e.SongID
	}
	songs, err := h.db.SongsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string, len(entries))
	for _, e := range entries {
		labels[e.SongID] = e.Title
		if song, ok := songs[e.SongID]; ok {
			labels[e.SongID] = song.Label()
		}
	}
	return labels, nil
}

func findEntry(entries []site.Entry, labels map[string]string, query string) (site.Entry, bool) {
	query = strings.TrimSpace(query)
	var match []site.Entry
	for _, e := range entries {
		if e.SongID == query {
			return e, true
		}
		if query != "" && strings.Contains(strings.ToLower(labels[e.SongID]), strings.ToLower(query)) {
			match = append(match, e)
		}
	}
	if len(match) == 1 {
		return match[0], true
	}
	return site.Entry{}, false
}

func (h *CommandHandler) playlistRename(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	id, err := ownedID(a)
	if err != nil {
		return playlistReply{}, err
	}
	p, err := h.site.Rename(ctx, user, id, a.name)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{text: fmt.Sprintf("Renamed to **%s**.\n%s", displayName(p.Name), h.site.ViewLink(p.ID))}, nil
}

func (h *CommandHandler) playlistDelete(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	id, err := ownedID(a)
	if err != nil {
		return playlistReply{}, err
	}
	p, err := h.site.Get(ctx, user, id)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{
		text: fmt.Sprintf("Delete **%s**, %s? Its link stops working.", displayName(p.Name), songCount(len(p.Entries))),
		components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Delete", Style: discordgo.DangerButton, CustomID: fanConfirmPrefix + id},
			discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: fanCancelID},
		}}},
	}, nil
}

func (h *CommandHandler) playlistShare(ctx context.Context, user string, a playlistArgs) (playlistReply, error) {
	id, err := ownedID(a)
	if err != nil {
		return playlistReply{}, err
	}
	p, err := h.site.Get(ctx, user, id)
	if err != nil {
		return playlistReply{}, err
	}
	return playlistReply{
		text:            "Shared in this channel.",
		channelFollowup: fmt.Sprintf("<@%s> shared **%s**, %s.\n%s", user, displayName(p.Name), songCount(len(p.Entries)), h.site.ViewLink(p.ID)),
	}, nil
}

func songCount(n int) string {
	if n == 1 {
		return "1 song"
	}
	return fmt.Sprintf("%d songs", n)
}

func (h *CommandHandler) playlistChoices(i *discordgo.InteractionCreate) []*discordgo.ApplicationCommandOptionChoice {
	user := actingUser(i)
	subs := i.ApplicationCommandData().Options
	if h.site == nil || user == "" || len(subs) == 0 {
		return nil
	}
	sub := subs[0]
	var focused *discordgo.ApplicationCommandInteractionDataOption
	var chosen string
	for _, opt := range sub.Options {
		if opt.Focused {
			focused = opt
		} else if opt.Name == "playlist" {
			chosen = opt.StringValue()
		}
	}
	if focused == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(focused.StringValue()))
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var choices []*discordgo.ApplicationCommandOptionChoice
	add := func(label, value string) {
		if label != "" && len(choices) < maxChoices {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: embeds.Truncate(label, 100), Value: value})
		}
	}
	switch {
	case focused.Name == "playlist":
		list, err := h.site.Mine(ctx, user)
		if err != nil {
			log.Printf("Error listing playlist choices: %v", err)
			return nil
		}
		for _, p := range list {
			if strings.Contains(strings.ToLower(p.Name), query) {
				add(fmt.Sprintf("%s (%s)", boundName(p.Name), songCount(p.EntryCount)), p.ID)
			}
		}
	case focused.Name == "song" && sub.Name == "remove":
		id, ok := site.PlaylistID(chosen)
		if !ok {
			return nil
		}
		p, err := h.site.Get(ctx, user, id)
		if err != nil {
			log.Printf("Error listing playlist choices: %v", err)
			return nil
		}
		labels, err := h.entryLabels(ctx, p.Entries)
		if err != nil {
			log.Printf("Error labelling playlist entries: %v", err)
			return nil
		}
		for _, e := range p.Entries {
			if strings.Contains(strings.ToLower(labels[e.SongID]), query) {
				add(labels[e.SongID], e.SongID)
			}
		}
	case focused.Name == "song":
		return h.songChoices(focused.StringValue())
	}
	return choices
}
