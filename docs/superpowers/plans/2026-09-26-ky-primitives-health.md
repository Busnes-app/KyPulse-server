# ky-primitives `health` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `health` package to ky-primitives that serves the suite's `ky.health/1` JSON on `/healthz` without leaking error text, and document it in the README.

**Architecture:** Three files, one responsibility each:
- `health.go`: the contract types and reason codes.
- `evaluate.go`: runs the checks with deadlines, a hung-check guard and panic recovery.
- `handler.go`: the `http.Handler`, which caches for 5 s and logs failed checks through the existing `logging` package.

Only the standard library and this module's own `logging` package are used.

**Tech Stack:** Go 1.26 (`go.mod` pins 1.26.6; `sync.WaitGroup.Go` needs 1.25+), `net/http`, `encoding/json`, ky-primitives `logging`.

**Spec:** `/home/yoshi/git/busnes.app/kyPulse-server/docs/superpowers/specs/2026-09-26-kypulse-design.md`, section 1 "Health contract".

**Repository:** `/home/yoshi/git/busnes.app/ky-primitives` (module `github.com/Busnes-app/ky-primitives`). Work on branch `feat/health` off `master`. All paths below are relative to that repo.

**Provenance:** every code block in this plan was compiled, vetted and run with `go test -race -count=20` in a scratch clone of ky-primitives `master` at `40904f6`, together with the repo's `nodeps_test.go`. It passed.

## Global Constraints

- The response has exactly these keys: `schema` (always `"ky.health/1"`), `service`, `status`, `time` (UTC, whole seconds, RFC 3339), `checks` (array, `[]` when empty). Each check has `name`, `status`, and `reason` only when non-empty.
- `status` is one of `ok`, `degraded`, `down`. The service status is the worst check status. No checks means `ok`.
- HTTP code: 200 for `ok` and `degraded`, 503 for `down`, 405 with `Allow: GET, HEAD` for other methods. `HEAD` sends no body.
- Error text from a check never appears in the response. Reasons come only from `DeclareReason`, matching `^[a-z0-9_]{1,64}$`.
- Check names match `^[a-z][a-z0-9_]{0,63}$`. Service matches `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`.
- There are no version, build or uptime fields.
- The per-check default deadline is 2 s. The cache lasts 5 s.
- Only the standard library plus this module's own packages may be imported (`nodeps_test.go` enforces this).
- The package writes to stderr only, through `logging`, as the event `health_check_failed` with the field `health_check`.

## Review Focus

1. **A check that ignores its context** (a hung DB driver). Expect `down`/`timeout` within the deadline, and no pile-up of goroutines across polls. Pinned by `TestHungCheckIsNotStartedAgain` (Task 2).
2. **Anyone hammering the public route.** Expect the checks to run at most once per 5 s, however many requests arrive. Pinned by `TestOneEvaluationAnswersRequestsWithinCacheFor` and `TestConcurrentRequestsShareOneEvaluation` (Task 3).
3. **A check error holding a DSN or password.** Expect none of it in the body, and the operator still sees which check failed on stderr. Pinned by `TestErrorTextNeverReachesTheResponse` (Task 3).
4. **A client disconnecting mid-request.** Expect the shared cached result not to be poisoned with a cancellation. Pinned by `TestClientHangupDoesNotCancelChecks` (Task 3).
5. **A check that panics, then recovers on the next poll.** Expect `down` once, then `ok`, never a stuck "timeout". Pinned by `TestPanickingCheckIsDownAndReportsThePanic` (Task 2), which also covers the busy-flag ordering bug found while prototyping.

---

### Task 1: Contract types and reason codes

**Files:**
- Create: `health/health.go`
- Test: `health/health_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - Constants: `Schema = "ky.health/1"` and `DefaultTimeout = 2 * time.Second`.
  - Status type: `type Status string`, with `OK`, `Degraded` and `Down`.
  - Reason codes: `type Reason struct{code string}`, `func DeclareReason(code string) Reason`, and `var Timeout Reason` (`"timeout"`).
  - Check outcomes: `func Degrade(r Reason) error` and `func Fail(r Reason) error`.
  - Check and response types:
    - `type Check struct{Name string; Timeout time.Duration; Run func(ctx context.Context) error}`
    - `type Response struct{Schema, Service string; Status Status; Time time.Time; Checks []CheckResult}`
    - `type CheckResult struct{Name string; Status Status; Reason string}`
  - Unexported, used by Tasks 2 and 3:
    - `func (Status) rank() int`
    - `func classify(err error) (Status, string)`
    - `codePattern`, `namePattern` and `servicePattern`
  - Test fixtures used by later tasks' tests: `appendDisabled` and `dbUnreachable`, declared in `health_test.go`.

- [ ] **Step 1: Create the branch**

Run: `git -C /home/yoshi/git/busnes.app/ky-primitives switch -c feat/health`
Expected: `Switched to a new branch 'feat/health'`

- [ ] **Step 2: Write the failing test** at `health/health_test.go`

```go
package health

import (
	"errors"
	"fmt"
	"testing"
)

var (
	appendDisabled = DeclareReason("append_disabled")
	dbUnreachable  = DeclareReason("db_unreachable")
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus Status
		wantReason string
	}{
		{"nil is ok", nil, OK, ""},
		{"degrade", Degrade(appendDisabled), Degraded, "append_disabled"},
		{"fail", Fail(dbUnreachable), Down, "db_unreachable"},
		{"wrapped fail keeps its reason", fmt.Errorf("ping: %w", Fail(dbUnreachable)), Down, "db_unreachable"},
		{"plain error is down with no reason", errors.New("dial tcp 10.0.0.5:5432: connection refused"), Down, ""},
		{"zero reason degrades without a code", Degrade(Reason{}), Degraded, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := classify(tc.err)
			if status != tc.wantStatus || reason != tc.wantReason {
				t.Fatalf("classify = (%q, %q), want (%q, %q)", status, reason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}

func TestDeclareReasonRejectsCodesOutsideThePattern(t *testing.T) {
	for _, code := range []string{"", "Upper", "has space", "db@host", "a-b", string(make([]byte, 65))} {
		t.Run(fmt.Sprintf("%q", code), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("DeclareReason(%q) did not panic", code)
				}
			}()
			DeclareReason(code)
		})
	}
}

func TestDeclareReasonAcceptsTheLongestCode(t *testing.T) {
	code := ""
	for range 64 {
		code += "a"
	}
	if got := DeclareReason(code).code; got != code {
		t.Fatalf("code = %q", got)
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./health/`
Expected: FAIL. The build fails with `undefined: DeclareReason` (and similar).

- [ ] **Step 4: Write the implementation** at `health/health.go`

```go
// Package health serves the suite's ky.health/1 endpoint: one public JSON shape that says
// whether a service works, without saying anything an attacker could use.
package health

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"
)

// Schema is the value of Response.Schema.
const Schema = "ky.health/1"

// DefaultTimeout bounds a check whose Timeout is zero.
const DefaultTimeout = 2 * time.Second

// Status is the state of one check or of the whole service.
type Status string

const (
	OK       Status = "ok"
	Degraded Status = "degraded"
	Down     Status = "down"
)

func (s Status) rank() int {
	switch s {
	case OK:
		return 0
	case Degraded:
		return 1
	}
	return 2
}

var (
	codePattern    = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	namePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	servicePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
)

// Reason is a fixed code anyone who can reach /healthz may read. Its field is unexported,
// so the only reasons that exist are ones DeclareReason validated.
type Reason struct{ code string }

// DeclareReason admits a reason code. Call it once, at package level, so a bad code panics
// at startup rather than inside a running check.
func DeclareReason(code string) Reason {
	if !codePattern.MatchString(code) {
		panic("health: reason " + strconv.Quote(code) + " must match [a-z0-9_]{1,64}")
	}
	return Reason{code: code}
}

// Timeout is the reason for a check that missed its deadline.
var Timeout = DeclareReason("timeout")

type outcome struct {
	status Status
	reason Reason
}

func (o outcome) Error() string { return string(o.status) + ": " + o.reason.code }

// Degrade reports the check degraded, with r.
func Degrade(r Reason) error { return outcome{Degraded, r} }

// Fail reports the check down, with r.
func Fail(r Reason) error { return outcome{Down, r} }

// classify maps a check's return to what the response may show. An error that did not
// come from Degrade or Fail is down with no reason: its text never reaches the response.
func classify(err error) (Status, string) {
	if err == nil {
		return OK, ""
	}
	var o outcome
	if errors.As(err, &o) {
		return o.status, o.reason.code
	}
	return Down, ""
}

// Check is one dependency the service needs. Run must honour ctx.
type Check struct {
	Name    string        // [a-z][a-z0-9_]{0,63}, unique per handler
	Timeout time.Duration // zero means DefaultTimeout
	Run     func(ctx context.Context) error
}

// Response is the ky.health/1 body.
type Response struct {
	Schema  string        `json:"schema"`
	Service string        `json:"service"`
	Status  Status        `json:"status"`
	Time    time.Time     `json:"time"`
	Checks  []CheckResult `json:"checks"`
}

// CheckResult is one check in a Response.
type CheckResult struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race -count=1 ./health/ . && go vet ./health/`
Expected: `ok` for both packages. The root package runs `nodeps_test.go`, which must still pass.

- [ ] **Step 6: Commit**

```bash
gofmt -l health/   # must print nothing
git add health/health.go health/health_test.go
git commit -m "health: ky.health/1 contract types and reason codes"
```

---

### Task 2: Check evaluation

**Files:**
- Create: `health/evaluate.go`
- Test: `health/evaluate_test.go`

**Interfaces:**
- Consumes: `Check`, `CheckResult`, `Status` and its `rank()`, `classify`, `Timeout`, `Degrade`, and the `appendDisabled` fixture (all from Task 1).
- Produces, unexported and used by Task 3:
  - `type runner struct{check Check; busy atomic.Bool}`
  - `func (r *runner) run(ctx context.Context) (CheckResult, error)`
  - `func evaluate(ctx context.Context, runners []*runner) (Status, []CheckResult, []error)`
  - `var errPanicked`

  The returned `[]error` holds each check's raw error, for the log only.

- [ ] **Step 1: Write the failing test** at `health/evaluate_test.go`

```go
package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func runners(checks ...Check) []*runner {
	rs := make([]*runner, len(checks))
	for i, c := range checks {
		rs[i] = &runner{check: c}
	}
	return rs
}

func ok(context.Context) error { return nil }

func TestEvaluateStatusIsTheWorstCheck(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		want   Status
	}{
		{"no checks", nil, OK},
		{"all ok", []Check{{Name: "a", Run: ok}, {Name: "b", Run: ok}}, OK},
		{"one degraded", []Check{{Name: "a", Run: ok}, {Name: "b", Run: func(context.Context) error { return Degrade(appendDisabled) }}}, Degraded},
		{"down beats degraded", []Check{
			{Name: "a", Run: func(context.Context) error { return Degrade(appendDisabled) }},
			{Name: "b", Run: func(context.Context) error { return errors.New("boom") }},
		}, Down},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, results, _ := evaluate(context.Background(), runners(tc.checks...))
			if status != tc.want {
				t.Fatalf("status = %q, want %q", status, tc.want)
			}
			if len(results) != len(tc.checks) {
				t.Fatalf("got %d results for %d checks", len(results), len(tc.checks))
			}
		})
	}
}

func TestEvaluateKeepsDeclaredOrder(t *testing.T) {
	_, results, _ := evaluate(context.Background(), runners(
		Check{Name: "database", Run: func(context.Context) error { time.Sleep(20 * time.Millisecond); return nil }},
		Check{Name: "audit", Run: ok},
	))
	if results[0].Name != "database" || results[1].Name != "audit" {
		t.Fatalf("order = %q, %q", results[0].Name, results[1].Name)
	}
}

func TestCheckPastItsDeadlineIsDownWithTimeout(t *testing.T) {
	_, results, errs := evaluate(context.Background(), runners(Check{
		Name:    "slow",
		Timeout: 20 * time.Millisecond,
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}))
	if results[0].Status != Down || results[0].Reason != "timeout" {
		t.Fatalf("result = %+v", results[0])
	}
	if !errors.Is(errs[0], context.DeadlineExceeded) {
		t.Fatalf("err = %v", errs[0])
	}
}

func TestHungCheckIsNotStartedAgain(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var starts atomic.Int32
	rs := runners(Check{
		Name:    "hung",
		Timeout: 10 * time.Millisecond,
		Run: func(context.Context) error {
			starts.Add(1)
			<-release // ignores its context
			return nil
		},
	})
	for range 3 {
		_, results, _ := evaluate(context.Background(), rs)
		if results[0].Status != Down || results[0].Reason != "timeout" {
			t.Fatalf("result = %+v", results[0])
		}
	}
	if n := starts.Load(); n != 1 {
		t.Fatalf("check started %d times, want 1", n)
	}
}

func TestPanickingCheckIsDownAndReportsThePanic(t *testing.T) {
	var calls atomic.Int32
	rs := runners(Check{
		Name: "bad",
		Run: func(context.Context) error {
			if calls.Add(1) == 1 {
				panic("nil map")
			}
			return nil
		},
	})
	_, results, errs := evaluate(context.Background(), rs)
	if results[0].Status != Down || results[0].Reason != "" {
		t.Fatalf("result = %+v", results[0])
	}
	if !errors.Is(errs[0], errPanicked) {
		t.Fatalf("err = %v", errs[0])
	}
	// The same runner is free again: the next evaluation runs the check rather than
	// reporting it hung.
	_, results, _ = evaluate(context.Background(), rs)
	if results[0].Status != OK {
		t.Fatalf("second run = %+v", results[0])
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./health/`
Expected: FAIL. The build fails with `undefined: runner` and `undefined: evaluate`.

- [ ] **Step 3: Write the implementation** at `health/evaluate.go`

```go
package health

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var errPanicked = errors.New("health: check panicked")

type runner struct {
	check Check
	busy  atomic.Bool
}

// run executes the check once. The error is the check's raw return, for the log only.
func (r *runner) run(ctx context.Context) (CheckResult, error) {
	res := CheckResult{Name: r.check.Name}
	// A previous run that ignored its context is still going. Starting another would
	// stack one goroutine per poll on a hung dependency.
	if !r.busy.CompareAndSwap(false, true) {
		res.Status, res.Reason = Down, Timeout.code
		return res, context.DeadlineExceeded
	}
	timeout := r.check.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := r.safeRun(ctx)
		// Free the runner before reporting, so the next evaluation never sees a
		// finished check as still running.
		r.busy.Store(false)
		done <- err
	}()
	select {
	case err := <-done:
		res.Status, res.Reason = classify(err)
		return res, err
	case <-ctx.Done():
		res.Status, res.Reason = Down, Timeout.code
		return res, ctx.Err()
	}
}

func (r *runner) safeRun(ctx context.Context) (err error) {
	defer func() {
		if recover() != nil {
			err = errPanicked
		}
	}()
	return r.check.Run(ctx)
}

// evaluate runs every check concurrently. The service status is the worst check status.
func evaluate(ctx context.Context, runners []*runner) (Status, []CheckResult, []error) {
	results := make([]CheckResult, len(runners))
	errs := make([]error, len(runners))
	var wg sync.WaitGroup
	for i, r := range runners {
		wg.Go(func() { results[i], errs[i] = r.run(ctx) })
	}
	wg.Wait()
	status := OK
	for _, res := range results {
		if res.Status.rank() > status.rank() {
			status = res.Status
		}
	}
	return status, results, errs
}
```

Note the order inside the goroutine: `busy` is cleared **before** the result is sent. If it were cleared in a `defer` after the send, the next evaluation could see a finished check as still running and report a false `timeout`. `TestPanickingCheckIsDownAndReportsThePanic` exercises this.

- [ ] **Step 4: Run the tests to verify they pass, repeatedly, under race**

Run: `go test -race -count=20 ./health/ && go vet ./health/`
Expected: `ok`. Twenty runs catch the ordering race if it ever comes back.

- [ ] **Step 5: Commit**

```bash
gofmt -l health/   # must print nothing
git add health/evaluate.go health/evaluate_test.go
git commit -m "health: run checks with deadlines, hung-check guard and panic recovery"
```

---

### Task 3: HTTP handler

**Files:**
- Create: `health/handler.go`
- Test: `health/handler_test.go`

**Interfaces:**
- Consumes:
  - From Tasks 1 and 2: `evaluate`, `runner`, `Response`, `Schema`, the patterns, `Degrade`, and the `appendDisabled` fixture.
  - From `logging`: `logging.New`, `logging.Config`, `logging.DeclareEvent`, `logging.DeclareString`, `logging.ReasonCode` and `logging.Err`.
- Produces:
  - `func Handler(service string, lg *logging.Logger, checks ...Check) http.Handler`
  - `const CacheFor = 5 * time.Second`
  - The log event `health_check_failed`, with the field `health_check`.

- [ ] **Step 1: Write the failing test** at `health/handler_test.go`

```go
package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
)

func testLogger(t *testing.T) (*logging.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	lg, err := logging.New(logging.Config{App: "kytest", Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	return lg, &buf
}

var t0 = time.Date(2026, 9, 26, 10, 41, 0, 500_000_000, time.UTC)

func serve(h http.Handler, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/healthz", nil))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	return resp
}

func TestHandlerServesTheContract(t *testing.T) {
	lg, _ := testLogger(t)
	h := newHandler("kyvault", lg, func() time.Time { return t0 }, []Check{
		{Name: "database", Run: ok},
		{Name: "audit", Run: func(context.Context) error { return Degrade(appendDisabled) }},
	})
	rec := serve(h, http.MethodGet)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	for k, want := range map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	want := `{"schema":"ky.health/1","service":"kyvault","status":"degraded","time":"2026-09-26T10:41:00Z","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`
	if got := rec.Body.String(); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestNoChecksIsOKWithAnEmptyList(t *testing.T) {
	lg, _ := testLogger(t)
	rec := serve(newHandler("kynotes", lg, func() time.Time { return t0 }, nil), http.MethodGet)
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), `"checks":[]`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestDownIs503(t *testing.T) {
	lg, _ := testLogger(t)
	h := newHandler("kyvault", lg, func() time.Time { return t0 }, []Check{
		{Name: "database", Run: func(context.Context) error { return errors.New("refused") }},
	})
	rec := serve(h, http.MethodGet)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", rec.Code)
	}
	if decode(t, rec).Status != Down {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestErrorTextNeverReachesTheResponse(t *testing.T) {
	lg, logs := testLogger(t)
	secret := "postgres://pulse:hunter2@db.internal:5432/kyvault"
	h := newHandler("kyvault", lg, func() time.Time { return t0 }, []Check{
		{Name: "database", Run: func(context.Context) error { return errors.New("connect " + secret + ": refused") }},
	})
	rec := serve(h, http.MethodGet)
	for _, leak := range []string{"hunter2", "db.internal", "refused", "postgres"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("response leaks %q: %s", leak, rec.Body.String())
		}
	}
	// The operator still gets a line saying which check failed.
	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log is not one JSON line: %v\n%s", err, logs.String())
	}
	if line["event"] != "health_check_failed" || line["health_check"] != "database" {
		t.Fatalf("log line = %v", line)
	}
}

func TestHeadHasStatusAndNoBody(t *testing.T) {
	lg, _ := testLogger(t)
	h := newHandler("kyvault", lg, func() time.Time { return t0 }, []Check{
		{Name: "database", Run: func(context.Context) error { return errors.New("x") }},
	})
	rec := serve(h, http.MethodHead)
	if rec.Code != http.StatusServiceUnavailable || rec.Body.Len() != 0 {
		t.Fatalf("code = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestOtherMethodsAre405(t *testing.T) {
	lg, _ := testLogger(t)
	rec := serve(newHandler("kyvault", lg, time.Now, nil), http.MethodPost)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("code = %d, Allow = %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestOneEvaluationAnswersRequestsWithinCacheFor(t *testing.T) {
	lg, _ := testLogger(t)
	var runs atomic.Int32
	now := t0
	h := newHandler("kyvault", lg, func() time.Time { return now }, []Check{
		{Name: "database", Run: func(context.Context) error { runs.Add(1); return nil }},
	})
	serve(h, http.MethodGet)
	now = t0.Add(CacheFor - time.Millisecond)
	serve(h, http.MethodGet)
	if n := runs.Load(); n != 1 {
		t.Fatalf("ran %d times inside CacheFor, want 1", n)
	}
	now = t0.Add(CacheFor)
	serve(h, http.MethodGet)
	if n := runs.Load(); n != 2 {
		t.Fatalf("ran %d times after CacheFor, want 2", n)
	}
}

func TestConcurrentRequestsShareOneEvaluation(t *testing.T) {
	lg, _ := testLogger(t)
	var runs atomic.Int32
	h := Handler("kyvault", lg, Check{Name: "database", Run: func(context.Context) error {
		runs.Add(1)
		time.Sleep(20 * time.Millisecond)
		return nil
	}})
	done := make(chan struct{})
	for range 20 {
		go func() { serve(h, http.MethodGet); done <- struct{}{} }()
	}
	for range 20 {
		<-done
	}
	if n := runs.Load(); n != 1 {
		t.Fatalf("20 concurrent requests ran the check %d times, want 1", n)
	}
}

func TestClientHangupDoesNotCancelChecks(t *testing.T) {
	lg, _ := testLogger(t)
	h := newHandler("kyvault", lg, func() time.Time { return t0 }, []Check{
		{Name: "database", Run: func(ctx context.Context) error { time.Sleep(10 * time.Millisecond); return ctx.Err() }},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil).WithContext(ctx))
	if decode(t, rec).Status != OK {
		t.Fatalf("a cancelled request context reached the check: %s", rec.Body.String())
	}
}

func TestHandlerRejectsBadConstruction(t *testing.T) {
	lg, _ := testLogger(t)
	cases := map[string]func(){
		"bad service":     func() { Handler("ky vault", lg) },
		"empty service":   func() { Handler("", lg) },
		"nil logger":      func() { Handler("kyvault", nil) },
		"bad check name":  func() { Handler("kyvault", lg, Check{Name: "Database", Run: ok}) },
		"duplicate check": func() { Handler("kyvault", lg, Check{Name: "db", Run: ok}, Check{Name: "db", Run: ok}) },
		"nil run":         func() { Handler("kyvault", lg, Check{Name: "db"}) },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("did not panic")
				}
			}()
			f()
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./health/`
Expected: FAIL. The build fails with `undefined: newHandler`, `undefined: Handler` and `undefined: CacheFor`.

- [ ] **Step 3: Write the implementation** at `health/handler.go`

```go
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
)

// CacheFor is how long one evaluation answers every request. The route is public, so
// without it each request would run every check against the service's dependencies.
const CacheFor = 5 * time.Second

var (
	checkFailed = logging.DeclareEvent("health_check_failed", "health check failed", slog.LevelWarn)
	checkName   = logging.DeclareString("health_check")
)

type handler struct {
	service string
	lg      *logging.Logger
	runners []*runner
	now     func() time.Time

	mu   sync.Mutex
	at   time.Time
	resp Response
}

// Handler serves GET and HEAD for service's /healthz. Failed checks are logged to lg with
// the error kind; the response carries only status and reason codes. It panics on an
// invalid service or check name, a duplicate check name, a nil Run or a nil logger.
func Handler(service string, lg *logging.Logger, checks ...Check) http.Handler {
	return newHandler(service, lg, time.Now, checks)
}

func newHandler(service string, lg *logging.Logger, now func() time.Time, checks []Check) *handler {
	if !servicePattern.MatchString(service) {
		panic("health: service " + strconv.Quote(service) + " must match [A-Za-z0-9][A-Za-z0-9_.-]{0,63}")
	}
	if lg == nil {
		panic("health: a logger is required; failed checks are logged, not shown")
	}
	h := &handler{service: service, lg: lg, now: now}
	seen := map[string]bool{}
	for _, c := range checks {
		if !namePattern.MatchString(c.Name) {
			panic("health: check " + strconv.Quote(c.Name) + " must match [a-z][a-z0-9_]{0,63}")
		}
		if seen[c.Name] {
			panic("health: check " + c.Name + " is declared twice")
		}
		if c.Run == nil {
			panic("health: check " + c.Name + " has no Run")
		}
		seen[c.Name] = true
		h.runners = append(h.runners, &runner{check: c})
	}
	return h
}

// current returns the cached response, evaluating when it is older than CacheFor.
// Concurrent callers wait on the one evaluation instead of starting their own.
func (h *handler) current(ctx context.Context) Response {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if !h.at.IsZero() && now.Sub(h.at) < CacheFor {
		return h.resp
	}
	// Not the request context: the result is shared, so one client hanging up must not
	// cancel the checks every other client sees.
	status, results, errs := evaluate(context.Background(), h.runners)
	for i, res := range results {
		if res.Status != OK {
			h.lg.Log(ctx, checkFailed, checkName(res.Name), logging.ReasonCode(res.Reason), logging.Err(errs[i]))
		}
	}
	h.at = now
	h.resp = Response{
		Schema:  Schema,
		Service: h.service,
		Status:  status,
		Time:    now.UTC().Truncate(time.Second),
		Checks:  results,
	}
	return h.resp
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := h.current(r.Context())
	body, err := json.Marshal(resp)
	if err != nil {
		panic("health: marshal: " + err.Error()) // fixed types; cannot fail
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	code := http.StatusOK
	if resp.Status == Down {
		code = http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
```

- [ ] **Step 4: Run the full module check, which is what CI runs**

Run: `go build ./... && go vet ./... && go test -race -count=1 ./...`
Expected: every package `ok`, including the root package (`nodeps_test.go`) and `logging`. `logging` panics at init on a duplicate declaration, so `health_check_failed` and `health_check` must not already exist there.

Then: `go test -race -count=20 ./health/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
gofmt -l health/   # must print nothing
git add health/handler.go health/handler_test.go
git commit -m "health: /healthz handler with 5s shared cache and stderr logging"
```

---

### Task 4: README, pull request and release

**Files:**
- Modify: `README.md`. Insert a new `## health` section immediately before the `## password` heading, which follows `## logging`.

**Interfaces:**
- Consumes: the public API from Tasks 1 to 3.
- Produces: the published contract that kyPulse (plan step 2) and each app (plan step 5) read.

- [ ] **Step 1: Add the README section.** Insert this text verbatim before `## password`:

```markdown
## health

Serves `/healthz` in the suite's one shape, `ky.health/1`, which kyPulse reads. It is here
rather than fixed per product because the shape is a wire contract: before it, the suite's
health routes disagreed on path, field names, status codes and even content type, so a
monitor needed an adapter per product.

```go
var appendDisabled = health.DeclareReason("append_disabled")

mux.Handle("GET /healthz", health.Handler("kyvault", lg,
	health.Check{Name: "database", Run: db.PingContext},
	health.Check{Name: "audit", Run: func(ctx context.Context) error {
		if auditor.Disabled() {
			return health.Degrade(appendDisabled)
		}
		return nil
	}},
))
```

```json
{"schema":"ky.health/1","service":"kyvault","status":"degraded","time":"2026-09-26T10:41:00Z",
 "checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}
```

The route is public, so the response is built to be safe to show anyone.

- **No error text.** A check's error text never reaches the response: a DSN, a path or a
  hostname in `err.Error()` is the leak this prevents. An ordinary error is `down` with no
  reason. A check shows `degraded` or a reason only by returning `health.Degrade(r)` or
  `health.Fail(r)`, where `r` came from `DeclareReason`, which panics at startup on a code
  outside `[a-z0-9_]{1,64}`. The operator gets the rest on stderr: each non-ok check writes
  a `health_check_failed` line with the check name, reason and `logging.Err` error kind.
- **No version, build or uptime.** On a public route they fingerprint releases and reveal
  restart timing. A monitor gets the version from the container image.
- **Cached and shared.** One evaluation answers every request for `CacheFor` (5 s), and
  concurrent requests share it, so hammering the route cannot hammer the database. The
  evaluation uses its own context, so one client hanging up cannot cancel the result every
  other client sees.
- **Deadlines.** A check has `Timeout` (default 2 s) and must honour its context. A check
  past its deadline is `down` with reason `timeout`, and a check still running from the
  previous evaluation is not started again: it reports `timeout` until it returns. A
  panicking check is `down`.

The service status is the worst check status; no checks is `ok`. `ok` and `degraded`
return 200, `down` returns 503, so an orchestrator liveness probe that reads only the code
keeps working. `HEAD` returns the code with no body; other methods return 405.
`Response` and `CheckResult` are exported for monitors that decode the body.
```

- [ ] **Step 2: DOX pass.** `AGENTS.md` says package contracts live in README.md, so it needs no edit. Confirm that `AGENTS.md` makes no claim that the new package contradicts, for example a list of packages. If it does, update it in this commit.

- [ ] **Step 3: Run the full CI command set**

Run: `test -z "$(gofmt -l .)" && go build ./... && go vet ./... && go test -race -count=1 ./... && go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
Expected: no gofmt output, every package `ok`, and govulncheck reports no vulnerabilities.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "health: document the ky.health/1 contract"
```

- [ ] **Step 5: Push and open the PR. Ask Yoshi first**, because this is outward-facing.

```bash
git push -u origin feat/health
gh pr create --title "health: ky.health/1 /healthz handler" --body-file - <<'EOF'
Adds `health`, the suite's one `/healthz` shape (`ky.health/1`), which kyPulse reads.

- Error text never reaches the public response; reasons are declared codes only.
- One evaluation per 5 s, shared by concurrent requests; checks have deadlines, a hung check is not restarted, panics are down.
- Failed checks log `health_check_failed` to stderr via `logging`.
- Standard library plus this module's `logging` only; nodeps tests pass.

Spec: kyPulse-server `docs/superpowers/specs/2026-09-26-kypulse-design.md` §1.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
```

Register the PR with `link_pull_request` (the t3-code MCP tool) and drive CI to green with the `pull-request` skill.

- [ ] **Step 6: Release after merge. Ask Yoshi before tagging**, because a published tag is permanent in the Go module proxy.

```bash
git switch master && git pull --ff-only
git tag -a v0.9.0 -m "health: ky.health/1 /healthz handler"
git push origin v0.9.0
```

`v0.9.0` is the next minor after `v0.8.0`, since the change is purely additive.
