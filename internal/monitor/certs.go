package monitor

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Busnes-app/kypulse-server/internal/alerts"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// settingCertThresholds holds "<warn>,<critical>" days left: one row, so a write is atomic.
const settingCertThresholds = "cert_thresholds"

// ErrBadThresholds is SetThresholds' answer to an invalid pair.
var ErrBadThresholds = errors.New("thresholds must satisfy 1 <= critical_days < warn_days <= 365")

// Thresholds returns the administrator's certificate thresholds, or the defaults when unset
// or unreadable.
func Thresholds(ctx context.Context, st store.SettingsStore) (warn, critical int) {
	warn, critical = alerts.DefaultCertWarnDays, alerts.DefaultCertCriticalDays
	v, err := st.GetSetting(ctx, settingCertThresholds)
	w, c, ok := strings.Cut(v, ",")
	if err != nil || !ok {
		return
	}
	wn, err1 := strconv.Atoi(w)
	cn, err2 := strconv.Atoi(c)
	if err1 != nil || err2 != nil || validThresholds(wn, cn) != nil {
		return
	}
	return wn, cn
}

// SetThresholds stores a valid pair.
func SetThresholds(ctx context.Context, st store.SettingsStore, warn, critical int) error {
	if err := validThresholds(warn, critical); err != nil {
		return err
	}
	return st.SetSetting(ctx, settingCertThresholds, strconv.Itoa(warn)+","+strconv.Itoa(critical))
}

func validThresholds(warn, critical int) error {
	if critical < 1 || critical >= warn || warn > 365 {
		return ErrBadThresholds
	}
	return nil
}
