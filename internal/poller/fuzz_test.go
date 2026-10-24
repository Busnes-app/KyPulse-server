package poller

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/Busnes-app/ky-primitives/health"
)

// FuzzNormalize feeds arbitrary probe answers to Normalize. Whatever a watched host sends, the
// result is one of the three states, a non-2xx is always down with its status code as cause,
// and only a ky.health/1 document can make a 2xx answer down or carry checks.
func FuzzNormalize(f *testing.F) {
	for _, tc := range []struct {
		code int
		body string
	}{
		{200, `{"schema":"ky.health/1","service":"kyvault","status":"degraded","time":"2026-09-26T10:41:00Z","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`},
		{503, `{"schema":"ky.health/1","service":"kyvault","status":"down","checks":[{"name":"database","status":"down"}]}`},
		{200, `{"schema":"ky.health/1","service":"x","status":"weird"}`},
		{200, `{"schema":"ky.health/1","status":"down"}`},
		{200, `{"status":"alive"}`}, {200, `{"status":"ready"}`}, {200, `{"status":"HEALTHY"}`},
		{200, `{"status":"degraded","service":"kymark-server","time":"2026-09-26T10:00:00Z"}`},
		{200, `{"healthy":true,"unhealthyForSeconds":0}`}, {200, `{"healthy":false,"failureReason":["imap"]}`},
		{503, `{"healthy":false}`}, {200, "ok"}, {204, ""}, {200, `{"uptime":42}`}, {200, `{"status":{"db":"ok"}}`},
		{404, "not found"}, {301, ""}, {199, ""}, {300, ""}, {-1, ""}, {200, "null"}, {200, `[]`},
	} {
		f.Add(tc.code, []byte(tc.body))
	}
	f.Fuzz(func(t *testing.T, code int, body []byte) {
		got := Normalize(resp(code, string(body)), nil)
		if again := Normalize(resp(code, string(body)), nil); !reflect.DeepEqual(got, again) {
			t.Fatalf("not deterministic: %+v then %+v", got, again)
		}
		if _, err := json.Marshal(got); err != nil {
			t.Fatalf("result does not encode: %v", err)
		}
		switch got.State {
		case OK, Degraded, Down:
		default:
			t.Fatalf("state %q", got.State)
		}
		if code < 200 || code > 299 {
			want := Result{State: Down, Cause: "status_" + strconv.Itoa(code)}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("status %d gave %+v", code, got)
			}
			return
		}
		if got.Cause != "" && (got.Cause != "bad_health_document" || got.State != Down) {
			t.Fatalf("2xx cause %q with state %q", got.Cause, got.State)
		}
		if got.Basic && (got.State != OK || got.Service != "" || got.Checks != nil) {
			t.Fatalf("basic result carries detail: %+v", got)
		}
		var doc struct {
			Schema string `json:"schema"`
		}
		isHealthDoc := json.Unmarshal(body, &doc) == nil && doc.Schema == health.Schema
		if !isHealthDoc && (got.State == Down || got.Checks != nil) {
			t.Fatalf("non-health document gave %+v", got)
		}
	})
}
