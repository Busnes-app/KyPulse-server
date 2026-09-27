package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/alerts"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

var (
	evStateChanged   = logging.DeclareEvent("target_state_changed", "watched app changed state", slog.LevelWarn)
	evAlertSent      = logging.DeclareEvent("alert_sent", "alert delivered to the webhook", slog.LevelInfo)
	evAlertFailed    = logging.DeclareEvent("alert_send_failed", "alert could not be delivered", slog.LevelWarn)
	evPollStoreError = logging.DeclareEvent("poll_store_error", "poll result could not be stored", slog.LevelError)
	evAlertDropped   = logging.DeclareEvent("alert_dropped", "alert dropped: the delivery queue is full", slog.LevelWarn)
	evPollDueFailed  = logging.DeclareEvent("poll_due_failed", "could not list the targets due for a poll", slog.LevelError)
	fState           = logging.DeclareString("state")
	fPrevious        = logging.DeclareString("previous")
)

// sendBudget bounds one delivery, retries included, after the poll's own context is gone.
const sendBudget = 60 * time.Second

// ErrNoWebhook is SendTest's answer when nothing is configured; the API maps it to 412.
var ErrNoWebhook = errors.New("monitor: no webhook is configured")

// Service is the monitoring loop's brain. One per process.
type Service struct {
	Store     store.Store
	Webhooks  *Webhooks
	Notifier  *notify.Notifier
	Logger    *logging.Logger
	AppURL    string
	Now       func() time.Time
	QueueSize int // deliveries waiting for the sender; default 64

	once   sync.Once
	queue  chan delivery
	sender sync.WaitGroup
}

// delivery is one queued message. done is a flush marker for tests: the sender closes it
// instead of sending.
type delivery struct {
	targetID string
	eventID  int64
	msg      notify.Message
	done     chan struct{}
}

func (s *Service) init() {
	s.once.Do(func() {
		if s.QueueSize <= 0 {
			s.QueueSize = 64
		}
		s.queue = make(chan delivery, s.QueueSize)
	})
}

// Start launches the one sender goroutine. Observe only enqueues, so a slow receiver never
// holds a poll worker.
func (s *Service) Start(ctx context.Context) {
	s.init()
	s.sender.Add(1)
	go func() {
		defer s.sender.Done()
		for d := range s.queue {
			if d.done != nil {
				close(d.done)
				continue
			}
			s.send(ctx, d)
		}
	}()
}

// Drain closes the queue and waits for the sender to deliver what is left. Call it once, after
// the poller has stopped: an Observe after Drain panics.
func (s *Service) Drain() {
	s.init()
	close(s.queue)
	s.sender.Wait()
}

// DueFailed is the poller's OnError.
func (s *Service) DueFailed(err error) {
	s.Logger.Log(context.Background(), evPollDueFailed, logging.Err(err))
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Track decodes a target's stored track and overlays the silence columns, which are the
// source of truth for silencing: they are never carried in the track JSON.
func Track(t *store.Target) alerts.Track {
	var tr alerts.Track
	if t.TrackJSON != "" {
		_ = json.Unmarshal([]byte(t.TrackJSON), &tr)
	}
	if tr.State == "" {
		tr.State, tr.Since = alerts.Pending, t.CreatedAt
	}
	tr.SilencedUntil = time.Time{}
	if t.SilencedUntil != nil {
		tr.SilencedUntil = *t.SilencedUntil
	}
	tr.UntilFixed = t.UntilFixed
	return tr
}

// Due lists enabled targets whose interval has elapsed since their last poll. Computed here,
// not in SQL, so both dialects share one rule and the list is small anyway.
func (s *Service) Due(ctx context.Context, now time.Time) ([]poller.Target, error) {
	all, err := s.Store.Targets().ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	var due []poller.Target
	for _, t := range all {
		if !t.Enabled {
			continue
		}
		interval := time.Duration(t.IntervalSec) * time.Second
		if t.LastPolledAt != nil && now.Before(t.LastPolledAt.Add(interval)) {
			continue
		}
		due = append(due, poller.Target{ID: t.ID, Name: t.Name, URL: t.URL})
	}
	return due, nil
}

// Observe applies one poll: state machine, persistence, event, and a queued webhook. The store
// is written before anything is queued, so a failed send never loses the state change.
func (s *Service) Observe(ctx context.Context, o poller.Observation) {
	tg, err := s.Store.Targets().GetTarget(ctx, o.Target.ID)
	if err != nil {
		return // deleted while in flight
	}
	before := Track(tg)
	track, tr := alerts.Next(before, o.Result, o.At)
	decision, track := alerts.Decide(track, tr, o.At)

	if before.UntilFixed && !track.UntilFixed {
		// Recovery cleared the silence: the columns are the source of truth, so clear them
		// too, not just the in-memory track.
		_ = s.Store.Targets().SetSilence(ctx, tg.ID, nil, false)
	}
	// The silence columns own this state; never let a stale copy ride along in the JSON.
	track.SilencedUntil, track.UntilFixed = time.Time{}, false

	trackJSON, _ := json.Marshal(track)
	resultJSON, _ := json.Marshal(o.Result)
	err = s.Store.Targets().RecordPoll(ctx, tg.ID, store.PollUpdate{
		PolledAt: o.At, LatencyMS: o.Latency.Milliseconds(), LastResult: string(resultJSON),
		TrackJSON: string(trackJSON), State: string(track.State), StateSince: track.Since, Cause: track.Cause,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return // deleted between GetTarget and RecordPoll
		}
		s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
		return
	}
	if tr == nil && !decision.Send {
		return
	}
	ev := &store.TargetEvent{TargetID: tg.ID, At: o.At, FromState: string(track.State), ToState: string(track.State), Cause: track.Cause, Reminder: decision.Reminder}
	if tr != nil {
		ev.FromState, ev.ToState, ev.Cause = string(tr.From), string(tr.To), tr.Cause
		s.Logger.Log(ctx, evStateChanged, logging.TargetID(tg.ID), fState(string(tr.To)), fPrevious(string(tr.From)), logging.ReasonCode(tr.Cause))
	}
	if err := s.Store.Targets().RecordEvent(ctx, ev); err != nil {
		s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
		return
	}
	if !decision.Send {
		return
	}
	msg := notify.Message{App: tg.Name, State: ev.ToState, Previous: ev.FromState, Reason: ev.Cause,
		Time: o.At, URL: s.AppURL + "/#/apps/" + tg.ID, Reminder: decision.Reminder}
	s.init()
	select {
	case s.queue <- delivery{targetID: tg.ID, eventID: ev.ID, msg: msg}:
	default:
		// Drop the newest: the queued ones are older news the operator is already owed.
		s.Logger.Log(ctx, evAlertDropped, logging.TargetID(tg.ID), logging.Count(1))
		_ = s.Store.Targets().SetEventNotified(ctx, ev.ID, false, "queue_full")
	}
}

// send delivers one message on a context detached from the loop, records the outcome on the
// event and in the delivery status, and audits it. The result is data, not an error.
func (s *Service) send(ctx context.Context, d delivery) {
	targetID, eventID, msg := d.targetID, d.eventID, d.msg
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendBudget)
	defer cancel()

	cfg, ok, err := s.Webhooks.Load(sendCtx)
	if !ok && err == nil {
		return // no webhook configured: nothing to send, nothing to record
	}
	// An unreadable webhook (e.g. a rotated deployment key) is a failed delivery, recorded
	// like any other. Only notify.Reason is stored: err.Error() can name the webhook URL.
	if err == nil {
		err = s.Notifier.Send(sendCtx, cfg, msg)
	}
	status := DeliveryStatus{At: s.now(), OK: err == nil, Error: notify.Reason(err)}
	action, details := "alert.sent", "target="+targetID+" state="+msg.State
	if err != nil {
		action, details = "alert.send_failed", details+" reason="+status.Error
		s.Logger.Log(sendCtx, evAlertFailed, logging.TargetID(targetID), fState(msg.State), logging.Err(err))
	} else {
		s.Logger.Log(sendCtx, evAlertSent, logging.TargetID(targetID), fState(msg.State))
	}
	_ = s.Webhooks.SetStatus(sendCtx, status)
	_ = s.Store.Targets().SetEventNotified(sendCtx, eventID, err == nil, status.Error)
	_ = s.Store.Audit().LogAudit(sendCtx, &store.AuditRecord{UserID: "system", Action: action, Resource: targetID, Details: details})
}

// Silence writes only the target's silence columns, the source of truth for silencing, so a
// poll landing in between never overwrites or loses it.
func (s *Service) Silence(ctx context.Context, id string, d time.Duration, untilFixed bool) error {
	var until *time.Time
	if d > 0 {
		u := s.now().Add(d)
		until = &u
	}
	return s.Store.Targets().SetSilence(ctx, id, until, untilFixed)
}

// SendTest delivers a test message and records the delivery status. The error is returned
// as well, so the admin's request can say what went wrong.
func (s *Service) SendTest(ctx context.Context) error {
	cfg, ok, err := s.Webhooks.Load(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoWebhook
	}
	msg := notify.Message{App: "kyPulse", State: "ok", Previous: "ok", Time: s.now(), URL: s.AppURL, Test: true}
	// A test is a diagnostic the admin is waiting on: one attempt, answered within the request.
	once := *s.Notifier
	once.Backoff = nil
	sendErr := once.Send(ctx, cfg, msg)
	_ = s.Webhooks.SetStatus(ctx, DeliveryStatus{At: s.now(), OK: sendErr == nil, Error: notify.Reason(sendErr)})
	return sendErr
}
