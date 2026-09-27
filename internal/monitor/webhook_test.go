package monitor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func testStore(t *testing.T) (*config.Config, store.Store) {
	t.Helper()
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = testdb.Config(t)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return cfg, st
}

func TestWebhookIsSealedAtRest(t *testing.T) {
	ctx := context.Background()
	cfg, st := testStore(t)
	w, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := w.Load(ctx); err != nil || ok {
		t.Fatalf("unset: ok=%v err=%v", ok, err)
	}
	in := notify.Config{Preset: notify.Gotify, URL: "https://gotify.lan", Token: "gk-secret"}
	if err := w.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	raw, err := st.Settings().GetSetting(ctx, "alert_webhook_enc")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "gk-secret") || strings.Contains(raw, "gotify.lan") {
		t.Fatalf("stored row is not sealed: %q", raw)
	}
	out, ok, err := w.Load(ctx)
	if err != nil || !ok || out != in {
		t.Fatalf("round trip: %+v ok=%v err=%v", out, ok, err)
	}

	// A different deployment key must not open it.
	other := *cfg
	other.Security.EncryptionKey = []byte(strings.Repeat("k", 32))
	w2, _ := monitor.NewWebhooks(&other, st.Settings())
	if _, _, err := w2.Load(ctx); err == nil {
		t.Fatal("another key opened the webhook")
	}

	if err := w.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := w.Load(ctx); ok {
		t.Fatal("still set after delete")
	}
}

func TestDeliveryStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	cfg, st := testStore(t)
	w, _ := monitor.NewWebhooks(cfg, st.Settings())
	if _, ok, _ := w.Status(ctx); ok {
		t.Fatal("status before any send")
	}
	at := time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC)
	if err := w.SetStatus(ctx, monitor.DeliveryStatus{At: at, OK: false, Error: "receiver answered 503"}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := w.Status(ctx)
	if err != nil || !ok || s.OK || !s.At.Equal(at) || s.Error != "receiver answered 503" {
		t.Fatalf("status: %+v ok=%v err=%v", s, ok, err)
	}
}
