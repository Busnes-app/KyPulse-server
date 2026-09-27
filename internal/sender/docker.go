package sender

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

const maxDockerFrame = 1 << 20

var dockerAPIVersion = regexp.MustCompile(`^1\.[0-9]+$`)

type dockerClient struct {
	http    *http.Client
	version string
}

func newDockerClient(socket string) (*dockerClient, error) {
	if !filepath.IsAbs(socket) {
		return nil, errors.New("sender: Docker socket must be absolute")
	}
	transport := &http.Transport{ResponseHeaderTimeout: 10 * time.Second, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &dockerClient{http: &http.Client{Transport: transport}}, nil
}

func (d *dockerClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("sender: Docker %s: HTTP %d", path, resp.StatusCode)
	}
	return resp, nil
}

func (d *dockerClient) negotiate(ctx context.Context) error {
	resp, err := d.get(ctx, "/version")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var v struct {
		APIVersion    string
		MinAPIVersion string
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&v); err != nil {
		return fmt.Errorf("sender: Docker version: %w", err)
	}
	if !dockerAPIVersion.MatchString(v.APIVersion) {
		return errors.New("sender: unsupported Docker API version")
	}
	d.version = "/v" + v.APIVersion
	return nil
}

func (d *dockerClient) inspect(ctx context.Context, name string) (string, bool, error) {
	resp, err := d.get(ctx, d.version+"/containers/"+url.PathEscape(name)+"/json")
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	var info struct {
		ID     string
		Config struct{ Tty bool }
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return "", false, err
	}
	if len(info.ID) != 64 {
		return "", false, errors.New("sender: Docker inspect returned invalid container ID")
	}
	for _, c := range info.ID {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", false, errors.New("sender: Docker inspect returned invalid container ID")
		}
	}
	return info.ID, info.Config.Tty, nil
}

// ResolveDocker pins a configured name to the immutable container ID before selecting checkpoints.
func ResolveDocker(ctx context.Context, socket, name string) (string, error) {
	d, err := newDockerClient(socket)
	if err != nil {
		return "", err
	}
	defer d.http.CloseIdleConnections()
	if err = d.negotiate(ctx); err != nil {
		return "", err
	}
	id, _, err := d.inspect(ctx, name)
	return id, err
}

// ReadDocker follows one container. The start map is keyed by ID and stream.
// Docker's `since` is second-granular. Equal-time lines replay because Docker
// cannot prove that earlier lines at that timestamp still exist after rotation.
func ReadDocker(ctx context.Context, socket, container string, start map[string]Position, out chan<- Item) error {
	return readDocker(ctx, socket, container, start, nil, out)
}

// FollowDocker reconnects a completed follow response, retaining emitted positions in
// memory. The delivery worker persists them only after acknowledgement.
func FollowDocker(ctx context.Context, socket, container string, start map[string]Position, out chan<- Item) error {
	local := make(map[string]Position, len(start))
	for key, p := range start {
		local[key] = p
	}
	for {
		if err := readDocker(ctx, socket, container, local, local, out); err != nil {
			return err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func readDocker(ctx context.Context, socket, container string, start, latest map[string]Position, out chan<- Item) error {
	if container == "" {
		return errors.New("sender: empty Docker container")
	}
	d, err := newDockerClient(socket)
	if err != nil {
		return err
	}
	defer d.http.CloseIdleConnections()
	if err = d.negotiate(ctx); err != nil {
		return err
	}
	id, tty, err := d.inspect(ctx, container)
	if err != nil {
		return err
	}
	saved := map[string]Position{}
	for _, stream := range []string{"stdout", "stderr"} {
		p := start[PositionKey(Position{Kind: "docker", Input: id, Stream: stream})]
		if p.Kind == "docker" && p.Input == id && p.Stream == stream {
			saved[stream] = p
		}
	}
	since := int64(0)
	for _, stream := range []string{"stdout", "stderr"} {
		p, ok := saved[stream]
		if !ok {
			since = 0
			break
		}
		t, e := time.Parse(time.RFC3339Nano, p.Timestamp)
		if e != nil {
			since = 0
			break
		}
		if since == 0 || t.Unix() < since {
			since = t.Unix()
		}
	}
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"}, "follow": {"1"}, "since": {strconv.FormatInt(since, 10)}}
	resp, err := d.get(ctx, d.version+"/containers/"+id+"/logs?"+q.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	emit := func(stream string, row ingest.Record, ts string, ordinal int) error {
		p := Position{Kind: "docker", Input: id, Stream: stream, Timestamp: ts, Ordinal: ordinal}
		select {
		case out <- Item{Record: row, Position: p}:
			if latest != nil && ts != "" {
				latest[PositionKey(p)] = p
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	readers := map[string]*dockerLines{}
	for _, stream := range []string{"stdout", "stderr"} {
		readers[stream] = &dockerLines{stream: stream, saved: saved[stream], emit: emit}
	}
	if tty {
		_, err = io.Copy(readers["stdout"], resp.Body)
	} else {
		for {
			var header [8]byte
			_, err = io.ReadFull(resp.Body, header[:])
			if err == io.EOF {
				err = nil
				break
			}
			if err != nil {
				break
			}
			stream := ""
			if header[0] == 1 {
				stream = "stdout"
			} else if header[0] == 2 {
				stream = "stderr"
			}
			if stream == "" || header[1] != 0 || header[2] != 0 || header[3] != 0 {
				err = errors.New("sender: malformed Docker log frame")
				break
			}
			length := binary.BigEndian.Uint32(header[4:])
			if length > maxDockerFrame {
				if _, err = io.CopyN(io.Discard, resp.Body, int64(length)); err != nil {
					break
				}
				err = emit(stream, ingest.Record{Line: "gap: Docker log frame exceeded 1 MiB; payload skipped"}, "", 0)
				if err != nil {
					break
				}
				continue
			}
			_, err = io.CopyN(readers[stream], resp.Body, int64(length))
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("sender: Docker logs: %w", err)
	}
	for _, stream := range []string{"stdout", "stderr"} {
		if err = readers[stream].flush(); err != nil {
			return err
		}
	}
	return nil
}

type dockerLines struct {
	stream       string
	saved        Position
	emit         func(string, ingest.Record, string, int) error
	buf          []byte
	truncated    bool
	last         string
	ordinal      int
	boundarySeen bool
	gapReported  bool
}

func (l *dockerLines) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		end := len(p)
		complete := i >= 0
		if complete {
			end = i
		}
		piece := p[:end]
		room := ingest.MaxLineBytes + 128 - len(l.buf)
		if len(piece) > room {
			piece = piece[:room]
			l.truncated = true
		}
		l.buf = append(l.buf, piece...)
		if complete {
			if err := l.flushLine(true); err != nil {
				return 0, err
			}
			p = p[i+1:]
		} else {
			p = nil
		}
	}
	return n, nil
}
func (l *dockerLines) flush() error {
	return l.flushLine(false)
}

func (l *dockerLines) flushLine(complete bool) error {
	if len(l.buf) == 0 && !l.truncated && !complete {
		return nil
	}
	raw := l.buf
	l.buf = nil
	truncated := l.truncated
	l.truncated = false
	split := bytes.IndexByte(raw, ' ')
	if split < 0 {
		return l.emit(l.stream, ingest.Record{Line: "gap: Docker log line lacks timestamp", Truncated: truncated}, "", 0)
	}
	t, err := time.Parse(time.RFC3339Nano, string(raw[:split]))
	if err != nil {
		return l.emit(l.stream, ingest.Record{Line: "gap: Docker log line has invalid timestamp", Truncated: truncated}, "", 0)
	}
	ts := t.UTC().Format(time.RFC3339Nano)
	if ts != l.last {
		l.last = ts
		l.ordinal = 0
	}
	l.ordinal++
	if l.saved.Timestamp != "" {
		old, err := time.Parse(time.RFC3339Nano, l.saved.Timestamp)
		if err == nil {
			if t.Before(old) {
				return nil
			}
			if t.Equal(old) {
				l.boundarySeen = true
			} else if !l.boundarySeen && !l.gapReported {
				l.gapReported = true
				if err := l.emit(l.stream, ingest.Record{Line: "gap: Docker checkpoint timestamp is absent from available history"}, l.saved.Timestamp, l.saved.Ordinal); err != nil {
					return err
				}
			}
		}
	}
	row := clip(Item{Record: ingest.Record{Line: string(bytes.ToValidUTF8(raw[split+1:], []byte("\ufffd"))), Time: t, Truncated: truncated}}).Record
	return l.emit(l.stream, row, ts, l.ordinal)
}
