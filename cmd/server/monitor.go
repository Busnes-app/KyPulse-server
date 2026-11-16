package main

import (
	"context"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/suitedir"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kypost"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// pollTick is how often due targets are looked for; intervals are per target (>= 10 s).
const pollTick = 5 * time.Second

// newMonitor builds the service and the poller that feeds it. Health polls use an egress
// client that admits plain http; the webhook client admits it only by opt-in.
func newMonitor(cfg *config.Config, st store.Store, lg *logging.Logger) (*monitor.Service, *poller.Poller, error) {
	webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		return nil, nil, err
	}
	// "Email through KyPost" pairs through KyIdentity only: the mailbox owner is named by
	// their KyIdentity subject. KyPost is reached through the same guard as every egress.
	link := &kypost.Link{Post: egress.New(egress.Options{})}
	if s := cfg.SSO; s.Enabled && s.KySignOnIssuer != "" && s.KySignOnClientID != "" && s.KySignOnSecret != "" {
		link.Dir = &suitedir.Client{Issuer: s.KySignOnIssuer, ClientID: s.KySignOnClientID, ClientSecret: s.KySignOnSecret}
	}
	mail, err := monitor.NewMail(cfg, st, link)
	if err != nil {
		return nil, nil, err
	}
	svc := &monitor.Service{Mail: mail,
		Store: st, Webhooks: webhooks, Logger: lg, AppURL: cfg.Server.AppURL,
		Notifier: &notify.Notifier{Post: egress.New(egress.Options{AllowHTTP: cfg.Alerts.AllowHTTP}), Backoff: notify.DefaultBackoff},
	}
	p := &poller.Poller{
		Get:     egress.New(egress.Options{AllowHTTP: true}),
		Workers: cfg.Poll.Workers,
		Due:     svc.Due,
		Observe: svc.Observe,
		OnError: svc.DueFailed,
	}
	return svc, p, nil
}

// monitorLoop runs the poller until ctx ends and closes done once in-flight polls have
// finished and queued alerts are delivered, so runServer can close the store behind it.
func monitorLoop(ctx context.Context, svc *monitor.Service, p *poller.Poller, done chan<- struct{}) {
	defer close(done)
	svc.Start(ctx)
	go svc.Mail.Link.Run(ctx)
	p.Run(ctx, pollTick)
	svc.Drain()
}
