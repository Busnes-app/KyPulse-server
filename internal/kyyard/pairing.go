// Package kyyard pairs kyPulse to one KyYard organization and reads what a pulse_reader
// may: endpoints, container inventory and resource samples. Everything it learns lives in
// memory; the only durable state is the sealed pairing.
package kyyard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const (
	pairingKey   = "kyyard_enc"
	pairingLabel = "kypulse:setting:kyyard"
)

// ErrUnreadable is a pairing that is set but cannot be read back (rotated key, bad JSON).
var ErrUnreadable = errors.New("kyyard: pairing cannot be read")

// Config is the pairing: where KyYard is, which organization, and the token that reads it.
// Token is never logged, audited or returned by any route.
type Config struct {
	URL              string `json:"url"`
	Token            string `json:"token"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
}

// Pairing stores Config sealed under the deployment key.
type Pairing struct {
	Settings store.SettingsStore
	Sealer   recoveryclient.Sealer
}

func NewPairing(cfg *config.Config, s store.SettingsStore) (*Pairing, error) {
	sealer, err := recoveryclient.NewAESGCMSealer(cfg.Security.EncryptionKey, pairingLabel)
	if err != nil {
		return nil, err
	}
	return &Pairing{Settings: s, Sealer: sealer}, nil
}

func (p *Pairing) Save(ctx context.Context, c Config) error {
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := p.Sealer.Seal(plain)
	if err != nil {
		return err
	}
	return p.Settings.SetSetting(ctx, pairingKey, sealed)
}

func (p *Pairing) Load(ctx context.Context) (Config, bool, error) {
	sealed, err := p.Settings.GetSetting(ctx, pairingKey)
	if errors.Is(err, store.ErrNotFound) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	var c Config
	plain, err := p.Sealer.Open(sealed)
	if err == nil {
		err = json.Unmarshal(plain, &c)
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return c, true, nil
}

func (p *Pairing) Delete(ctx context.Context) error {
	return p.Settings.DeleteSetting(ctx, pairingKey)
}
