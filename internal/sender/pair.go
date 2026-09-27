package sender

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Busnes-app/ky-primitives/keyfile"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

var sourceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Pair consumes a one-time code only after local state has passed its safety checks.
func Pair(ctx context.Context, h HTTP, dir, rawURL, code, name string) (State, error) {
	if h == nil {
		return State{}, errors.New("sender: no HTTP client")
	}
	if err := egress.ValidateURL(rawURL, false); err != nil {
		return State{}, err
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return State{}, errors.New("sender: URL must be a server origin")
	}
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return State{}, errors.New("sender: pairing code must be six digits")
	}
	if !sourceName.MatchString(name) {
		return State{}, errors.New("sender: invalid source name")
	}
	lock, err := Lock(dir)
	if err != nil {
		return State{}, err
	}
	defer lock.Close()
	if err := InspectStateDir(dir); err != nil {
		return State{}, err
	}
	request, _ := json.Marshal(map[string]string{"pairing_code": code, "name": name})
	resp, err := h.Post(ctx, strings.TrimRight(rawURL, "/")+"/api/log-sources/claim", "application/json", request, nil)
	if err != nil {
		return State{}, err
	}
	if resp.StatusCode != 200 {
		return State{}, fmt.Errorf("sender: pairing refused (HTTP %d)", resp.StatusCode)
	}
	var reply struct {
		Token  string `json:"token"`
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
	}
	if err := json.Unmarshal(resp.Body, &reply); err != nil {
		return State{}, errors.New("sender: invalid claim response; revoke the source and pair again")
	}
	token, err := hex.DecodeString(reply.Token)
	if err != nil || len(token) != 32 || reply.Source.ID == "" {
		return State{}, errors.New("sender: invalid claim response; revoke the source and pair again")
	}
	state := State{URL: strings.TrimRight(rawURL, "/"), SourceID: reply.Source.ID, Positions: map[string]Position{}}
	if err := keyfile.Store(filepath.Join(dir, "token"), token, keyfile.Hex); err != nil {
		return State{}, fmt.Errorf("sender: remote claim succeeded but saving the token failed; revoke the source and pair again: %w", err)
	}
	if err := SaveState(dir, state); err != nil {
		return State{}, fmt.Errorf("sender: remote claim succeeded but saving state failed; revoke the source and pair again: %w", err)
	}
	return state, nil
}
