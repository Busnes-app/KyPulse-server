package sso

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// KySignOnClient manages interactions with the central KySignOn identity provider.
type KySignOnClient struct {
	config config.SSOConfig
	store  store.Store
	flow   *oauthFlow
	mu     sync.Mutex
	seen   map[[32]byte]time.Time // sync body hash -> end of its timestamp window
}

func NewKySignOnClient(cfg config.SSOConfig, st store.Store) *KySignOnClient {
	return &KySignOnClient{
		config: cfg,
		store:  st,
		flow:   newOAuthFlow(cfg.KySignOnIssuer, cfg.KySignOnClientID, cfg.KySignOnSecret),
		seen:   make(map[[32]byte]time.Time),
	}
}

// BuildAuthURL generates the authorization code URL with PKCE for KySignOn.
func (k *KySignOnClient) BuildAuthURL(ctx context.Context, redirectURI, state, verifier, nonce string) (string, error) {
	return k.flow.authCodeURL(ctx, redirectURI, state, verifier, nonce)
}

// ExchangeCode exchanges the authorization code and verifier for identity claims.
func (k *KySignOnClient) ExchangeCode(ctx context.Context, code, verifier, redirectURI, expectedNonce string) (*IdentityClaims, error) {
	claims, err := k.flow.exchange(ctx, code, verifier, redirectURI, expectedNonce)
	if err != nil {
		return nil, err
	}
	claims.Provider = "kysignon"
	return claims, nil
}

// KySignOnSyncPayload defines the schema received during automatic directory replication webhooks.
type KySignOnSyncPayload struct {
	Event       string `json:"event"` // "user.created", "user.updated", "user.deactivated", "user.deleted"
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Timestamp   int64  `json:"timestamp"`
}

// HandleSyncWebhook processes inbound HMAC-SHA256 signed user updates from KySignOn server.
func (k *KySignOnClient) HandleSyncWebhook(ctx context.Context, body []byte, signature string) error {
	if len(k.config.KySignOnHMACSecret) < crypto.MinHMACKeyBytes {
		return fmt.Errorf("webhook HMAC secret must contain at least %d bytes", crypto.MinHMACKeyBytes)
	}

	if !crypto.VerifyHMACSHA256(body, k.config.KySignOnHMACSecret, signature) {
		return errors.New("invalid webhook signature")
	}

	var payload KySignOnSyncPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid json payload: %w", err)
	}
	sent := time.Unix(payload.Timestamp, 0)
	if payload.Timestamp == 0 || time.Since(sent).Abs() > syncWindow {
		return errors.New("webhook timestamp is missing or expired")
	}
	// The payload carries no event ID, so the signed body itself is the replay key.
	key := sha256.Sum256(body)
	if err := k.reserve(key, sent.Add(syncWindow)); err != nil {
		return err
	}
	if err := k.applySync(ctx, payload); err != nil {
		k.release(key) // a failed delivery may be retried
		return err
	}
	return nil
}

// syncWindow is how far a webhook timestamp may drift; syncSeenCap bounds the replay set.
// Only a correctly signed body is recorded, so only the identity provider can fill it, and
// a full set refuses rather than forgets.
const (
	syncWindow  = 5 * time.Minute
	syncSeenCap = 10000
)

func (k *KySignOnClient) reserve(key [32]byte, until time.Time) error {
	now := time.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, dup := k.seen[key]; dup {
		return errors.New("webhook already delivered")
	}
	if len(k.seen) >= syncSeenCap {
		for h, exp := range k.seen {
			if now.After(exp) {
				delete(k.seen, h)
			}
		}
		if len(k.seen) >= syncSeenCap {
			return errors.New("too many webhook deliveries")
		}
	}
	k.seen[key] = until
	return nil
}

func (k *KySignOnClient) release(key [32]byte) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.seen, key)
}

func (k *KySignOnClient) applySync(ctx context.Context, payload KySignOnSyncPayload) error {
	switch payload.Event {
	case "user.created", "user.updated":
		existing, err := k.store.Users().GetUserBySSO(ctx, "kysignon", payload.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}

		role := RoleFor(payload.Role)
		status := payload.Status
		if status == "" {
			status = "active"
		}

		if existing != nil {
			privilegesChanged := existing.Role != role || existing.Status != status
			existing.Username = payload.Username
			existing.Email = payload.Email
			existing.DisplayName = payload.DisplayName
			existing.Role = role
			existing.Status = status
			if err := k.store.Users().UpdateUser(ctx, existing); err != nil {
				return err
			}
			if privilegesChanged {
				return k.store.Sessions().DeleteUserSessions(ctx, existing.ID)
			}
			return nil
		}

		newUser := &store.User{
			ID:          fmt.Sprintf("usr_%s", crypto.RandomHex(12)),
			Username:    payload.Username,
			Email:       payload.Email,
			DisplayName: payload.DisplayName,
			Role:        role,
			Status:      status,
			SSOProvider: "kysignon",
			SSOSubject:  payload.ID,
		}
		return k.store.Users().CreateUser(ctx, newUser)

	case "user.deactivated":
		existing, err := k.store.Users().GetUserBySSO(ctx, "kysignon", payload.ID)
		if err != nil {
			return nil // User might not exist locally
		}
		existing.Status = "inactive"
		if err := k.store.Users().UpdateUser(ctx, existing); err != nil {
			return err
		}
		return k.store.Sessions().DeleteUserSessions(ctx, existing.ID)

	case "user.deleted":
		existing, err := k.store.Users().GetUserBySSO(ctx, "kysignon", payload.ID)
		if err != nil {
			return nil
		}
		return k.store.Users().DeleteUser(ctx, existing.ID)
	}

	return nil
}
