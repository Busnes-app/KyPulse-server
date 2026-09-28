package main

import (
	"context"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// newKyYard builds the KyYard reader. Its egress client admits plain http only by opt-in.
func newKyYard(cfg *config.Config, st store.Store, lg *logging.Logger) (*kyyard.Service, error) {
	pairing, err := kyyard.NewPairing(cfg, st.Settings())
	if err != nil {
		return nil, err
	}
	// KyYard caps an inventory snapshot at 1 MiB (protocol.MaxSnapshotBytes); 2 MiB leaves room
	// for the envelope around it.
	return &kyyard.Service{Pairing: pairing, HTTP: egress.New(egress.Options{AllowHTTP: cfg.KyYard.AllowHTTP, MaxBody: 2 << 20}), LogHTTP: egress.New(egress.Options{AllowHTTP: cfg.KyYard.AllowHTTP, MaxBody: kyyard.MaxLogResponse, Timeout: 30 * time.Second}), Store: st, MaxLogBytes: cfg.Logs.MaxBytes, Logger: lg}, nil
}

// kyyardLoop refreshes the snapshot until ctx ends and closes done.
func kyyardLoop(ctx context.Context, svc *kyyard.Service, done chan<- struct{}) {
	svc.Run(ctx, kyyard.PullEvery, done)
}
