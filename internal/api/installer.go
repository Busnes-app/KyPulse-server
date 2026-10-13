package api

import (
	"context"
	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

// ApplyInstaller compares desired configuration with the server's immutable
// startup snapshot. A committed configuration awaiting restart stays pending.
func (s *Server) ApplyInstaller(ctx context.Context, raw []byte, check bool) (applysetup.Report, error) {
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
	r, err := s.store.ApplyInstaller(ctx, b, check)
	if err != nil {
		return r, err
	}
	if want.SSO != s.config.SSO || want.Backup != s.config.Backup {
		r.Pending = append(r.Pending, applysetup.Object{Kind: "activation", Name: "restart"})
	}
	return r, nil
}
