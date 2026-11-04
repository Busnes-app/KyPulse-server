package api

import (
	"context"
	"log/slog"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

// evInstallerRefused gives the operator the reason the installer CLI does not show: it
// reports only "setup refused; check product configuration locally".
var evInstallerRefused = logging.DeclareEvent("installer_setup_refused", "installer setup refused", slog.LevelWarn)

// ApplyInstaller compares desired configuration with the server's immutable
// startup snapshot. A committed configuration awaiting restart stays pending.
func (s *Server) ApplyInstaller(ctx context.Context, raw []byte, check bool) (r applysetup.Report, err error) {
	defer func() {
		if err != nil {
			s.lg.Log(ctx, evInstallerRefused, logging.Err(err))
		}
	}()
	var b config.InstallerBundle
	if err := applysetup.DecodeRaw(raw, &b); err != nil {
		return applysetup.Report{}, err
	}
	if err := b.ValidateFor(s.config.Server.AppURL); err != nil {
		return applysetup.Report{}, err
	}
	want := *s.config
	if err := config.ApplyInstallerConfiguration(&want, b.Common); err != nil {
		return applysetup.Report{}, err
	}
	if err := b.PrepareMonitoring(&want); err != nil {
		return applysetup.Report{}, err
	}
	r, err = s.store.ApplyInstaller(ctx, b, check)
	if err != nil {
		return r, err
	}
	if want.SSO != s.config.SSO || want.Backup != s.config.Backup {
		r.Pending = append(r.Pending, applysetup.Object{Kind: "activation", Name: "restart"})
	}
	return r, nil
}
