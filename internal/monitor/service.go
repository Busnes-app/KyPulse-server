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
	evAlertCancelled = logging.DeclareEvent("alert_cancelled", "alert not sent: the process is stopping", slog.LevelWarn)
	fState           = logging.DeclareString("state")
	fPrevious        = logging.DeclareString("previous")
)

// sendBudget bounds one delivery, retries included, after the poll's own context is gone.
const sendBudget = 60 * time.Second

// recordBudget bounds the store writes that record a delivery's outcome.
const recordBudget = 10 * time.Second

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
	// ShutdownGrace is how long an in-flight delivery may go on once the process is stopping;
	// default 5 s.
	ShutdownGrace time.Duration

	once   sync.Once
	drain  sync.Once
	queue  chan delivery
	sender sync.WaitGroup

	mu       sync.Mutex
	overflow map[string]delivery // held while the queue was full; see enqueue
	closed   bool                // queue closed by Drain
}

// delivery is one queued message. done is a flush marker for tests: the sender closes it
// instead of sending.
type delivery struct {
	targetID   string
	eventID    int64
	msg        notify.Message
	done       chan struct{}
	persistent bool // offboarding alert survives cancellation for the next process
}

func (s *Service) init() {
	s.once.Do(func() {
		if s.QueueSize <= 0 {
			s.QueueSize = 64
		}
		if s.ShutdownGrace <= 0 {
			s.ShutdownGrace = 5 * time.Second
		}
		s.queue = make(chan delivery, s.QueueSize)
		s.overflow = map[string]delivery{}
	})
}

// Start launches the one sender goroutine. Observe only enqueues, so a slow receiver never
// holds a poll worker. Once ctx ends nothing more is sent: what is still queued is recorded
// as cancelled, so Drain returns promptly even with a dead receiver.
func (s *Service) Start(ctx context.Context) {
	s.init()
	s.sender.Add(1)
	go func() {
		defer s.sender.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case d, ok := <-s.queue:
				if !ok {
					s.mu.Lock()
					held := s.overflow
					s.overflow = nil
					s.mu.Unlock()
					for _, d := range held {
						if ctx.Err() != nil {
							s.cancelled(d)
						} else {
							s.send(ctx, d)
						}
					}
					return
				}
				switch {
				case d.done != nil:
					close(d.done)
				case ctx.Err() != nil:
					s.cancelled(d)
				default:
					s.send(ctx, d)
				}
				s.refill()
			case <-ticker.C:
				if ctx.Err() == nil {
					s.sendOffboarding(ctx)
				}
				s.refill()
			}
		}
	}()
}

// sendOffboarding consumes one durable failure on the existing serial sender. A crash
// before recording the outcome may repeat the webhook; it cannot create another alert.
func (s *Service) sendOffboarding(ctx context.Context) {
	ev, err := s.Store.Targets().NextOffboardingAlert(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		s.Logger.Log(ctx, evPollStoreError, logging.Err(err))
		return
	}
	tg, err := s.Store.Targets().GetTarget(ctx, ev.TargetID)
	if err != nil || ctx.Err() != nil {
		return
	}
	reason := ""
	if !tg.Enabled {
		reason = "paused"
	} else if tg.UntilFixed || (tg.SilencedUntil != nil && s.now().Before(*tg.SilencedUntil)) {
		reason = "silenced"
	}
	if reason != "" {
		_ = s.Store.Targets().SetEventNotified(ctx, ev.ID, false, reason)
		return
	}
	s.send(ctx, delivery{targetID: tg.ID, eventID: ev.ID, persistent: true, msg: notify.Message{App: tg.Name, State: ev.ToState, Previous: ev.FromState, Reason: ev.Cause, Time: ev.At, URL: s.AppURL + "/#/apps/" + tg.ID}})
}

// Drain closes the queue and waits for the sender to finish what is left. Call it after the
// poller has stopped: an Observe after Drain panics. A second call just waits.
func (s *Service) Drain() {
	s.init()
	s.drain.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.queue)
		s.mu.Unlock()
	})
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
		// too, unless an admin set a new silence since this poll read the row.
		if err := s.Store.Targets().ClearUntilFixed(ctx, tg.ID); err != nil {
			s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
		}
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
	// Each transition and each flapping notice is its own event; the last one recorded is what
	// a send reports. Without either, a send is a reminder about the current state.
	var ev *store.TargetEvent
	record := func(from, to alerts.State, cause string) bool {
		ev = &store.TargetEvent{TargetID: tg.ID, At: o.At, FromState: string(from), ToState: string(to), Cause: cause, Reminder: decision.Reminder}
		if err := s.Store.Targets().RecordEvent(ctx, ev); err != nil {
			s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
			return false
		}
		return true
	}
	if tr != nil {
		s.Logger.Log(ctx, evStateChanged, logging.TargetID(tg.ID), fState(string(tr.To)), fPrevious(string(tr.From)), logging.ReasonCode(tr.Cause))
		if !record(tr.From, tr.To, tr.Cause) {
			return
		}
	}
	if n := decision.Notice; n != nil {
		if !record(n.From, n.To, n.Cause) {
			return
		}
	} else if tr == nil && decision.Send && !record(track.State, track.State, track.Cause) {
		return
	}
	if !decision.Send {
		return
	}
	msg := notify.Message{App: tg.Name, State: ev.ToState, Previous: ev.FromState, Reason: ev.Cause,
		Time: o.At, URL: s.AppURL + "/#/apps/" + tg.ID, Reminder: decision.Reminder}
	s.enqueue(ctx, delivery{targetID: tg.ID, eventID: ev.ID, msg: msg})
}

// enqueue never blocks a poll worker. On a full queue a reminder is dropped, but any other
// message is held, newest per target, and queued by the sender as room frees up: a recovery
// is never lost to a queue full of reminders. While a target has a held message its later
// ones wait behind it, so each target's messages stay in order.
func (s *Service) enqueue(ctx context.Context, d delivery) {
	s.init()
	var dropped *delivery
	s.mu.Lock()
	held, isHeld := s.overflow[d.targetID]
	switch {
	case isHeld && d.msg.Reminder:
		dropped = &d
	case isHeld:
		s.overflow[d.targetID], dropped = d, &held // the newer message supersedes it
	default:
		select {
		case s.queue <- d:
		default:
			if d.msg.Reminder {
				dropped = &d
			} else {
				s.overflow[d.targetID] = d
			}
		}
	}
	s.mu.Unlock()
	if dropped != nil {
		s.Logger.Log(ctx, evAlertDropped, logging.TargetID(dropped.targetID), logging.Count(1))
		_ = s.Store.Targets().SetEventNotified(ctx, dropped.eventID, false, "queue_full")
	}
}

// refill moves held messages into the queue while it has room.
func (s *Service) refill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.overflow {
		if s.closed {
			return
		}
		select {
		case s.queue <- d:
			delete(s.overflow, id)
		default:
			return
		}
	}
}

// send delivers one message on a context detached from the loop, records the outcome on the
// event and in the delivery status, and audits it. The result is data, not an error. When
// the loop's ctx ends mid-send the delivery gets ShutdownGrace more, then is cut off.
func (s *Service) send(ctx context.Context, d delivery) {
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendBudget)
	defer cancel()
	stop := context.AfterFunc(ctx, func() {
		grace := time.NewTimer(s.ShutdownGrace)
		defer grace.Stop()
		select {
		case <-grace.C:
			cancel()
		case <-sendCtx.Done():
		}
	})
	defer stop()

	cfg, ok, err := s.Webhooks.Load(sendCtx)
	if !ok && err == nil {
		if d.persistent {
			_ = s.Store.Targets().SetEventNotified(sendCtx, d.eventID, false, "unconfigured")
		}
		return // no webhook configured: nothing to send, nothing to record
	}
	// An unreadable webhook (e.g. a rotated deployment key) is a failed delivery, recorded
	// like any other. Only notify.Reason is stored: err.Error() can name the webhook URL.
	if err == nil {
		err = s.Notifier.Send(sendCtx, cfg, d.msg)
	}
	s.record(d, err)
}

// cancelled records a delivery that was never attempted because the process is stopping.
func (s *Service) cancelled(d delivery) {
	s.record(d, context.Canceled)
}

// record writes a delivery's outcome on a context of its own, so a cut-off send is still
// recorded. Only notify.Reason is stored: err.Error() can name the webhook URL.
func (s *Service) record(d delivery, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), recordBudget)
	defer cancel()
	targetID, msg := d.targetID, d.msg
	status := DeliveryStatus{At: s.now(), OK: err == nil, Error: notify.Reason(err)}
	action, details := "alert.sent", "target="+targetID+" state="+msg.State
	switch {
	case err == nil:
		s.Logger.Log(ctx, evAlertSent, logging.TargetID(targetID), fState(msg.State))
	case status.Error == "cancelled":
		action, details = "alert.send_failed", details+" reason="+status.Error
		s.Logger.Log(ctx, evAlertCancelled, logging.TargetID(targetID), fState(msg.State))
	default:
		action, details = "alert.send_failed", details+" reason="+status.Error
		s.Logger.Log(ctx, evAlertFailed, logging.TargetID(targetID), fState(msg.State), logging.Err(err))
	}
	_ = s.Webhooks.SetStatus(ctx, status)
	notifyError := status.Error
	if d.persistent && status.Error == "cancelled" {
		notifyError = "pending"
	}
	_ = s.Store.Targets().SetEventNotified(ctx, d.eventID, err == nil, notifyError)
	_ = s.Store.Audit().LogAudit(ctx, &store.AuditRecord{UserID: "system", Action: action, Resource: targetID, Details: details})
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
