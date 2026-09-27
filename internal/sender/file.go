package sender

import (
	"context"
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

func sibling(path string, p Position) string {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		candidate := filepath.Join(filepath.Dir(path), entry.Name())
		fi, err := os.Stat(candidate)
		if err == nil && fi.Mode().IsRegular() && samePosition(fi, p) {
			return candidate
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
			if pending && (err != nil || !samePosition(configured, start)) {
				candidate = sibling(path, start)
				if candidate == "" && err == nil { // Upstream history vanished; mark the gap before reading replacement.
					identity = Position{Kind: "file", Input: path, Device: device(configured), Inode: inode(configured)}
					if e := emit(ingest.Record{Line: "gap: previous file history unavailable"}, 0); e != nil {
						return e
					}
					pending = false
					candidate = path
				}
			}
			if candidate == "" || err != nil && candidate == path {
				if e := poll(); e != nil {
					return e
				}
				continue
			}
			opened, e := os.Open(candidate)
			if errors.Is(e, os.ErrNotExist) {
				if e := poll(); e != nil {
					return e
				}
				continue
			}
			if e != nil {
				return e
			}
			fi, e := opened.Stat()
			if e != nil {
				opened.Close()
				return e
			}
			if !fi.Mode().IsRegular() {
				opened.Close()
				return fmt.Errorf("sender: file input is not regular: %s", candidate)
			}
			offset := int64(0)
			if pending && samePosition(fi, start) {
				offset = start.Offset
				if fi.Size() < offset {
					offset = 0
				}
			}
			if _, e = opened.Seek(offset, 0); e != nil {
				opened.Close()
				return e
			}
			f = opened
			lines = newLineReader(f)
			base = offset
			identity = Position{Kind: "file", Input: path, Device: device(fi), Inode: inode(fi)}
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
				if err := emit(ingest.Record{Line: "gap: file was truncated"}, offset); err != nil {
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
