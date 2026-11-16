package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kypost"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const (
	mailKey       = "alert_kypost_enc" // sealed: the recipients are personal data
	mailStatusKey = "alert_kypost_status"
	mailLabel     = "kypulse:setting:alert_kypost"
)

// MailConfig is the "Email through KyPost" channel: whose mailbox sends, and to whom.
type MailConfig struct {
	OwnerID    string   `json:"owner_id"`
	Recipients []string `json:"recipients"`
}

// ErrBadMailConfig is Validate's refusal.
var ErrBadMailConfig = errors.New("pick a mailbox owner and 1 to 11 distinct email addresses")

// Validate canonicalises the recipients (bare, lowercased, distinct) and checks the count.
func (c *MailConfig) Validate() error {
	seen := map[string]bool{}
	var out []string
	for _, r := range c.Recipients {
		r = strings.ToLower(strings.TrimSpace(r))
		a, err := mail.ParseAddress(r)
		if err != nil || a.Address != r || a.Name != "" || len(r) > 254 || seen[r] {
			return ErrBadMailConfig
		}
		seen[r] = true
		out = append(out, r)
	}
	if c.OwnerID == "" || len(out) == 0 || len(out) > kypost.MaxRecipients {
		return ErrBadMailConfig
	}
	c.Recipients = out
	return nil
}

// Mail is the KyPost channel. It is nil-safe: a nil *Mail is never configured.
type Mail struct {
	Settings store.SettingsStore
	Users    store.UserStore
	Sealer   recoveryclient.Sealer
	Link     *kypost.Link
	Issuer   string          // KyIdentity issuer the owner's subject belongs to
	Backoff  []time.Duration // waits between attempts; nil is one attempt
}

func NewMail(cfg *config.Config, st store.Store, link *kypost.Link) (*Mail, error) {
	sealer, err := recoveryclient.NewAESGCMSealer(cfg.Security.EncryptionKey, mailLabel)
	if err != nil {
		return nil, err
	}
	return &Mail{Settings: st.Settings(), Users: st.Users(), Sealer: sealer, Link: link,
		Issuer: cfg.SSO.KySignOnIssuer, Backoff: notify.DefaultBackoff}, nil
}

// Eligible is whether u can own the mailbox: an active account signed in through KyIdentity.
func Eligible(u *store.User) bool {
	return u != nil && u.Status == "active" && u.SSOProvider == "kysignon" && u.SSOSubject != ""
}

func (m *Mail) Save(ctx context.Context, c MailConfig) error {
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := m.Sealer.Seal(plain)
	if err != nil {
		return err
	}
	return m.Settings.SetSetting(ctx, mailKey, sealed)
}

// Load returns the channel and whether one is set; one that cannot be read back is
// notify.ErrUnreadable.
func (m *Mail) Load(ctx context.Context) (MailConfig, bool, error) {
	if m == nil {
		return MailConfig{}, false, nil
	}
	sealed, err := m.Settings.GetSetting(ctx, mailKey)
	if errors.Is(err, store.ErrNotFound) {
		return MailConfig{}, false, nil
	}
	var c MailConfig
	if err == nil {
		var plain []byte
		if plain, err = m.Sealer.Open(sealed); err == nil {
			err = json.Unmarshal(plain, &c)
		}
	}
	if err != nil {
		return MailConfig{}, false, fmt.Errorf("%w: %w", notify.ErrUnreadable, err)
	}
	return c, true, nil
}

func (m *Mail) Delete(ctx context.Context) error {
	if err := m.Settings.DeleteSetting(ctx, mailKey); err != nil {
		return err
	}
	return m.Settings.DeleteSetting(ctx, mailStatusKey)
}

func (m *Mail) SetStatus(ctx context.Context, s DeliveryStatus) error {
	b, _ := json.Marshal(s)
	return m.Settings.SetSetting(ctx, mailStatusKey, string(b))
}

func (m *Mail) Status(ctx context.Context) (DeliveryStatus, bool) {
	raw, err := m.Settings.GetSetting(ctx, mailStatusKey)
	var s DeliveryStatus
	if err != nil || json.Unmarshal([]byte(raw), &s) != nil {
		return DeliveryStatus{}, false
	}
	return s, true
}

// errOwner is a configured owner who can no longer send: deleted, disabled or unlinked.
var errOwner = kypost.Refused{Code: "owner_unavailable"}

// Send delivers msg under id, retrying a retryable failure over Backoff when retry is set.
// The error is for kypost.Reason only: it never carries an address.
func (m *Mail) Send(ctx context.Context, cfg MailConfig, id string, msg notify.Message, retry bool) error {
	u, err := m.Users.GetUserByID(ctx, cfg.OwnerID)
	if err != nil || !Eligible(u) {
		return errOwner
	}
	subject := strings.TrimPrefix(msg.Title(), "KyPulse: ")
	if len(subject) > kypost.MaxSubject {
		subject = subject[:kypost.MaxSubject]
	}
	req := kypost.SendRequest{Sub: u.SSOSubject, Issuer: m.Issuer, Recipients: cfg.Recipients, Subject: subject, Text: msg.Text()}
	waits := m.Backoff
	if !retry {
		waits = nil
	}
	for i := 0; ; i++ {
		err = m.Link.Send(ctx, id, req)
		if err == nil || !errors.Is(err, kypost.ErrRetry) || i >= len(waits) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waits[i]):
		}
	}
}
