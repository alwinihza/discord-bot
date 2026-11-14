package commissions

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const voiceJSON = `{"id":"7b1f3c52-0a4e-4c1b-9d57-2f6a7e1d9a10","kind":"voice","createdAt":"2026-03-01T10:00:00Z","status":"received","next":["quoted","declined"],
"fields":{"name":"Rin","contact":"rin#0001","usage":"Game voice lines","commercial":true,"languages":["English","Japanese"],"newFutureField":"hello"},
"quote":{"_tag":"Priced","lines":[{"label":"Base licence","idr":1500000,"usd":95},{"label":"Rush","idr":250000,"usd":15.5}],"total":{"idr":1750000,"usd":110.5},"deposit":{"idr":875000,"usd":55.25}}}`

const studioJSON = `{"id":"studio-1","kind":"studio","createdAt":"2026-03-02T10:00:00Z","status":"quoted","next":["deposit_paid","declined"],
"fields":{"song":"Some Song","mixOnly":false},"quote":{"_tag":"Priced","lines":[{"label":"Cover","idr":3000000,"usd":190}],"total":{"idr":3000000,"usd":190},"deposit":null}}`

const endorseJSON = `{"id":"endorse-1","kind":"endorse","createdAt":"2026-03-03T10:00:00Z","status":"received","next":["quoted","declined"],
"fields":{"brand":"Acme","budget":null,"nested":{"a":1}},"quote":{"_tag":"ByCase","reason":"Depends on campaign scope"}}`

func decode(t *testing.T, raw string) site.Commission {
	t.Helper()
	var c site.Commission
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func fieldMap(e *discordgo.MessageEmbed) map[string]string {
	m := map[string]string{}
	for _, f := range e.Fields {
		m[f.Name] = f.Value
	}
	return m
}

func TestEmbedVoice(t *testing.T) {
	e := Embed(decode(t, voiceJSON))
	f := fieldMap(e)
	if e.Title != "Voice licensing" || f["Status"] != "Received" {
		t.Errorf("title=%q status=%q", e.Title, f["Status"])
	}
	want := map[string]string{
		"Name": "Rin", "Commercial": "Yes", "Languages": "English, Japanese",
		"New future field": "hello", "Usage": "Game voice lines",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("field %q = %q, want %q", k, f[k], v)
		}
	}
	for _, line := range []string{
		"Base licence: Rp1.500.000 / $95",
		"Rush: Rp250.000 / $15.50",
		"**Total: Rp1.750.000 / $110.50**",
		"Deposit: Rp875.000 / $55.25",
	} {
		if !strings.Contains(f["Quote"], line) {
			t.Errorf("quote %q lacks %q", f["Quote"], line)
		}
	}
	if e.Fields[1].Name != "Name" || e.Fields[2].Name != "Contact" {
		t.Errorf("name and contact should lead, got %q then %q", e.Fields[1].Name, e.Fields[2].Name)
	}
	if e.Timestamp != "2026-03-01T10:00:00Z" {
		t.Errorf("timestamp = %q", e.Timestamp)
	}
}

func TestEmbedStudioHasNoDepositLine(t *testing.T) {
	e := Embed(decode(t, studioJSON))
	f := fieldMap(e)
	if e.Title != "Paopoa Studio cover" || f["Status"] != "Quoted" || f["Mix only"] != "No" {
		t.Errorf("title=%q fields=%v", e.Title, f)
	}
	if strings.Contains(f["Quote"], "Deposit") || !strings.Contains(f["Quote"], "Total: Rp3.000.000 / $190") {
		t.Errorf("quote = %q", f["Quote"])
	}
}

func TestEmbedEndorseByCaseAndOddValues(t *testing.T) {
	e := Embed(decode(t, endorseJSON))
	f := fieldMap(e)
	if e.Title != "Brand endorsement" || f["Quote"] != "Priced case by case: Depends on campaign scope" {
		t.Errorf("title=%q quote=%q", e.Title, f["Quote"])
	}
	if f["Budget"] != "-" || f["Nested"] != `{"a":1}` {
		t.Errorf("odd values rendered as budget=%q nested=%q", f["Budget"], f["Nested"])
	}
}

func TestEmbedToleratesMissingAndUnknownParts(t *testing.T) {
	c := decode(t, `{"id":"x1","kind":"newkind","status":"on_hold","createdAt":"yesterday","quote":{"_tag":"Mystery"}}`)
	e := Embed(c)
	f := fieldMap(e)
	if e.Title != "Newkind" || f["Status"] != "On hold" || e.Timestamp != "" {
		t.Errorf("title=%q status=%q timestamp=%q", e.Title, f["Status"], e.Timestamp)
	}
	if !strings.Contains(f["Quote"], "Unrecognised") {
		t.Errorf("quote = %q", f["Quote"])
	}
	if got := fieldMap(Embed(decode(t, `{"id":"x1","kind":"voice","status":"received"}`)))["Quote"]; got != "None" {
		t.Errorf("null quote = %q", got)
	}
}

func TestEmbedStaysWithinDiscordLimits(t *testing.T) {
	fields := map[string]any{}
	for i := 0; i < 40; i++ {
		fields[strings.Repeat("k", i+1)] = strings.Repeat("v", 3000)
	}
	c := commission("big")
	c.Fields = fields
	e := Embed(c)
	total := len(e.Title) + len(e.Footer.Text)
	for _, f := range e.Fields {
		if utf8.RuneCountInString(f.Value) > 1024 || utf8.RuneCountInString(f.Name) > 256 {
			t.Fatalf("field %q too long", f.Name)
		}
		total += len(f.Name) + len(f.Value)
	}
	if len(e.Fields) > 25 || total > 6000 {
		t.Errorf("fields=%d total=%d", len(e.Fields), total)
	}
	if e.Description == "" {
		t.Error("dropped fields should be mentioned")
	}
}

func TestComponentsFollowNext(t *testing.T) {
	c := decode(t, voiceJSON)
	rows := Components(c)
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	buttons := rows[0].(discordgo.ActionsRow).Components
	if len(buttons) != 2 {
		t.Fatalf("buttons = %d", len(buttons))
	}
	b := buttons[0].(discordgo.Button)
	if b.Label != "Quoted" || b.CustomID != "commission:"+c.ID+":quoted" || len(b.CustomID) > 100 {
		t.Errorf("button = %+v", b)
	}
	if got := buttons[1].(discordgo.Button); got.Style != discordgo.DangerButton {
		t.Errorf("decline style = %v", got.Style)
	}

	c.Next = []string{"deposit_paid", "in_progress"}
	if l := Components(c)[0].(discordgo.ActionsRow).Components[0].(discordgo.Button).Label; l != "Deposit paid" {
		t.Errorf("label = %q", l)
	}
	c.Next = nil
	if rows := Components(c); rows == nil || len(rows) != 0 {
		t.Errorf("final commission should clear buttons with an empty, non-nil slice, got %#v", rows)
	}
}

func TestStatusLabels(t *testing.T) {
	for status, want := range map[string]string{"deposit_paid": "Deposit paid", "in_progress": "In progress", "final": "Final", "brand_new": "Brand new"} {
		if got := StatusLabel(status); got != want {
			t.Errorf("StatusLabel(%q) = %q, want %q", status, got, want)
		}
	}
}
