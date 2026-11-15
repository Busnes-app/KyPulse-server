package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/monitor"
)

// handleListCertificates answers every target that has shown a certificate, soonest expiry
// first, with the thresholds that classify them.
func (s *Server) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.Targets().ListTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list certificates")
		return
	}
	type row struct {
		view     map[string]any
		notAfter time.Time
	}
	var rows []row
	for _, t := range targets {
		notAfter := certNotAfter(t.LastResult)
		if notAfter.IsZero() {
			continue
		}
		rows = append(rows, row{view: targetView(t), notAfter: notAfter})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].notAfter.Before(rows[j].notAfter) })
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view)
	}
	warn, critical := monitor.Thresholds(r.Context(), s.store.Settings())
	s.writeJSON(w, http.StatusOK, map[string]any{"certificates": out, "warn_days": warn, "critical_days": critical})
}

func certNotAfter(lastResult string) time.Time {
	var res struct {
		Cert *struct {
			NotAfter time.Time `json:"not_after"`
		} `json:"cert"`
	}
	if json.Unmarshal([]byte(lastResult), &res) != nil || res.Cert == nil {
		return time.Time{}
	}
	return res.Cert.NotAfter
}

type thresholdsRequest struct {
	WarnDays     int `json:"warn_days"`
	CriticalDays int `json:"critical_days"`
}

func (s *Server) handleSetCertThresholds(w http.ResponseWriter, r *http.Request) {
	var in thresholdsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	if err := monitor.SetThresholds(r.Context(), s.store.Settings(), in.WarnDays, in.CriticalDays); err != nil {
		if errors.Is(err, monitor.ErrBadThresholds) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to save thresholds")
		return
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.cert_thresholds", "", fmt.Sprintf("warn_days=%d critical_days=%d", in.WarnDays, in.CriticalDays))
	s.writeJSON(w, http.StatusOK, map[string]any{"warn_days": in.WarnDays, "critical_days": in.CriticalDays})
}
