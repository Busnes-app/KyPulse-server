package store

import (
	"context"
	"errors"
	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"time"
)

var (
	ErrNotFound       = errors.New("record not found")
	ErrAlreadyExists  = errors.New("record already exists")
	ErrSessionExpired = errors.New("session expired")
	// ErrSSOSessionEnded means the identity provider already ended the login being signed in.
	ErrSSOSessionEnded = errors.New("SSO session already ended")
)

// Roles. A viewer sees status and alerts; an admin sees everything and changes settings.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// ErrInvalidRole is returned for any role other than RoleAdmin or RoleViewer.
var ErrInvalidRole = errors.New("role must be admin or viewer")

// ErrAuditUnplaceable reports an audit log the store cannot place at open: keyed records with
// no anchor, an anchor counting more records than exist, or a tail that is not the anchor's.
var ErrAuditUnplaceable = errors.New("audit log cannot be placed against its anchor")

func validRole(role string) bool { return role == RoleAdmin || role == RoleViewer }

// Store defines the unified storage contract implemented across SQLite, PostgreSQL, and MySQL.
type Store interface {
	ApplyInstaller(context.Context, config.InstallerBundle, bool) (applysetup.Report, error)
	ApplyInstallerDirectory(context.Context, DirectoryUpdate) error
	Users() UserStore
	Sessions() SessionStore
	Audit() AuditStore
	Settings() SettingsStore
	Targets() TargetStore
	Logs() LogStore
	Sources() SourceStore

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
	// SetTOTPSecret stores a sealed pending secret on a local account while TOTP is off,
	// compare-and-swapped on the previous ciphertext. ErrAlreadyExists when the row moved on
	// (enabled, a concurrent enrolment, not local), so an enabled secret is never replaced.
	SetTOTPSecret(ctx context.Context, userID, oldEnc, newEnc string) error
	// EnableTOTP turns on the pending secretEnc with recoveryHashes, and DisableTOTP clears
	// secret and recovery codes. Each is one transaction that also deletes every session but
	// keepSession (a token hash), every MFA challenge, and chains an audit row.
	// ErrAlreadyExists when the account is not in the expected state, including a passwordHash
	// that has since changed; ErrNotFound when keepSession has been revoked meanwhile.
	EnableTOTP(ctx context.Context, userID, passwordHash, secretEnc, recoveryHashes, keepSession, ip string) error
	DisableTOTP(ctx context.Context, userID, passwordHash, keepSession, ip string) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, offset, limit int, search string) ([]*User, int, error)
	CountUsers(ctx context.Context) (int, error)
}

// SessionStore defines repository operations for active login sessions.
type SessionStore interface {
	// CreateSession inserts s only while the account still has exactly the expected credentials
	// (password hash, active, and the TOTP state the sign-in checked); ErrNotFound otherwise.
	CreateSession(ctx context.Context, s *Session, expected Credentials) error
	GetSession(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	// EndSSOSubject ends the provider subject's sessions whose credentials were verified
	// before cutoff and fences that subject's earlier logins until fenceUntil.
	EndSSOSubject(ctx context.Context, provider, subject string, cutoff, fenceUntil time.Time) error
	// EndSSOSession ends every session of an SSO login and fences its sid until fenceUntil.
	EndSSOSession(ctx context.Context, sid string, fenceUntil time.Time) error
	CleanExpiredSessions(ctx context.Context) error
	CreateMFAChallenge(ctx context.Context, challenge *MFAChallenge, expectedPasswordHash string) error
	ConsumeMFAChallenge(ctx context.Context, tokenHash string) (userID, passwordHash string, err error)
}

// AuditStore logs security events as links of a keyed hash chain.
type AuditStore interface {
	LogAudit(ctx context.Context, r *AuditRecord) error
	ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error)
	// VerifyChain walks every record against the anchor. It never writes.
	VerifyChain(ctx context.Context) (ChainStatus, error)
	// Ready reports whether the next append can chain onto the stored tail.
	Ready(ctx context.Context) error
	// Placement is how the chain was found at open.
	Placement() ChainPlacement
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
	UpdateTarget(ctx context.Context, t *Target) error  // name, url, interval, enabled, container; a new url resets health
	DeleteTarget(ctx context.Context, id string) error  // cascades events
	RecordPoll(ctx context.Context, id string, u PollUpdate) error
	SetSilence(ctx context.Context, id string, until *time.Time, untilFixed bool) error // nil until and false clear it
	ClearUntilFixed(ctx context.Context, id string) error                               // only while until_fixed and no timed silence
	RecordEvent(ctx context.Context, e *TargetEvent) error                              // sets e.ID
	SetEventNotified(ctx context.Context, id int64, notified bool, notifyError string) error
	NextOffboardingAlert(ctx context.Context) (*TargetEvent, error)                                     // oldest pending; ErrNotFound when empty
	ListEvents(ctx context.Context, targetID string, beforeID int64, limit int) ([]*TargetEvent, error) // targetID "" = all; newest id first; beforeID 0 = from the newest
	PruneEvents(ctx context.Context, before time.Time) error                                            // keeps pending offboarding alerts
	TransitionsSince(ctx context.Context, since time.Time) ([]*TargetEvent, error)                      // every target, oldest first
}
