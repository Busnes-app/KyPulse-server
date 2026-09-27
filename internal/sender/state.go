package sender

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/Busnes-app/ky-primitives/keyfile"
)

type Position struct {
	Kind      string `json:"kind"`
	Input     string `json:"input"`
	Device    uint64 `json:"device,omitempty"`
	Inode     uint64 `json:"inode,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Ordinal   int    `json:"ordinal,omitempty"`
}

type State struct {
	URL       string              `json:"url"`
	SourceID  string              `json:"source_id"`
	Positions map[string]Position `json:"positions"`
}

func PositionKey(p Position) string { return p.Kind + ":" + p.Input }

func DefaultStateDir() (string, error) {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "kypulse-send"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "kypulse-send"), nil
}

func checkDir(dir string) error {
	fi, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		fi, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !ok || st.Uid != uint32(os.Getuid()) || fi.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("sender: unsafe state directory %s", dir)
	}
	return nil
}

func checkRegular(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok || st.Uid != uint32(os.Getuid()) || fi.Mode().Perm()&0077 != 0 {
		return false, fmt.Errorf("sender: unsafe state file %s", path)
	}
	return true, nil
}

// InspectStateDir validates all existing paths before a remote claim consumes its code.
func InspectStateDir(dir string) error {
	if err := checkDir(dir); err != nil {
		return err
	}
	stateExists, err := checkRegular(filepath.Join(dir, "positions.json"))
	if err != nil {
		return err
	}
	tokenPath := filepath.Join(dir, "token")
	tokenExists, err := checkRegular(tokenPath)
	if err != nil {
		return err
	}
	if tokenExists {
		if _, err = keyfile.Load(tokenPath, 32); err != nil {
			return err
		}
	}
	if stateExists {
		if _, err = readState(dir); err != nil {
			return err
		}
	}
	if stateExists || tokenExists {
		return errors.New("sender: state directory already paired")
	}
	return nil
}

func readState(dir string) (State, error) {
	path := filepath.Join(dir, "positions.json")
	before, err := os.Lstat(path)
	if err != nil {
		return State{}, err
	}
	if _, err := checkRegular(path); err != nil {
		return State{}, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return State{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return State{}, errors.New("sender: state file changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return State{}, err
	}
	if len(data) > 1<<20 {
		return State{}, errors.New("sender: state file too large")
	}
	var state State
	if err = json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("sender: invalid state: %w", err)
	}
	if state.URL == "" || state.SourceID == "" || state.Positions == nil {
		return State{}, errors.New("sender: incomplete state")
	}
	return state, nil
}

func LoadState(dir string) (State, []byte, error) {
	if err := checkDir(dir); err != nil {
		return State{}, nil, err
	}
	state, err := readState(dir)
	if err != nil {
		return State{}, nil, err
	}
	token, err := keyfile.Load(filepath.Join(dir, "token"), 32)
	return state, token, err
}

func SaveState(dir string, state State) error {
	if err := checkDir(dir); err != nil {
		return err
	}
	if _, err := checkRegular(filepath.Join(dir, "positions.json")); err != nil {
		return err
	}
	if state.URL == "" || state.SourceID == "" || state.Positions == nil {
		return errors.New("sender: incomplete state")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".positions-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, "positions.json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

type LockFile struct{ file *os.File }

func Lock(dir string) (*LockFile, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("sender: state locking requires Linux")
	}
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lock")
	if _, err := checkRegular(path); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if _, err := checkRegular(path); err != nil {
		f.Close()
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("sender: state directory already in use: %w", err)
	}
	return &LockFile{f}, nil
}

func (l *LockFile) Close() error { return l.file.Close() }
