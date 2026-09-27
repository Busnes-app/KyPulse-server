package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("record not found")
	ErrAlreadyExists  = errors.New("record already exists")
	ErrSessionExpired = errors.New("session expired")
)

// Roles. A viewer sees status and alerts; an admin sees everything and changes settings.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// ErrInvalidRole is returned for any role other than RoleAdmin or RoleViewer.
var ErrInvalidRole = errors.New("role must be admin or viewer")

func validRole(role string) bool { return role == RoleAdmin || role == RoleViewer }

// Store defines the unified storage contract implemented across SQLite, PostgreSQL, and MySQL.
type Store interface {
	Users() UserStore
	Sessions() SessionStore
	Audit() AuditStore
	Settings() SettingsStore
	Targets() TargetStore

	Driver() string
	Ping(ctx context.Context) error
	Close() error
}

// UserStore defines repository operations for accounts.
type UserStore interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserBySSO(ctx context.Context, provider, subject string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error
	ResetAdminPassword(ctx context.Context, userID, newHash string) error
	CompletePasswordChange(ctx context.Context, userID, oldHash, newHash, ip string) error
	UpdateRecoveryCodes(ctx context.Context, userID, oldHashes, newHashes string) error
	// SpendTOTPCounter records counter as used. It returns ErrAlreadyExists when counter is
	// not greater than the stored one, which is how a replayed code inside the skew window fails.
	SpendTOTPCounter(ctx context.Context, userID string, counter int64) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, offset, limit int, search string) ([]*User, int, error)
	CountUsers(ctx context.Context) (int, error)
}

// SessionStore defines repository operations for active login sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, s *Session, expectedPasswordHash string) error
	GetSession(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	CleanExpiredSessions(ctx context.Context) error
	CreateMFAChallenge(ctx context.Context, challenge *MFAChallenge, expectedPasswordHash string) error
	ConsumeMFAChallenge(ctx context.Context, tokenHash string) (userID, passwordHash string, err error)
}

// AuditStore logs security events.
type AuditStore interface {
	LogAudit(ctx context.Context, r *AuditRecord) error
	ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error)
}

// SettingsStore handles persistent key-value configuration.
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, val string) error
	DeleteSetting(ctx context.Context, key string) error
	GetAllSettings(ctx context.Context) (map[string]string, error)
}

// TargetStore handles watched apps and their state transition events.
type TargetStore interface {
	CreateTarget(ctx context.Context, t *Target) error // ErrAlreadyExists on a duplicate name
	GetTarget(ctx context.Context, id string) (*Target, error)
	ListTargets(ctx context.Context) ([]*Target, error) // ordered by name
	UpdateTarget(ctx context.Context, t *Target) error  // name, url, interval, enabled, container only
	DeleteTarget(ctx context.Context, id string) error  // cascades events
	RecordPoll(ctx context.Context, id string, u PollUpdate) error
	SetSilence(ctx context.Context, id string, until *time.Time, untilFixed bool) error // nil until and false clear it
	RecordEvent(ctx context.Context, e *TargetEvent) error                              // sets e.ID
	SetEventNotified(ctx context.Context, id int64, notified bool, notifyError string) error
	ListEvents(ctx context.Context, targetID string, offset, limit int) ([]*TargetEvent, int, error) // targetID "" = all; newest first
}
