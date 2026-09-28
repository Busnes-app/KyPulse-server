package kyyard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

const MaxLogResponse = 32 << 20

var (
	ErrAuditCursorUnsupported = errors.New("kyyard: audit cursor unsupported")
	ErrAuditPage              = errors.New("kyyard: invalid audit page")
	ErrLogResponse            = errors.New("kyyard: invalid log response")
)

type LogPage struct {
	Lines  []ingest.Record
	Notice string
	Capped bool
}
type AuditPage struct {
	Items       []AuditEvent `json:"items"`
	NextAfterID int64        `json:"next_after_id"`
}
type AuditEvent struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Result    string    `json:"result"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) Logs(ctx context.Context, endpointID, containerID, since string) (LogPage, error) {
	query := url.Values{"tail": {"1000"}, "timestamps": {"1"}, "follow": {"0"}}
	if since != "" {
		query.Set("since", since)
	}
	path := "/api/organizations/" + url.PathEscape(c.Config.OrganizationID) + "/endpoints/" + url.PathEscape(endpointID) + "/containers/" + url.PathEscape(containerID) + "/logs?" + query.Encode()
	resp, err := c.LogHTTP.GetWith(ctx, c.Config.URL+path, map[string]string{"Authorization": "Bearer " + c.Config.Token})
	if err != nil {
		return LogPage{}, err
	}
	if resp.StatusCode == 401 {
		return LogPage{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return LogPage{}, StatusError{resp.StatusCode}
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "text/plain" {
		return LogPage{}, ErrLogResponse
	}
	return parseLogs(resp.Body, resp.Header.Get("X-KyYard-Notice-Token"))
}

// parseLogs walks bounded response bytes without allocating a slice of every wire line.
// Only a matching per-response token authorizes a server notice; forged notices are logs.
func parseLogs(body []byte, token string) (LogPage, error) {
	if len(body) > MaxLogResponse {
		return LogPage{}, egress.ErrBodyTooLarge
	}
	p := LogPage{}
	var notices []string
	noticeBytes := 0
	prefix := "--- kyyard " + token + ": "
	for len(body) > 0 {
		raw, rest, found := bytes.Cut(body, []byte{'\n'})
		body = rest
		if !found {
			body = nil
		}
		raw = bytes.TrimSuffix(raw, []byte{'\r'})
		if token != "" && len(raw) >= len(prefix)+4 && bytes.HasPrefix(raw, []byte(prefix)) && bytes.HasSuffix(raw, []byte(" ---")) {
			// Bound all retained notices together, including a malicious upstream's repetition.
			if noticeBytes < ingest.MaxLineBytes {
				text := raw[len(prefix) : len(raw)-4]
				text = text[:min(len(text), ingest.MaxLineBytes-noticeBytes)]
				notices = append(notices, strings.ToValidUTF8(string(text), "\ufffd"))
				noticeBytes += len(notices[len(notices)-1]) + 2
			}
			continue
		}
		if len(p.Lines) >= ingest.MaxRecords {
			p.Capped = true
			continue
		}
		r := ingest.Record{}
		stamp, line, ok := bytes.Cut(raw, []byte{' '})
		if ok {
			if ts, err := time.Parse(time.RFC3339Nano, string(stamp)); err == nil {
				r.Time = ts
				raw = line
			}
		}
		if len(raw) > ingest.MaxLineBytes {
			raw = raw[:ingest.MaxLineBytes]
			r.Truncated = true
		}
		r.Line = strings.ToValidUTF8(string(raw), "\ufffd")
		if len(r.Line) > ingest.MaxLineBytes {
			r.Line = r.Line[:ingest.MaxLineBytes]
			for !utf8.ValidString(r.Line) {
				r.Line = r.Line[:len(r.Line)-1]
			}
			r.Truncated = true
		}
		if r.Truncated && len(notices) == 0 {
			notices = append(notices, "history may be incomplete: a log line was truncated")
		}
		p.Lines = append(p.Lines, r)
	}
	if len(p.Lines) == ingest.MaxRecords {
		p.Capped = true
	}
	if p.Capped {
		notices = append([]string{"history may be incomplete: pull reached 1000 lines"}, notices...)
	}
	p.Notice = strings.Join(notices, "; ")
	if len(p.Notice) > ingest.MaxLineBytes {
		p.Notice = p.Notice[:ingest.MaxLineBytes]
		for !utf8.ValidString(p.Notice) {
			p.Notice = p.Notice[:len(p.Notice)-1]
		}
	}
	return p, nil
}

func (c *Client) AuditAfter(ctx context.Context, afterID int64) (AuditPage, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/audit?after_id="+strconv.FormatInt(afterID, 10)+"&limit=200", &raw); err != nil {
		return AuditPage{}, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		return AuditPage{}, ErrAuditCursorUnsupported
	}
	// Pointers distinguish missing/null fields from a legitimate empty cursor page.
	var wire struct {
		Items *[]AuditEvent `json:"items"`
		Next  *int64        `json:"next_after_id"`
	}
	if json.Unmarshal(raw, &wire) != nil || wire.Items == nil || wire.Next == nil || len(*wire.Items) > 200 {
		return AuditPage{}, ErrAuditPage
	}
	last := afterID
	for _, item := range *wire.Items {
		if item.ID <= last || item.CreatedAt.IsZero() || item.Action == "" {
			return AuditPage{}, ErrAuditPage
		}
		last = item.ID
	}
	if *wire.Next != last {
		return AuditPage{}, ErrAuditPage
	}
	return AuditPage{Items: *wire.Items, NextAfterID: last}, nil
}
