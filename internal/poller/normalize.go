// Package poller checks every watched app's health URL on its interval and turns whatever
// comes back into one of three states. The rules for reading a response live in Normalize
// and nowhere else.
package poller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Busnes-app/ky-primitives/health"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// State is what a poll concluded. It is the alerts package's input.
type State string

const (
	OK       State = "ok"
	Degraded State = "degraded"
	Down     State = "down"
)

// Result is one poll, normalised. Basic marks an app that answered 2xx without a health
// document, so the screen can say "basic health, no checks". Cause names a down: a transport
// failure from egress.Cause, or status_<code> for a non-2xx answer.
type Result struct {
	State   State                `json:"state"`
	Basic   bool                 `json:"basic,omitempty"`
	Cause   string               `json:"cause,omitempty"`
	Service string               `json:"service,omitempty"`
	Checks  []health.CheckResult `json:"checks,omitempty"`
}

// Normalize applies the spec's four rules in order: a ky.health/1 document, a known status
// field, any other 2xx, and everything else is down.
func Normalize(resp *egress.Response, err error) Result {
	if err != nil {
		return Result{State: Down, Cause: egress.Cause(err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{State: Down, Cause: "status_" + strconv.Itoa(resp.StatusCode)}
	}
	var doc struct {
		Schema  string               `json:"schema"`
		Service string               `json:"service"`
		Status  json.RawMessage      `json:"status"`
		Healthy *bool                `json:"healthy"`
		Checks  []health.CheckResult `json:"checks"`
	}
	if json.Unmarshal(resp.Body, &doc) != nil {
		return Result{State: OK, Basic: true}
	}
	if doc.Schema == health.Schema {
		var s string
		_ = json.Unmarshal(doc.Status, &s)
		switch State(s) {
		case OK, Degraded, Down:
			return Result{State: State(s), Service: doc.Service, Checks: doc.Checks}
		}
		return Result{State: Down, Cause: "bad_health_document"}
	}
	var s string
	if json.Unmarshal(doc.Status, &s) == nil {
		switch strings.ToLower(s) {
		case "ok", "alive", "ready", "healthy":
			return Result{State: OK, Service: doc.Service}
		case "degraded":
			return Result{State: Degraded, Service: doc.Service}
		}
	}
	if doc.Healthy != nil {
		if *doc.Healthy {
			return Result{State: OK}
		}
		return Result{State: Degraded}
	}
	return Result{State: OK, Basic: true}
}
