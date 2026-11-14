package commissions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func pressed(user, customID string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionMessageComponent,
		Member:  &discordgo.Member{User: &discordgo.User{ID: user}},
		Message: &discordgo.Message{ID: "m1", ChannelID: channel},
		Data:    discordgo.MessageComponentInteractionData{CustomID: customID},
	}}
}

func submitted(user, customID, note string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionModalSubmit,
		Member:  &discordgo.Member{User: &discordgo.User{ID: user}},
		Message: &discordgo.Message{ID: "m1", ChannelID: channel},
		Data: discordgo.ModalSubmitInteractionData{CustomID: customID, Components: []discordgo.MessageComponent{
			&discordgo.ActionsRow{Components: []discordgo.MessageComponent{&discordgo.TextInput{CustomID: "note", Value: note}}},
		}},
	}}
}

func newHandler(t *testing.T, fs *fakeSite) (*Handler, *discordgo.Session, *fakeDiscord) {
	t.Helper()
	s, fd := newSession(t)
	return NewHandler(fs.client(t), map[string]bool{manager: true}), s, fd
}

func updatedJSON(t *testing.T, status string, next ...string) string {
	t.Helper()
	c := commission("c1")
	c.Status, c.Next = status, next
	raw, _ := json.Marshal(c)
	return string(raw)
}

func callbackType(c discordCall) float64 {
	v, _ := c.Body["type"].(float64)
	return v
}

func TestNonManagerIsRefusedEphemerally(t *testing.T) {
	fs := &fakeSite{}
	h, s, fd := newHandler(t, fs)

	h.HandleButton(s, pressed(other, ButtonID(Action{"c1", "in_progress"})))
	h.HandleButton(s, pressed(other, ButtonID(Action{"c1", "quoted"})))
	h.HandleModal(s, submitted(other, ModalID(Action{"c1", "quoted"}), "sneaky"))

	if len(fs.callsTo("/events")) != 0 {
		t.Error("a non-manager reached the site")
	}
	callbacks := fd.find("POST", "/callback")
	if len(callbacks) != 3 {
		t.Fatalf("callbacks = %d, want 3 refusals", len(callbacks))
	}
	for _, c := range callbacks {
		data := c.Body["data"].(map[string]any)
		if callbackType(c) != float64(discordgo.InteractionResponseChannelMessageWithSource) ||
			data["flags"] != float64(discordgo.MessageFlagsEphemeral) || !strings.Contains(data["content"].(string), "Only commission managers") {
			t.Errorf("refusal = %+v", c.Body)
		}
	}
	if len(fd.find("POST", "/channels/m1/messages")) != 0 {
		t.Error("a refusal wrote to the thread")
	}
}

func TestIsManager(t *testing.T) {
	h := NewHandler(nil, map[string]bool{manager: true})
	if !h.IsManager(manager) || h.IsManager(other) || h.IsManager("") {
		t.Error("IsManager answered wrongly")
	}
	if NewHandler(nil, nil).IsManager(manager) {
		t.Error("an empty manager list must allow nobody")
	}
}

func TestPlainButtonSendsEventAndUpdatesMessage(t *testing.T) {
	fs := &fakeSite{}
	fs.eventReply = updatedJSON(t, "in_progress", "preview")
	h, s, fd := newHandler(t, fs)

	h.HandleButton(s, pressed(manager, ButtonID(Action{"c1", "in_progress"})))

	events := fs.callsTo("/events")
	if len(events) != 1 || events[0].Body != `{"status":"in_progress"}` || events[0].Actor != manager || events[0].Path != "/admin/commissions/c1/events" {
		t.Fatalf("events = %+v", events)
	}
	cb := fd.find("POST", "/callback")
	if len(cb) != 1 || callbackType(cb[0]) != float64(discordgo.InteractionResponseDeferredMessageUpdate) {
		t.Errorf("callback = %+v", cb)
	}
	edit := fd.find("PATCH", "/messages/@original")
	if len(edit) != 1 {
		t.Fatalf("edits = %d", len(edit))
	}
	body, _ := json.Marshal(edit[0].Body)
	if !strings.Contains(string(body), "In progress") || !strings.Contains(string(body), "commission:c1:preview") || strings.Contains(string(body), ":quoted") {
		t.Errorf("edit = %s", body)
	}
	line := fd.find("POST", "/channels/m1/messages")
	if len(line) != 1 || line[0].Body["content"] != "In progress by <@"+manager+">" {
		t.Errorf("thread line = %+v", line)
	}
	am := line[0].Body["allowed_mentions"].(map[string]any)
	if parse, _ := am["parse"].([]any); len(parse) != 0 {
		t.Errorf("thread line allows mentions: %v", am)
	}
}

func TestQuotedOpensNoteModalWithoutCallingSite(t *testing.T) {
	fs := &fakeSite{}
	h, s, fd := newHandler(t, fs)

	h.HandleButton(s, pressed(manager, ButtonID(Action{"c1", "quoted"})))
	h.HandleButton(s, pressed(manager, ButtonID(Action{"c1", "declined"})))

	if len(fs.callsTo("/events")) != 0 {
		t.Error("opening a modal must not send the event")
	}
	cb := fd.find("POST", "/callback")
	if len(cb) != 2 {
		t.Fatalf("callbacks = %d", len(cb))
	}
	for n, status := range []string{"quoted", "declined"} {
		data := cb[n].Body["data"].(map[string]any)
		if callbackType(cb[n]) != float64(discordgo.InteractionResponseModal) || data["custom_id"] != "commission-note:c1:"+status {
			t.Errorf("%s modal = %+v", status, cb[n].Body)
		}
		input := data["components"].([]any)[0].(map[string]any)["components"].([]any)[0].(map[string]any)
		if input["required"] == true {
			t.Errorf("%s note must be optional", status)
		}
	}
}

func TestModalSubmitSendsNoteAndPostsItInThread(t *testing.T) {
	fs := &fakeSite{eventReply: updatedJSON(t, "quoted", "deposit_paid", "declined")}
	h, s, fd := newHandler(t, fs)

	h.HandleModal(s, submitted(manager, ModalID(Action{"c1", "quoted"}), "  Rp1.5jt, 2 weeks  "))

	events := fs.callsTo("/events")
	if len(events) != 1 || events[0].Body != `{"note":"Rp1.5jt, 2 weeks","status":"quoted"}` {
		t.Fatalf("events = %+v", events)
	}
	line := fd.find("POST", "/channels/m1/messages")
	if len(line) != 1 || line[0].Body["content"] != "Quoted by <@"+manager+">: Rp1.5jt, 2 weeks" {
		t.Errorf("thread line = %+v", line)
	}
}

func TestModalSubmitWithoutNoteOmitsIt(t *testing.T) {
	fs := &fakeSite{eventReply: updatedJSON(t, "declined")}
	h, s, fd := newHandler(t, fs)

	h.HandleModal(s, submitted(manager, ModalID(Action{"c1", "declined"}), "   "))

	if ev := fs.callsTo("/events"); len(ev) != 1 || ev[0].Body != `{"status":"declined"}` {
		t.Errorf("events = %+v", ev)
	}
	if line := fd.find("POST", "/channels/m1/messages"); len(line) != 1 || line[0].Body["content"] != "Declined by <@"+manager+">" {
		t.Errorf("thread line = %+v", line)
	}
}

func TestSiteRefusalGoesToPresserOnly(t *testing.T) {
	fs := &fakeSite{eventStatusByCall: 400, eventReply: `{"error":"cannot go from final to quoted"}`}
	h, s, fd := newHandler(t, fs)

	h.HandleButton(s, pressed(manager, ButtonID(Action{"c1", "in_progress"})))

	follow := fd.find("POST", "/webhooks/2/tok")
	if len(follow) != 1 {
		t.Fatalf("followups = %d", len(follow))
	}
	if follow[0].Body["flags"] != float64(discordgo.MessageFlagsEphemeral) || !strings.Contains(follow[0].Body["content"].(string), "cannot go from final to quoted") {
		t.Errorf("followup = %+v", follow[0].Body)
	}
	if len(fd.find("PATCH", "/messages/@original")) != 0 || len(fd.find("POST", "/channels/m1/messages")) != 0 {
		t.Error("a refused change edited the message or wrote to the thread")
	}
}

func TestSiteDownGivesGenericEphemeralError(t *testing.T) {
	fs := &fakeSite{eventStatusByCall: 500, eventReply: `oops`}
	h, s, fd := newHandler(t, fs)

	h.HandleButton(s, pressed(manager, ButtonID(Action{"c1", "in_progress"})))

	follow := fd.find("POST", "/webhooks/2/tok")
	if len(follow) != 1 || strings.Contains(follow[0].Body["content"].(string), "oops") {
		t.Errorf("followup = %+v", follow)
	}
}

func TestThreadLine(t *testing.T) {
	if got := ThreadLine("deposit_paid", manager, ""); got != "Deposit paid by <@"+manager+">" {
		t.Errorf("line = %q", got)
	}
	if got := ThreadLine("final", manager, strings.Repeat("x", 3000)); len([]rune(got)) > 2000 {
		t.Errorf("line has %d runes", len([]rune(got)))
	}
}
