// Package monitor wires the poller, the state machine and the notifier to the store. It
// owns what is due, what each observation means, what gets recorded, and what gets sent.
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const (
	webhookKey       = "alert_webhook_enc"
	webhookStatusKey = "alert_webhook_status"
	webhookLabel     = "kypulse:setting:alert_webhook"
)

// Webhooks stores the admin's webhook sealed under the deployment key, and the last
// delivery result in the clear (it holds no secret).
type Webhooks struct {
	Settings store.SettingsStore
	Sealer   recoveryclient.Sealer
}

func NewWebhooks(cfg *config.Config, s store.SettingsStore) (*Webhooks, error) {
	sealer, err := recoveryclient.NewAESGCMSealer(cfg.Security.EncryptionKey, webhookLabel)
	if err != nil {
		return nil, err
	}
	return &Webhooks{Settings: s, Sealer: sealer}, nil
}

func (w *Webhooks) Save(ctx context.Context, c notify.Config) error {
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := w.Sealer.Seal(plain)
	if err != nil {
		return err
	}
	return w.Settings.SetSetting(ctx, webhookKey, sealed)
}

// Load returns the webhook and whether one is set. A webhook that is set but cannot be read
// back (store error, rotated key, bad JSON) is notify.ErrUnreadable.
func (w *Webhooks) Load(ctx context.Context) (notify.Config, bool, error) {
	sealed, err := w.Settings.GetSetting(ctx, webhookKey)
	if errors.Is(err, store.ErrNotFound) {
		return notify.Config{}, false, nil
	}
	if err != nil {
		return notify.Config{}, false, fmt.Errorf("%w: %w", notify.ErrUnreadable, err)
	}
	var c notify.Config
	plain, err := w.Sealer.Open(sealed)
	if err == nil {
		err = json.Unmarshal(plain, &c)
	}
	if err != nil {
		return notify.Config{}, false, fmt.Errorf("%w: %w", notify.ErrUnreadable, err)
	}
	return c, true, nil
}

func (w *Webhooks) Delete(ctx context.Context) error {
	if err := w.Settings.DeleteSetting(ctx, webhookKey); err != nil {
		return err
	}
	return w.Settings.DeleteSetting(ctx, webhookStatusKey)
}

// DeliveryStatus is the last send's outcome, for the alert bar's "alerts not being delivered".
type DeliveryStatus struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
}

func (w *Webhooks) SetStatus(ctx context.Context, s DeliveryStatus) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return w.Settings.SetSetting(ctx, webhookStatusKey, string(b))
}

func (w *Webhooks) Status(ctx context.Context) (DeliveryStatus, bool, error) {
	raw, err := w.Settings.GetSetting(ctx, webhookStatusKey)
	if errors.Is(err, store.ErrNotFound) {
		return DeliveryStatus{}, false, nil
	}
	if err != nil {
		return DeliveryStatus{}, false, err
	}
	var s DeliveryStatus
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return DeliveryStatus{}, false, err
	}
	return s, true, nil
}
