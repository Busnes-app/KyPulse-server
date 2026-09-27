package kyyard

import (
	"regexp"
	"strconv"
	"time"
)

var (
	exitRe   = regexp.MustCompile(`^(?:Exited|Restarting) \((\d+)\)`)
	healthRe = regexp.MustCompile(`\((healthy|unhealthy|health: starting)\)`)
)

// ParseStatus reads Docker's status text: health from the "(healthy)" suffix Docker adds
// for a container with a healthcheck, exit code from "Exited (N)" or "Restarting (N)".
func ParseStatus(status string) (health string, exitCode *int) {
	health = "none"
	if m := healthRe.FindStringSubmatch(status); m != nil {
		switch m[1] {
		case "healthy", "unhealthy":
			health = m[1]
		default:
			health = "starting"
		}
	}
	if m := exitRe.FindStringSubmatch(status); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			exitCode = &n
		}
	}
	return health, exitCode
}

// LinkFor is the value a target's container field holds.
func LinkFor(endpointID, name string) string { return endpointID + "/" + name }

// ContainerFacts is what the app detail page shows for a linked container.
type ContainerFacts struct {
	Link            string    `json:"link"`
	EndpointID      string    `json:"endpoint_id"`
	EndpointName    string    `json:"endpoint_name"`
	ContainerID     string    `json:"container_id"`
	Name            string    `json:"name"`
	Image           string    `json:"image"`
	State           string    `json:"state"`
	Status          string    `json:"status"`
	Health          string    `json:"health"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	ObservedAt      time.Time `json:"observed_at"` // inventory time
	EndpointOffline bool      `json:"endpoint_offline"`
	// Memory and restart fields mean something only when HasSample; KyYard samples running
	// containers only.
	HasSample        bool       `json:"has_sample"`
	SampleAt         *time.Time `json:"sample_at,omitempty"`
	MemoryBytes      int64      `json:"memory_bytes"`
	MemoryLimit      int64      `json:"memory_limit"`
	RestartCount     int64      `json:"restart_count"`
	RestartsLastHour int64      `json:"restarts_last_hour"`
	HistoryMinutes   int        `json:"history_minutes"` // span RestartsLastHour covers, capped at 60
	Stale            bool       `json:"stale"`
}

// Suggestion prefills the add form: the container's name and its link.
type Suggestion struct {
	Link         string `json:"link"`
	EndpointName string `json:"endpoint_name"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
}

// StatusView is the pairing as the API and the alert bar see it. The token is not here.
type StatusView struct {
	Paired       bool       `json:"paired"`
	URL          string     `json:"url,omitempty"`
	Organization string     `json:"organization,omitempty"`
	FetchedAt    *time.Time `json:"fetched_at,omitempty"`
	Stale        bool       `json:"stale"`
	Error        string     `json:"error,omitempty"`
}
