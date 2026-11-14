package commissions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	maxEmbedFields = 24
	embedBudget    = 5500
	maxFieldValue  = 1024
)

var statusLabels = map[string]string{
	"received":     "Received",
	"quoted":       "Quoted",
	"deposit_paid": "Deposit paid",
	"in_progress":  "In progress",
	"preview":      "Preview",
	"draft":        "Draft",
	"revision":     "Revision",
	"final":        "Final",
	"declined":     "Declined",
}

var kindLabels = map[string]string{
	"voice":   "Voice licensing",
	"studio":  "Paopoa Studio cover",
	"endorse": "Brand endorsement",
}

var statusColors = map[string]int{
	"received": 0x95a5a6, "quoted": 0x3498db, "deposit_paid": 0xf1c40f, "in_progress": 0xe67e22,
	"preview": 0x9b59b6, "draft": 0x9b59b6, "revision": 0xe91e63, "final": 0x2ecc71, "declined": 0xe74c3c,
}

func StatusLabel(status string) string {
	if l, ok := statusLabels[status]; ok {
		return l
	}
	return humanize(status)
}

func kindLabel(kind string) string {
	if l, ok := kindLabels[kind]; ok {
		return l
	}
	return humanize(kind)
}

func humanize(key string) string {
	var b strings.Builder
	for n, r := range key {
		switch {
		case r == '_' || r == '-':
			b.WriteRune(' ')
		case unicode.IsUpper(r) && n > 0:
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "Unknown"
	}
	return strings.ToUpper(out[:1]) + out[1:]
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func formatIDR(v float64) string {
	sign, n := "", int64(v+0.5)
	if v < 0 {
		sign, n = "-", int64(-v+0.5)
	}
	digits := fmt.Sprint(n)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	return sign + "Rp" + b.String()
}

func formatUSD(v float64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	if v == float64(int64(v)) {
		return fmt.Sprintf("%s$%d", sign, int64(v))
	}
	return fmt.Sprintf("%s$%.2f", sign, v)
}

func formatMoney(idr, usd float64) string { return formatIDR(idr) + " / " + formatUSD(usd) }

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		if strings.TrimSpace(x) == "" {
			return "-"
		}
		return x
	case bool:
		if x {
			return "Yes"
		}
		return "No"
	case float64:
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, formatValue(item))
		}
		if len(parts) == 0 {
			return "-"
		}
		return strings.Join(parts, ", ")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "-"
	}
	return string(raw)
}

var leadingKeys = []string{"name", "contact"}

func orderedKeys(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rank := func(k string) int {
		for n, lead := range leadingKeys {
			if k == lead {
				return n
			}
		}
		return len(leadingKeys)
	}
	sort.SliceStable(keys, func(a, b int) bool { return rank(keys[a]) < rank(keys[b]) })
	return keys
}

func quoteField(q *site.Quote) *discordgo.MessageEmbedField {
	if q == nil {
		return &discordgo.MessageEmbedField{Name: "Quote", Value: "None"}
	}
	switch q.Tag {
	case "ByCase":
		return &discordgo.MessageEmbedField{Name: "Quote", Value: "Priced case by case: " + formatValue(q.Reason)}
	case "Priced":
		var lines []string
		for _, l := range q.Lines {
			lines = append(lines, fmt.Sprintf("%s: %s", l.Label, formatMoney(l.IDR, l.USD)))
		}
		if q.Total != nil {
			lines = append(lines, "**Total: "+formatMoney(q.Total.IDR, q.Total.USD)+"**")
		}
		if q.Deposit != nil {
			lines = append(lines, "Deposit: "+formatMoney(q.Deposit.IDR, q.Deposit.USD))
		}
		return &discordgo.MessageEmbedField{Name: "Quote", Value: strings.Join(lines, "\n")}
	}
	return &discordgo.MessageEmbedField{Name: "Quote", Value: "Unrecognised quote format (" + truncate(q.Tag, 40) + ")"}
}

// Embed renders a commission. Unknown field keys show as label and value pairs.
func Embed(c site.Commission) *discordgo.MessageEmbed {
	color, ok := statusColors[c.Status]
	if !ok {
		color = 0x95a5a6
	}
	embed := &discordgo.MessageEmbed{
		Title:  kindLabel(c.Kind),
		Color:  color,
		Footer: &discordgo.MessageEmbedFooter{Text: footerPrefix + c.ID},
	}
	if t, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
		embed.Timestamp = t.UTC().Format(time.RFC3339)
	}
	fields := []*discordgo.MessageEmbedField{{Name: "Status", Value: StatusLabel(c.Status)}}
	for _, k := range orderedKeys(c.Fields) {
		fields = append(fields, &discordgo.MessageEmbedField{Name: truncate(humanize(k), 256), Value: formatValue(c.Fields[k])})
	}
	fields = append(fields, quoteField(c.Quote))

	used := len(embed.Title) + len(embed.Footer.Text)
	for _, f := range fields {
		if len(embed.Fields) == maxEmbedFields {
			break
		}
		f.Value = truncate(f.Value, maxFieldValue)
		if used+len(f.Name)+len(f.Value) > embedBudget {
			break
		}
		used += len(f.Name) + len(f.Value)
		embed.Fields = append(embed.Fields, f)
	}
	if len(embed.Fields) < len(fields) {
		embed.Description = "Some fields are not shown here."
	}
	return embed
}

// Components renders one button per allowed next status. The result is never nil, so an edit clears old buttons.
func Components(c site.Commission) []discordgo.MessageComponent {
	rows := []discordgo.MessageComponent{}
	var row discordgo.ActionsRow
	for _, status := range c.Next {
		if !statusName.MatchString(status) || !site.ValidCommissionID(c.ID) {
			continue
		}
		style := discordgo.PrimaryButton
		switch status {
		case "declined":
			style = discordgo.DangerButton
		case "final":
			style = discordgo.SuccessButton
		}
		row.Components = append(row.Components, discordgo.Button{
			Label: StatusLabel(status), Style: style, CustomID: ButtonID(Action{ID: c.ID, Status: status}),
		})
		if len(row.Components) == 5 {
			rows = append(rows, row)
			row = discordgo.ActionsRow{}
		}
	}
	if len(row.Components) > 0 {
		rows = append(rows, row)
	}
	if len(rows) > 5 {
		rows = rows[:5]
	}
	return rows
}
