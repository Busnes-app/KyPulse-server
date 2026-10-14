package api

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func refusalServer(t *testing.T) (*Server, store.Store, *time.Time) {
	t.Helper()
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, st, lg, nil, nil)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.refusalNow = func() time.Time { return now }
	return s, st, &now
}

func refuseFrom(s *Server, addr string) {
	r := httptest.NewRequest("GET", "/api/logs", nil)
	r.RemoteAddr = addr
	s.auditRefusal(r.Context(), "", r, "admin.log_list", "outcome=refused reason=authentication")
}

func auditRows(t *testing.T, st store.Store) []*store.AuditRecord {
	t.Helper()
	rows, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRefusalsAreSummarisedPerClient(t *testing.T) {
	s, st, now := refusalServer(t)
	for i := 0; i < 50; i++ {
		refuseFrom(s, "203.0.113.7:1000")
		refuseFrom(s, fmt.Sprintf("[2001:db8:1:2::%x]:1000", i+1)) // one /64
	}
	rows := auditRows(t, st)
	if len(rows) != 2 {
		t.Fatalf("100 refusals from two clients wrote %d rows, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Action != "admin.log_list" || row.Details != "outcome=refused reason=authentication" {
			t.Fatalf("first refusal row: %+v", row)
		}
	}

	*now = now.Add(refusalWindow + time.Second)
	refuseFrom(s, "198.51.100.1:1000")
	summaries := map[string]string{}
	for _, row := range auditRows(t, st)[:3] {
		summaries[row.IPAddress] = row.Details
	}
	for _, ip := range []string{"203.0.113.7", "2001:db8:1:2::/64"} {
		if summaries[ip] != "outcome=refused reason=authentication repeats=49" {
			t.Fatalf("summary for %s: %q (rows %v)", ip, summaries[ip], summaries)
		}
	}
	if summaries["198.51.100.1"] != "outcome=refused reason=authentication" {
		t.Fatalf("new window must write its first refusal: %v", summaries)
	}
}

func TestRefusalBucketsAreBounded(t *testing.T) {
	s, st, now := refusalServer(t)
	for i := 0; i < refusalCap+100; i++ {
		refuseFrom(s, fmt.Sprintf("10.%d.%d.1:1000", i/256, i%256))
	}
	if n := len(s.refusals); n > refusalCap+1 {
		t.Fatalf("%d buckets, want at most %d", n, refusalCap+1)
	}
	if n := len(auditRows(t, st)); n != refusalCap+1 {
		t.Fatalf("%d rows, want %d: past the cap new clients share one bucket", n, refusalCap+1)
	}
	*now = now.Add(refusalWindow + time.Second)
	refuseFrom(s, "192.0.2.1:1000")
	found := false
	for _, row := range auditRows(t, st) {
		if row.Action == "audit.refusal_overflow" && strings.HasSuffix(row.Details, "repeats=99") {
			found = true
		}
	}
	if !found {
		t.Fatal("overflow repeats were not summarised")
	}
}
