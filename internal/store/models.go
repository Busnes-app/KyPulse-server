package store

import (
	"time"
)

// User represents an identity within the system.
type User struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	PasswordHash       string     `json:"-"`            // Never serialized to JSON
	Role               string     `json:"role"`         // RoleAdmin or RoleViewer
	Status             string     `json:"status"`       // "active", "suspended", "inactive"
	SSOProvider        string     `json:"sso_provider"` // "local", "kysignon", "oidc", "saml"
	SSOSubject         string     `json:"sso_subject,omitempty"`
	TOTPSecretEnc      string     `json:"-"` // AES-256-GCM encrypted
	TOTPEnabled        bool       `json:"totp_enabled"`
	TOTPLastCounter    int64      `json:"-"` // last RFC 6238 counter accepted; refuses replay inside the skew window
	RecoveryCodesHash  string     `json:"-"` // JSON array of sha256 hashes
	PushDeviceID       string     `json:"push_device_id,omitempty"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}

// Session represents an active authenticated user session.
type Session struct {
	TokenHash string    `json:"token_hash"`
	UserID    string    `json:"user_id"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MFAChallenge struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
}

// AuditRecord logs security and operational events with tamper-evident structure.
type AuditRecord struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"` // e.g. "auth.login", "admin.backup_run"
	Resource  string    `json:"resource"`
	Details   string    `json:"details,omitempty"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

// Setting represents a durable server-wide key-value configuration entry.
type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Target is a watched app. Track and LastResult are JSON the monitor package owns
// (alerts.Track and poller.Result); the store never decodes them.
type Target struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	IntervalSec   int        `json:"interval_sec"`
	Enabled       bool       `json:"enabled"`
	Container     string     `json:"container,omitempty"` // KyYard container link, unused until step 3
	State         string     `json:"state"`               // pending|ok|degraded|down, denormalised from Track
	StateSince    time.Time  `json:"state_since"`
	Cause         string     `json:"cause,omitempty"`
	SilencedUntil *time.Time `json:"silenced_until,omitempty"`
	UntilFixed    bool       `json:"until_fixed"`
	TrackJSON     string     `json:"-"`
	LastResult    string     `json:"last_result"` // poller.Result JSON, "" before the first poll
	LastPolledAt  *time.Time `json:"last_polled_at,omitempty"`
	LastLatencyMS int64      `json:"last_latency_ms"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TargetEvent is one transition or reminder, the unit the Alerts tab lists.
type TargetEvent struct {
	ID          int64     `json:"id"`
	TargetID    string    `json:"target_id"`
	At          time.Time `json:"at"`
	FromState   string    `json:"from"`
	ToState     string    `json:"to"`
	Cause       string    `json:"cause,omitempty"`
	Reminder    bool      `json:"reminder"`
	Notified    bool      `json:"notified"`
	NotifyError string    `json:"notify_error,omitempty"`
}

// PollUpdate is what one observation writes back.
type PollUpdate struct {
	PolledAt   time.Time
	LatencyMS  int64
	LastResult string
	TrackJSON  string
	State      string
	StateSince time.Time
	Cause      string
}
