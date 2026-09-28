package sender

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

func device(fi os.FileInfo) uint64 { return uint64(fi.Sys().(*syscall.Stat_t).Dev) }
func inode(fi os.FileInfo) uint64  { return fi.Sys().(*syscall.Stat_t).Ino }
func samePosition(fi os.FileInfo, p Position) bool {
	return device(fi) == p.Device && inode(fi) == p.Inode
}

// fingerprintAt binds a checkpoint to the last 64 consumed bytes. A matching
// suffix cannot distinguish all histories, but a mismatch prevents a silent skip.
func fingerprintAt(f *os.File, offset int64) (string, error) {
	if offset < 0 {
		return "", errors.New("sender: negative file offset")
	}
	start := offset - int64(64)
	if start < 0 {
		start = 0
	}
	buf := make([]byte, offset-start)
	if len(buf) > 0 {
		if _, err := f.ReadAt(buf, start); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

func openRegular(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("sender: file input is not regular: %s", path)
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("sender: file input is not regular: %s", path)
	}
	if !os.SameFile(before, fi) {
		f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, fi, nil
}

func matchesSaved(f *os.File, fi os.FileInfo, p Position) bool {
	if !samePosition(fi, p) || fi.Size() < p.Offset || p.Fingerprint == "" {
		return false
	}
	actual, err := fingerprintAt(f, p.Offset)
	return err == nil && actual == p.Fingerprint
}

func sibling(path string, p Position) string {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		candidate := filepath.Join(filepath.Dir(path), entry.Name())
		f, fi, err := openRegular(candidate)
		if err == nil && matchesSaved(f, fi, p) {
			f.Close()
			return candidate
		}
		if f != nil {
			f.Close()
		}
	}
	return ""
}

// ReadFile follows a regular file by device and inode. A truncate and regrow
// entirely between polls cannot be distinguished from an unchanged file.
func ReadFile(ctx context.Context, path string, start Position, out chan<- Item) error {
	if !filepath.IsAbs(path) {
		return errors.New("sender: file path must be absolute")
	}
	if start.Input != "" && (start.Kind != "file" || start.Input != path || start.Offset < 0) {
		return errors.New("sender: invalid file checkpoint")
	}
	var f *os.File
	var lines *lineReader
	var base int64
	var identity Position
	pending := start.Inode != 0 || start.Device != 0
	emit := func(row ingest.Record, offset int64) error {
		p := identity
		p.Offset = offset
		p.Fingerprint, _ = fingerprintAt(f, offset)
		select {
		case out <- Item{Record: row, Position: p}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	closeCurrent := func() {
		if f != nil {
			f.Close()
			f = nil
			lines = nil
		}
	}
	defer closeCurrent()
	poll := func() error {
		select {
		case <-time.After(250 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return nil // let the loop flush a partial record before exiting
		}
	}
	for {
		if ctx.Err() != nil {
			// On orderly cancellation, deliver a pending terminal record if the consumer still accepts it.
			if lines != nil && lines.partial() {
				p := identity
				p.Offset = base + lines.total
				p.Fingerprint, _ = fingerprintAt(f, p.Offset)
				select {
				case out <- Item{Record: lines.finish(), Position: p}:
				case <-time.After(time.Second):
				}
			}
			return ctx.Err()
		}
		if f == nil {
			configured, err := os.Stat(path)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			candidate := path
			gap := false
			if pending && (err != nil || !samePosition(configured, start)) {
				candidate = sibling(path, start)
				if candidate == "" && err == nil { // Upstream history vanished; mark the gap before reading replacement.
					gap = true
					candidate = path
				}
			}
			if candidate == "" || err != nil && candidate == path {
				if e := poll(); e != nil {
					return e
				}
				continue
			}
			opened, fi, e := openRegular(candidate)
			if errors.Is(e, os.ErrNotExist) {
				if e := poll(); e != nil {
					return e
				}
				continue
			}
			if e != nil {
				return e
			}
			offset := int64(0)
			if pending && matchesSaved(opened, fi, start) {
				offset = start.Offset
			} else if pending && candidate != path {
				opened.Close()
				continue // sibling changed after discovery
			} else if pending {
				gap = true
			}
			f = opened
			identity = Position{Kind: "file", Input: path, Device: device(fi), Inode: inode(fi)}
			if gap {
				if e := emit(ingest.Record{Line: "gap: previous file history unavailable"}, 0); e != nil {
					return e
				}
			}
			if _, e = f.Seek(offset, 0); e != nil {
				return e
			}
			lines = newLineReader(f)
			base = offset
			pending = false
		}
		row, ok, err := lines.read()
		if err != nil {
			return err
		}
		if ok {
			if err := emit(row, base+lines.total); err != nil {
				return err
			}
			continue
		}
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		current, statErr := os.Stat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		offset := base + lines.total
		changed := statErr == nil && !samePosition(current, identity)
		shrunk := fi.Size() < offset
		if changed || shrunk {
			if lines.partial() {
				if err := emit(lines.finish(), offset); err != nil {
					return err
				}
			}
			if shrunk && !changed {
				if err := emit(ingest.Record{Line: "gap: file was truncated"}, 0); err != nil {
					return err
				}
			}
			closeCurrent()
			continue
		}
		if err := poll(); err != nil {
			return err
		}
	}
}
