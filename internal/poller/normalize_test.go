package poller

import (
	"errors"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

func resp(code int, body string) *egress.Response {
	return &egress.Response{StatusCode: code, Body: []byte(body)}
}

// The fixtures are the real answers of the suite's apps on 2026-09-26.
func TestNormalizeAgainstTodaysApps(t *testing.T) {
	cases := []struct {
		name string
		resp *egress.Response
		err  error
		want Result
	}{
		{"ky.health/1 degraded", resp(200, `{"schema":"ky.health/1","service":"kyvault","status":"degraded","time":"2026-09-26T10:41:00Z","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`), nil,
			Result{State: Degraded, Service: "kyvault"}},
		{"ky.health/1 down 503", resp(503, `{"schema":"ky.health/1","service":"kyvault","status":"down","checks":[{"name":"database","status":"down"}]}`), nil,
			Result{State: Down, Cause: "status_503"}},
		{"ky.health/1 unknown status", resp(200, `{"schema":"ky.health/1","service":"x","status":"weird"}`), nil,
			Result{State: Down, Cause: "bad_health_document"}},
		{"KyIdentity healthz alive", resp(200, `{"status":"alive"}`), nil, Result{State: OK}},
		{"kynotes readyz", resp(200, `{"status":"ready"}`), nil, Result{State: OK}},
		{"kybookmarks always-200 degraded", resp(200, `{"status":"degraded","service":"kymark-server","time":"2026-09-26T10:00:00Z"}`), nil,
			Result{State: Degraded, Service: "kymark-server"}},
		{"KyVault ok", resp(200, `{"status":"ok","service":"kyvault-server","time":"x"}`), nil, Result{State: OK, Service: "kyvault-server"}},
		{"kypost healthy true", resp(200, `{"healthy":true,"unhealthyForSeconds":0}`), nil, Result{State: OK}},
		{"kypost healthy false", resp(200, `{"healthy":false,"failureReason":["imap"]}`), nil, Result{State: Degraded}},
		{"kypost 503 when unhealthy", resp(503, `{"healthy":false}`), nil, Result{State: Down, Cause: "status_503"}},
		{"kydns plain ok", resp(200, "ok"), nil, Result{State: OK, Basic: true}},
		{"KyYard live", resp(200, `{"status":"ok"}`), nil, Result{State: OK}},
		{"empty 204", resp(204, ""), nil, Result{State: OK, Basic: true}},
		{"JSON without a status", resp(200, `{"uptime":42}`), nil, Result{State: OK, Basic: true}},
		{"status is not a string", resp(200, `{"status":{"db":"ok"}}`), nil, Result{State: OK, Basic: true}},
		{"404", resp(404, "not found"), nil, Result{State: Down, Cause: "status_404"}},
		{"redirect status 301", resp(301, ""), nil, Result{State: Down, Cause: "status_301"}},
		{"transport error", nil, egress.ErrRefusedAddress, Result{State: Down, Cause: "address_refused"}},
		{"plain error", nil, errors.New("dial tcp: connection refused"), Result{State: Down, Cause: "refused"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.resp, tc.err)
			got.Checks = nil // compared separately below
			if got.State != tc.want.State || got.Basic != tc.want.Basic || got.Cause != tc.want.Cause || got.Service != tc.want.Service {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNormalizeKeepsTheChecksOfAHealthDocument(t *testing.T) {
	got := Normalize(resp(200, `{"schema":"ky.health/1","service":"kyvault","status":"ok","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`), nil)
	if len(got.Checks) != 2 || got.Checks[1].Reason != "append_disabled" {
		t.Fatalf("checks = %+v", got.Checks)
	}
	if got := Normalize(resp(200, `{"status":"ok"}`), nil); got.Checks != nil {
		t.Fatalf("a bare status must not invent checks: %+v", got.Checks)
	}
}
