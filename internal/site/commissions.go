package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

var (
	ErrCommissionNotFound = errors.New("commission not found")
	ErrCommissionConflict = errors.New("commission is already posted somewhere else")
	ErrInvalidCommission  = errors.New("invalid commission id")
)

type Money struct {
	IDR float64 `json:"idr"`
	USD float64 `json:"usd"`
}

type QuoteLine struct {
	Label string  `json:"label"`
	IDR   float64 `json:"idr"`
	USD   float64 `json:"usd"`
}

// Quote is the site's tagged union. Tag is "Priced" or "ByCase"; any other tag is kept so the bot can say it could not read it.
type Quote struct {
	Tag     string      `json:"_tag"`
	Lines   []QuoteLine `json:"lines"`
	Total   *Money      `json:"total"`
	Deposit *Money      `json:"deposit"`
	Reason  string      `json:"reason"`
}

type Commission struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	CreatedAt string         `json:"createdAt"`
	Status    string         `json:"status"`
	Next      []string       `json:"next"`
	Fields    map[string]any `json:"fields"`
	Quote     *Quote         `json:"quote"`
	ChannelID string         `json:"channelId"`
	MessageID string         `json:"messageId"`
}

var commissionID = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// ValidCommissionID reports whether id is safe to put in a path and a Discord custom ID.
func ValidCommissionID(id string) bool { return commissionID.MatchString(id) }

// Unposted lists the commissions that have no Discord message yet, oldest first.
func (c *Client) Unposted(ctx context.Context) ([]Commission, error) {
	var out []Commission
	err := c.roundTrip(ctx, http.MethodGet, "/admin/commissions/unposted", "", nil, &out, commissionError)
	return out, err
}

// MarkPosted records where the commission was posted. ErrCommissionConflict means another message already holds it.
func (c *Client) MarkPosted(ctx context.Context, id, channelID, messageID string) error {
	if !ValidCommissionID(id) {
		return ErrInvalidCommission
	}
	body := map[string]string{"channelId": channelID, "messageId": messageID}
	return c.roundTrip(ctx, http.MethodPost, "/admin/commissions/"+url.PathEscape(id)+"/posted", "", body, nil, commissionError)
}

// SendEvent moves the commission to status on behalf of user. note may be empty.
func (c *Client) SendEvent(ctx context.Context, user, id, status, note string) (Commission, error) {
	var out Commission
	if !discordID.MatchString(user) {
		return out, ErrInvalidUser
	}
	if !ValidCommissionID(id) {
		return out, ErrInvalidCommission
	}
	body := map[string]string{"status": status}
	if note != "" {
		body["note"] = note
	}
	err := c.roundTrip(ctx, http.MethodPost, "/admin/commissions/"+url.PathEscape(id)+"/events", user, body, &out, commissionError)
	return out, err
}

func commissionError(resp *http.Response, raw []byte) error {
	var fail struct {
		Error string `json:"error"`
	}
	json.Unmarshal(raw, &fail)
	switch resp.StatusCode {
	case http.StatusBadRequest:
		return &RuleError{Message: fail.Error}
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrCommissionNotFound
	case http.StatusConflict:
		return ErrCommissionConflict
	}
	return fmt.Errorf("call site: %s", resp.Status)
}
