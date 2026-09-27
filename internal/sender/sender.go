package sender

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

const queueLimit = 16 << 20

var (
	deliveryRetried = logging.DeclareEvent("sender_delivery_retry", "sender retrying a log batch", slog.LevelWarn)
	deliveryStopped = logging.DeclareEvent("sender_delivery_stopped", "sender delivery stopped", slog.LevelError)
	inputDropped    = logging.DeclareEvent("sender_input_dropped", "sender dropped buffered input", slog.LevelWarn)
	statusField     = logging.DeclareString("sender_http_status")
)

type Item struct {
	Record   ingest.Record
	Position Position
}

// HTTP is the guarded POST surface; egress.Client is the production implementation.
type HTTP interface {
	Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error)
}

type Sender struct {
	HTTP      HTTP
	StateDir  string
	Token     []byte
	State     State
	Logger    *logging.Logger
	newTimer  func(time.Duration) <-chan time.Time
	wait      func(context.Context, time.Duration) error
	now       func() time.Time
	saveState func(string, State) error
}

type queued struct {
	item Item
	size int
}
type batch struct {
	body        []byte
	items       []queued
	markerCount uint64
	dropped     map[string]Position
	bytes       int
}
type result struct{ err error }

func itemSize(item Item) int {
	return len(item.Record.Line) + len(item.Position.Input) + len(item.Position.Timestamp) + 128
}

func clip(item Item) Item {
	if len(item.Record.Line) > ingest.MaxLineBytes {
		item.Record.Line = item.Record.Line[:ingest.MaxLineBytes]
		for !utf8.ValidString(item.Record.Line) {
			item.Record.Line = item.Record.Line[:len(item.Record.Line)-1]
		}
		item.Record.Truncated = true
	}
	return item
}

func encode(record ingest.Record) []byte { b, _ := json.Marshal(record); return append(b, '\n') }

func makeBatch(queue []queued, count uint64, positions map[string]Position) batch {
	b := batch{dropped: map[string]Position{}}
	if count > 0 {
		b.markerCount = count
		for k, v := range positions {
			b.dropped[k] = v
		}
		b.body = encode(ingest.Record{Line: fmt.Sprintf("dropped %d lines", count)})
	}
	for _, entry := range queue {
		row := encode(entry.item.Record)
		if len(b.items) >= 500 || len(b.body)+len(row) > ingest.MaxRequestBytes {
			break
		}
		b.body = append(b.body, row...)
		b.items = append(b.items, entry)
		b.bytes += entry.size
	}
	return b
}

func (s *Sender) Run(ctx context.Context, input <-chan Item) error {
	lock, err := Lock(s.StateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if s.HTTP == nil || len(s.Token) != 32 || s.State.URL == "" || s.State.SourceID == "" || s.StateDir == "" {
		return errors.New("sender: incomplete delivery configuration")
	}
	if err := egress.ValidateURL(s.State.URL, false); err != nil {
		return err
	}
	stored, token, err := LoadState(s.StateDir)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(token, s.Token) != 1 || !reflect.DeepEqual(stored, s.State) {
		return errors.New("sender: local state does not match paired credentials")
	}
	save := s.saveState
	if save == nil {
		save = SaveState
	}
	timer := s.newTimer
	if timer == nil {
		timer = func(d time.Duration) <-chan time.Time { return time.After(d) }
	}
	queue := make([]queued, 0, 500)
	dropped := map[string]Position{}
	inputs := make(map[string]bool, len(s.State.Positions))
	for k := range s.State.Positions {
		inputs[k] = true
	}
	var droppedCount uint64
	var used int
	var active *batch
	var done chan result
	var tick <-chan time.Time
	var startErr error
	closed := false
	start := func() {
		if active != nil || len(queue) == 0 && droppedCount == 0 {
			return
		}
		b := makeBatch(queue, droppedCount, dropped)
		active = &b
		queue = append([]queued(nil), queue[len(b.items):]...)
		used -= b.bytes
		for used+b.bytes+len(b.body) > queueLimit && len(queue) > 0 {
			old := queue[0]
			queue[0] = queued{}
			queue = queue[1:]
			used -= old.size
			if err := recordDrop(dropped, &droppedCount, old.item.Position); err != nil {
				startErr = err
				return
			}
		}
		done = make(chan result, 1)
		go func() { done <- result{s.deliver(ctx, b.body)} }()
		tick = nil
	}
	for {
		if startErr != nil {
			return startErr
		}
		if active == nil && (len(queue) >= 500 || closed || (len(queue) > 0 || droppedCount > 0) && tick == nil) {
			if closed || len(queue) >= 500 {
				start()
			} else {
				tick = timer(2 * time.Second)
			}
		}
		if closed && active == nil && len(queue) == 0 && droppedCount == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item, ok := <-input:
			if !ok {
				input = nil
				closed = true
				start()
				continue
			}
			item = clip(item)
			if item.Position.Kind != "file" && item.Position.Kind != "docker" && item.Position.Kind != "stdin" || item.Position.Input == "" || len(item.Position.Input) > 512 || len(item.Position.Timestamp) > 64 {
				return errors.New("sender: invalid input position")
			}
			key := PositionKey(item.Position)
			if !inputs[key] && len(inputs) >= 256 {
				return errors.New("sender: too many input streams")
			}
			inputs[key] = true
			entry := queued{item: item, size: itemSize(item)}
			for used+entry.size+func() int {
				if active != nil {
					return active.bytes + len(active.body)
				}
				return 0
			}() > queueLimit && len(queue) > 0 {
				old := queue[0]
				queue[0] = queued{}
				queue = queue[1:]
				used -= old.size
				if err := recordDrop(dropped, &droppedCount, old.item.Position); err != nil {
					return err
				}
			}
			if used+entry.size+func() int {
				if active != nil {
					return active.bytes + len(active.body)
				}
				return 0
			}() > queueLimit {
				if err := recordDrop(dropped, &droppedCount, item.Position); err != nil {
					return err
				}
			} else {
				queue = append(queue, entry)
				used += entry.size
			}
			if len(queue) >= 500 {
				start()
			}
		case <-tick:
			tick = nil
			start()
		case r := <-done:
			if r.err != nil {
				if s.Logger != nil {
					s.Logger.Log(ctx, deliveryStopped, logging.Err(r.err))
				}
				return r.err
			}
			next := s.State
			next.Positions = make(map[string]Position, len(s.State.Positions)+len(active.items)+len(active.dropped))
			for k, v := range s.State.Positions {
				next.Positions[k] = v
			}
			for k, v := range active.dropped {
				next.Positions[k] = v
			}
			for _, entry := range active.items {
				next.Positions[PositionKey(entry.item.Position)] = entry.item.Position
			}
			if err := save(s.StateDir, next); err != nil {
				return fmt.Errorf("sender: checkpoint: %w", err)
			}
			s.State = next
			droppedCount -= active.markerCount
			for k, v := range active.dropped {
				if dropped[k] == v {
					delete(dropped, k)
				}
			}
			active = nil
			done = nil
			if droppedCount > 0 && s.Logger != nil {
				s.Logger.Log(ctx, inputDropped, logging.Count(int64(droppedCount)))
			}
			if closed {
				start()
			} else if len(queue) > 0 {
				tick = timer(2 * time.Second)
			}
		}
	}
}

func recordDrop(positions map[string]Position, count *uint64, p Position) error {
	key := PositionKey(p)
	if _, ok := positions[key]; !ok && len(positions) >= 256 {
		return errors.New("sender: too many dropped input streams")
	}
	positions[key] = p
	*count++
	return nil
}

func (s *Sender) deliver(ctx context.Context, body []byte) error {
	backoff := time.Second
	for {
		resp, err := s.HTTP.Post(ctx, strings.TrimRight(s.State.URL, "/")+"/api/ingest/logs", "application/x-ndjson", body, map[string]string{"Authorization": "Bearer " + hex.EncodeToString(s.Token)})
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil && (resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 413 || resp.StatusCode == 415) {
			return fmt.Errorf("sender: delivery refused (HTTP %d); fix sender configuration or protocol", resp.StatusCode)
		}
		delay := backoff + time.Duration(rand.Int64N(int64(backoff/2)+1))
		if err == nil && resp.StatusCode == 429 {
			now := time.Now
			if s.now != nil {
				now = s.now
			}
			if d := retryAfter(resp.Header.Get("Retry-After"), now()); d > 0 {
				delay = d
			}
		}
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
		if s.Logger != nil {
			status := "network"
			if err == nil {
				status = strconv.Itoa(resp.StatusCode)
			}
			s.Logger.Log(ctx, deliveryRetried, statusField(status))
		}
		wait := s.wait
		if wait == nil {
			wait = func(ctx context.Context, d time.Duration) error {
				t := time.NewTimer(d)
				defer t.Stop()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-t.C:
					return nil
				}
			}
		}
		if err := wait(ctx, delay); err != nil {
			return err
		}
		if backoff < 5*time.Minute {
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		}
	}
}

func retryAfter(value string, now time.Time) time.Duration {
	var d time.Duration
	if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 0 {
		if n > 300 {
			n = 300
		}
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	}
	if d < 0 {
		return 0
	}
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}
