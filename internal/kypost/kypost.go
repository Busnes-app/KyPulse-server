// Package kypost is KyPulse's client for alert mail through a member's KyPost mailbox
// (docs/KYPOST.md): one signed route, kypulse.alert.send, under the pair secret KyIdentity
// gives both sides. KyPost owns every rule about who may receive mail; this side only names
// the member and the recipients.
package kypost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/syncauth"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

const (
	EventSend = "kypulse.alert.send"
	SendPath  = "/api/suite/kypulse/alert"
	// Field limits, mirrored from the contract so a message KyPost would refuse never leaves.
	MaxRecipients = 11
	MaxSubject    = 150
	MaxText       = 8 << 10
)

// SendRequest is the whole body: KyPost refuses any other field.
type SendRequest struct {
	Sub        string   `json:"sub"`
	Issuer     string   `json:"issuer"`
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	Text       string   `json:"text"`
}

// ErrRetry is a failure worth retrying: the network, 429 or a 5xx.
var ErrRetry = errors.New("kypost: try again later")

// ErrNotPaired is a send while KyIdentity has not paired KyPost.
var ErrNotPaired = errors.New("kypost: not paired")

// Refused is KyPost refusing an alert for good; Code is its reason.
type Refused struct{ Code string }

func (r Refused) Error() string { return "kypost refused: " + r.Code }

// Poster is the slice of egress.Client the client uses.
type Poster interface {
	Post(ctx context.Context, url, contentType string, body []byte, headers map[string]string) (*egress.Response, error)
}

// send posts req to base under event id, which stays the same across retries of one alert.
func send(ctx context.Context, post Poster, base string, secret []byte, now time.Time, id string, req SendRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	h, err := syncauth.Sign(secret, now, EventSend, id, body)
	if err != nil {
		return err
	}
	resp, err := post.Post(ctx, base+SendPath, "application/json", body, map[string]string{
		syncauth.HeaderSignature: h.Signature, syncauth.HeaderTimestamp: h.Timestamp,
		syncauth.HeaderEventType: h.EventType, syncauth.HeaderEventID: h.EventID,
	})
	if err != nil {
		return fmt.Errorf("%w: %s", ErrRetry, egress.Cause(err))
	}
	switch c := resp.StatusCode; {
	case c >= 200 && c < 300:
		return nil
	case c == 429 || c >= 500:
		return fmt.Errorf("%w: status %d", ErrRetry, c)
	}
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(resp.Body, &out)
	if !validCode(out.Code) {
		out.Code = fmt.Sprintf("status_%d", resp.StatusCode)
	}
	return Refused{Code: out.Code}
}

// validCode admits a short lowercase reason code; anything else from the peer is dropped.
func validCode(s string) bool {
	return s != "" && len(s) <= 64 && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789_") == ""
}

// Reason is the code KyPulse stores and shows for a send error: KyPost's refusal code,
// not_paired, retry_exhausted or cancelled. Never an address or the message.
func Reason(err error) string {
	var r Refused
	switch {
	case err == nil:
		return ""
	case errors.As(err, &r):
		return r.Code
	case errors.Is(err, ErrNotPaired):
		return "not_paired"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	case errors.Is(err, ErrRetry):
		return "unavailable"
	}
	return "network"
}
