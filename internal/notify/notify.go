// Package notify delivers alert messages to one outbound webhook. It knows four shapes
// (ntfy, Gotify, Discord, generic JSON) and nothing about why a message is sent.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Preset names the receiver's wire format.
type Preset string

const (
	Ntfy    Preset = "ntfy"
	Gotify  Preset = "gotify"
	Discord Preset = "discord"
	Generic Preset = "generic"
)

// Config is the admin's webhook. It is stored sealed; the token never leaves the process.
type Config struct {
	Preset Preset `json:"preset"`
	URL    string `json:"url"`
	Token  string `json:"token,omitempty"`
}

var ErrBadPreset = errors.New("notify: preset must be ntfy, gotify, discord or generic")

// Validate checks a config an admin saved. allowHTTP mirrors KYPULSE_ALERT_ALLOW_HTTP.
func Validate(c Config, allowHTTP bool) error {
	switch c.Preset {
	case Ntfy, Gotify, Discord, Generic:
	default:
		return ErrBadPreset
	}
	return egress.ValidateURL(c.URL, allowHTTP)
}

// Message is one alert. It carries names and codes only: no log lines, no user names, no
// IPs, because hosted ntfy and Discord are third parties.
type Message struct {
	App      string    `json:"app"`
	State    string    `json:"state"`
	Previous string    `json:"previous"`
	Reason   string    `json:"reason,omitempty"`
	Time     time.Time `json:"time"`
	URL      string    `json:"url"` // link back to the kyPulse app page
	Reminder bool      `json:"reminder,omitempty"`
	Test     bool      `json:"test,omitempty"`
}

// Title is the one-line summary every preset uses.
func (m Message) Title() string {
	switch {
	case m.Test:
		return "kyPulse: test alert"
	case m.Reminder:
		return fmt.Sprintf("kyPulse: %s still %s", m.App, m.State)
	case m.State == "ok":
		return fmt.Sprintf("kyPulse: %s recovered", m.App)
	}
	return fmt.Sprintf("kyPulse: %s is %s", m.App, m.State)
}

func (m Message) text() string {
	s := m.Title()
	if m.Reason != "" {
		s += " (" + m.Reason + ")"
	}
	return s + " at " + m.Time.UTC().Format(time.RFC3339) + "\n" + m.URL
}

// Request is the wire form of one message for one preset.
type Request struct {
	URL         string
	ContentType string
	Headers     map[string]string
	Body        []byte
}

// Build renders m for c's preset. Tokens ride in headers, never in the URL, so they do not
// land in the receiver's access log.
func Build(c Config, m Message) (Request, error) {
	h := map[string]string{}
	switch c.Preset {
	case Ntfy:
		h["Title"] = m.Title()
		h["Priority"] = "3"
		if m.State == "down" && !m.Reminder {
			h["Priority"] = "5"
		}
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
		return Request{URL: c.URL, ContentType: "text/plain", Headers: h, Body: []byte(m.text())}, nil
	case Gotify:
		if c.Token != "" {
			h["X-Gotify-Key"] = c.Token
		}
		prio := 5
		if m.State == "down" {
			prio = 8
		}
		body, _ := json.Marshal(map[string]any{"title": m.Title(), "message": m.text(), "priority": prio})
		return Request{URL: strings.TrimRight(c.URL, "/") + "/message", ContentType: "application/json", Headers: h, Body: body}, nil
	case Discord:
		body, _ := json.Marshal(map[string]string{"content": m.text()})
		return Request{URL: c.URL, ContentType: "application/json", Headers: h, Body: body}, nil
	case Generic:
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
		body, _ := json.Marshal(m)
		return Request{URL: c.URL, ContentType: "application/json", Headers: h, Body: body}, nil
	}
	return Request{}, ErrBadPreset
}

// Poster is what Notifier needs from egress; a fake stands in for tests.
type Poster interface {
	Post(ctx context.Context, url, contentType string, body []byte, headers map[string]string) (*egress.Response, error)
}

// Notifier sends with retries. Backoff is exported so tests do not wait.
type Notifier struct {
	Post    Poster
	Backoff []time.Duration // waits between attempts; len+1 attempts in total
}

// DefaultBackoff gives four attempts over about twenty seconds.
var DefaultBackoff = []time.Duration{2 * time.Second, 6 * time.Second, 12 * time.Second}

// ErrRejected is a 4xx other than 429: the receiver understood and refused, so retrying
// would only repeat the refusal.
var ErrRejected = errors.New("notify: receiver rejected the message")

// Send delivers m, retrying transport errors, 429 and 5xx. It returns the last error.
func (n *Notifier) Send(ctx context.Context, c Config, m Message) error {
	req, err := Build(c, m)
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; ; attempt++ {
		resp, err := n.Post.Post(ctx, req.URL, req.ContentType, req.Body, req.Headers)
		switch {
		case err != nil:
			last = err
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode == 429 || resp.StatusCode >= 500:
			last = fmt.Errorf("notify: receiver answered %d", resp.StatusCode)
		default:
			return fmt.Errorf("%w: %d", ErrRejected, resp.StatusCode)
		}
		if attempt >= len(n.Backoff) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(n.Backoff[attempt]):
		}
	}
}
