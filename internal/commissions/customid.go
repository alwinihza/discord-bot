package commissions

import (
	"regexp"
	"strings"

	"github.com/alwinihza/discord-bot/internal/site"
)

const (
	buttonPrefix = "commission:"
	modalPrefix  = "commission-note:"
)

var statusName = regexp.MustCompile(`^[a-z][a-z_]{0,31}$`)

// Action is one status change on one commission, as carried by a button or a note modal.
type Action struct{ ID, Status string }

func ButtonID(a Action) string { return buttonPrefix + a.ID + ":" + a.Status }
func ModalID(a Action) string  { return modalPrefix + a.ID + ":" + a.Status }

func ParseButtonID(customID string) (Action, bool) { return parse(customID, buttonPrefix) }
func ParseModalID(customID string) (Action, bool)  { return parse(customID, modalPrefix) }

// OwnsComponent reports whether customID belongs to this feature, even when it is malformed.
func OwnsComponent(customID string) bool {
	return strings.HasPrefix(customID, buttonPrefix) || strings.HasPrefix(customID, modalPrefix)
}

func parse(customID, prefix string) (Action, bool) {
	rest, ok := strings.CutPrefix(customID, prefix)
	if !ok {
		return Action{}, false
	}
	id, status, ok := strings.Cut(rest, ":")
	if !ok || !site.ValidCommissionID(id) || !statusName.MatchString(status) {
		return Action{}, false
	}
	return Action{ID: id, Status: status}, true
}

// needsNote lists the statuses whose button asks for an optional note first.
func needsNote(status string) bool { return status == "quoted" || status == "declined" }

// ParseManagerIDs reads a comma-separated list of Discord user ids.
// Entries that are not snowflakes are returned in bad and left out of the set.
func ParseManagerIDs(csv string) (managers map[string]bool, bad []string) {
	managers = map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case snowflake.MatchString(part):
			managers[part] = true
		default:
			bad = append(bad, part)
		}
	}
	return managers, bad
}

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)
