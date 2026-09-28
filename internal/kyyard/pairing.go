// Package kyyard pairs kyPulse to one KyYard organization and reads what a pulse_reader
// may: inventory/samples in memory, and logs/audit with durable generation-scoped cursors.
package kyyard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

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
	Generation       string `json:"generation"`
	URL              string `json:"url"`
	Token            string `json:"token"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
}

// Pairing stores Config sealed under the deployment key.
type Pairing struct {
	mu sync.Mutex // serializes replacement, legacy migration and imported commits

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
	p.mu.Lock()
	defer p.mu.Unlock()
	c.Generation = newGeneration()
	return p.saveLocked(ctx, c)
}

func newGeneration() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}

func (p *Pairing) saveLocked(ctx context.Context, c Config) error {
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
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loadLocked(ctx)
}

func (p *Pairing) loadLocked(ctx context.Context) (Config, bool, error) {
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
	if c.Generation == "" {
		c.Generation = newGeneration()
		if err := p.saveLocked(ctx, c); err != nil {
			return Config{}, false, err
		}
	}
	return c, true, nil
}

func (p *Pairing) Delete(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Settings.DeleteSetting(ctx, pairingKey)
}
