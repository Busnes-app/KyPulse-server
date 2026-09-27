package kyyard_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func TestPairingSealsAndFiltersFromSettings(t *testing.T) {
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := kyyard.NewPairing(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := p.Load(context.Background()); err != nil || ok {
		t.Fatalf("empty: %v %v", ok, err)
	}
	if err := p.Save(context.Background(), kyyard.Config{URL: "https://yard.lan", Token: "secret-token", OrganizationID: "org_a", OrganizationName: "A"}); err != nil {
		t.Fatal(err)
	}
	all, _ := st.Settings().GetAllSettings(context.Background())
	for k, v := range all {
		if k != "kyyard_enc" && strings.Contains(v, "secret-token") {
			t.Fatalf("token leaked into %s", k)
		}
	}
	if raw := all["kyyard_enc"]; raw == "" || strings.Contains(raw, "secret-token") {
		t.Fatal("kyyard_enc must be sealed")
	}
	got, ok, err := p.Load(context.Background())
	if err != nil || !ok || got.Token != "secret-token" || got.OrganizationName != "A" {
		t.Fatalf("load: %+v %v %v", got, ok, err)
	}
	if err := p.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.Load(context.Background()); ok {
		t.Fatal("deleted pairing still loads")
	}
}
