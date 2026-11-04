package sso

import (
	"errors"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

var (
	ErrInvalidState       = errors.New("invalid or expired SSO state")
	ErrInvalidIDToken     = errors.New("invalid ID token")
	ErrProviderDisabled   = errors.New("SSO provider is not enabled")
	ErrInvalidLogoutToken = errors.New("invalid logout token")
)

// IdentityClaims holds the unified identity extracted from any SSO provider.
type IdentityClaims struct {
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Role              string `json:"role,omitempty"`
	// Roles is KyIdentity's per-app `roles` claim; nil when the ID token has none.
	Roles    []string `json:"roles,omitempty"`
	Provider string   `json:"provider"` // "kysignon", "oidc", "saml"
	// SessionID is the provider's ID token `sid`; back-channel logout names the login by it.
	SessionID string `json:"sid,omitempty"`
	// IssuedAt is the ID token's iat: when the provider vouched for this login.
	IssuedAt time.Time `json:"-"`
	// AuthTime is the ID token's auth_time: when the user last gave the provider their
	// credentials. Zero when the provider did not say.
	AuthTime time.Time `json:"-"`
}

// RoleFor maps a role asserted by an identity provider onto KyPulse's two roles. Only an
// exact "admin" grants admin; every other value, including a missing one, is a viewer, so a
// provider that speaks another product's role vocabulary cannot grant privilege by accident.
func RoleFor(claimed string) string {
	if claimed == store.RoleAdmin {
		return store.RoleAdmin
	}
	return store.RoleViewer
}

// LocalRole maps KyIdentity's role claims onto KyPulse's roles: "admin" in the per-app
// `roles` claim means admin, otherwise viewer. Without `roles` the legacy IdP-wide `role`
// goes through RoleFor. ok is false when the token carries neither, and then a stored role
// must not change.
func (c *IdentityClaims) LocalRole() (role string, ok bool) {
	switch {
	case c.Roles != nil:
		for _, r := range c.Roles {
			if r == store.RoleAdmin {
				return store.RoleAdmin, true
			}
		}
		return store.RoleViewer, true
	case c.Role != "":
		return RoleFor(c.Role), true
	}
	return "", false
}

// SSOState represents ephemeral state held during OAuth/OIDC authorization flow.
type SSOState struct {
	State        string    `json:"state"`
	Verifier     string    `json:"verifier"`
	Provider     string    `json:"provider"`
	RedirectBack string    `json:"redirect_back"`
	CreatedAt    time.Time `json:"created_at"`
}
