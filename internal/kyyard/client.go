package kyyard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// HTTP is what the client needs from the egress guard; *egress.Client satisfies it.
type HTTP interface {
	GetWith(ctx context.Context, rawURL string, headers map[string]string) (*egress.Response, error)
	Post(ctx context.Context, rawURL, contentType string, body []byte, headers map[string]string) (*egress.Response, error)
}

var (
	ErrPairingRefused = errors.New("kyyard: pairing refused")
	ErrRateLimited    = errors.New("kyyard: too many pairing attempts; wait a minute")
	ErrUnauthorized   = errors.New("kyyard: token refused")
)

// StatusError is a non-2xx answer other than the ones above; the body is never kept.
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("kyyard: status %d", e.Code) }

var codeRe = regexp.MustCompile(`^[0-9]{6}$`)

// Claim exchanges a pairing code for a token. The code and the token never reach a log.
func Claim(ctx context.Context, h HTTP, baseURL, code string) (Config, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if _, err := url.Parse(base); err != nil || base == "" {
		return Config{}, fmt.Errorf("kyyard: bad url")
	}
	if !codeRe.MatchString(code) {
		return Config{}, ErrPairingRefused
	}
	body, _ := json.Marshal(map[string]string{"pairing_code": code, "service_name": "kypulse"})
	resp, err := h.Post(ctx, base+"/api/service-tokens/claim", "application/json", body, nil)
	if err != nil {
		return Config{}, err
	}
	switch resp.StatusCode {
	case 200:
	case 403:
		return Config{}, ErrPairingRefused
	case 429:
		return Config{}, ErrRateLimited
	default:
		return Config{}, StatusError{resp.StatusCode}
	}
	var out struct {
		Token        string `json:"token"`
		Organization struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil || out.Token == "" || out.Organization.ID == "" {
		return Config{}, fmt.Errorf("kyyard: claim answer not understood")
	}
	return Config{URL: base, Token: out.Token, OrganizationID: out.Organization.ID, OrganizationName: out.Organization.Name}, nil
}

// Endpoint is what kyPulse keeps of a KyYard endpoint.
type Endpoint struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Runtime    string     `json:"runtime"`
	State      string     `json:"state"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// Container is one inventory entry; Status is Docker's human text ("Up 3 hours (healthy)",
// "Exited (137) 2 hours ago"), which is where health and exit code come from.
type Container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
}

type Inventory struct {
	EndpointID string
	State      string
	ObservedAt time.Time
	ReceivedAt time.Time
	Containers []Container
}

type Sample struct {
	ContainerID  string    `json:"container_id"`
	ObservedAt   time.Time `json:"observed_at"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemoryBytes  int64     `json:"memory_bytes"`
	MemoryLimit  int64     `json:"memory_limit"`
	RestartCount int64     `json:"restart_count"`
}

// Client reads one organization with a bearer token.
type Client struct {
	HTTP    HTTP
	LogHTTP HTTP
	Config  Config
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	resp, err := c.HTTP.GetWith(ctx, c.Config.URL+"/api/organizations/"+url.PathEscape(c.Config.OrganizationID)+path, map[string]string{"Authorization": "Bearer " + c.Config.Token})
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == 401:
		return ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return StatusError{resp.StatusCode}
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("kyyard: answer not understood: %w", err)
	}
	return nil
}

func (c *Client) Endpoints(ctx context.Context) ([]Endpoint, error) {
	var out []Endpoint
	if err := c.get(ctx, "/endpoints?limit=200", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Inventory(ctx context.Context, endpointID string) (Inventory, error) {
	var raw struct {
		EndpointID string    `json:"endpoint_id"`
		State      string    `json:"state"`
		ObservedAt time.Time `json:"observed_at"`
		ReceivedAt time.Time `json:"received_at"`
		Snapshot   struct {
			Containers []Container `json:"containers"`
		} `json:"snapshot"`
	}
	if err := c.get(ctx, "/endpoints/"+url.PathEscape(endpointID)+"/inventory", &raw); err != nil {
		return Inventory{}, err
	}
	return Inventory{EndpointID: raw.EndpointID, State: raw.State, ObservedAt: raw.ObservedAt, ReceivedAt: raw.ReceivedAt, Containers: raw.Snapshot.Containers}, nil
}

func (c *Client) Samples(ctx context.Context, endpointID string) ([]Sample, error) {
	var out []Sample
	if err := c.get(ctx, "/endpoints/"+url.PathEscape(endpointID)+"/samples", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Reason names a pull failure in a fixed vocabulary for the status view and the log.
func Reason(err error) string {
	var se StatusError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAuditCursorUnsupported):
		return "audit_cursor_unsupported"
	case errors.Is(err, ErrAuditPage):
		return "audit_invalid_page"
	case errors.Is(err, ErrLogResponse):
		return "logs_invalid_response"
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrPairingRefused):
		return "pairing_refused" // egress already uses "refused" for connection refused
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.As(err, &se):
		return fmt.Sprintf("status_%d", se.Code)
	}
	if c := egress.Cause(err); c != "" {
		return c
	}
	return "network"
}
