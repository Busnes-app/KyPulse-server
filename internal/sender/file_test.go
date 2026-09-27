package sender

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nextFile(t *testing.T, ch <-chan Item) Item {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file record")
		return Item{}
	}
}

func followFile(t *testing.T, path string, start Position) (<-chan Item, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Item, 32)
	done := make(chan error, 1)
	go func() { done <- ReadFile(ctx, path, start, ch) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("file reader did not stop")
		}
	})
	return ch, cancel
}

func TestFileOffsetPartialAndOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("skip\npart"), 0600); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	ch, _ := followFile(t, path, Position{Kind: "file", Input: path, Device: device(fi), Inode: inode(fi), Offset: 5})
	select {
	case item := <-ch:
		t.Fatalf("premature record: %+v", item)
	case <-time.After(350 * time.Millisecond):
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString("ial\n" + strings.Repeat("x", 20<<10) + "\ngood\n"); err != nil {
		t.Fatal(err)
	}
	first := nextFile(t, ch)
	if first.Record.Line != "partial" || first.Position.Offset != 13 {
		t.Fatalf("first: %+v", first)
	}
	big := nextFile(t, ch)
	if !big.Record.Truncated || len(big.Record.Line) != 16<<10 {
		t.Fatalf("big: %+v", big.Record)
	}
	good := nextFile(t, ch)
	if good.Record.Line != "good" || good.Position.Offset != int64(13+(20<<10)+1+5) {
		t.Fatalf("good: %+v", good)
	}
}

func TestFileRenameReplacementAndResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("head\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	ch, _ := followFile(t, path, Position{})
	if got := nextFile(t, ch); got.Record.Line != "head" {
		t.Fatal(got)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if _, err := old.WriteString("old-tail\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new-head\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tail := nextFile(t, ch)
	head := nextFile(t, ch)
	if tail.Record.Line != "old-tail" || head.Record.Line != "new-head" || tail.Position.Inode == head.Position.Inode {
		t.Fatalf("rotation: %+v %+v", tail, head)
	}
	// A checkpoint after old-tail must find the renamed inode before the new file.
	resumed, _ := followFile(t, path, tail.Position)
	if got := nextFile(t, resumed); got.Record.Line != "new-head" {
		t.Fatalf("resume: %+v", got)
	}
}

func TestFileCopyTruncateMissingAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ch, _ := followFile(t, path, Position{})
	first := nextFile(t, ch)
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if gap := nextFile(t, ch); !strings.Contains(gap.Record.Line, "gap") {
		t.Fatalf("truncate marker: %+v", gap)
	}
	if err := os.WriteFile(path, []byte("next\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := nextFile(t, ch); got.Record.Line != "next" || got.Position.Offset != 5 {
		t.Fatalf("truncate: %+v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(350 * time.Millisecond)
	if err := os.WriteFile(path, []byte("fresh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := nextFile(t, ch)
	if got.Record.Line != "fresh" {
		t.Fatalf("missing/replaced: %+v", got)
	}
	// A saved offset from a different inode/device must never seek into the new file.
	resumed, _ := followFile(t, path, Position{Kind: "file", Input: path, Device: first.Position.Device + 1, Inode: first.Position.Inode, Offset: 100})
	gap := nextFile(t, resumed)
	fresh := nextFile(t, resumed)
	if !strings.Contains(gap.Record.Line, "gap") || fresh.Record.Line != "fresh" || fresh.Position.Offset != 6 {
		t.Fatalf("identity: %+v %+v", gap, fresh)
	}
}

func TestFileRotationFlushesPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("ready\nunfinished"), 0600); err != nil {
		t.Fatal(err)
	}
	ch, _ := followFile(t, path, Position{})
	if got := nextFile(t, ch); got.Record.Line != "ready" {
		t.Fatal(got)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("next\n"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, next := nextFile(t, ch), nextFile(t, ch)
	if partial.Record.Line != "unfinished" || partial.Position.Offset != 16 || next.Record.Line != "next" {
		t.Fatalf("rotation partial: %+v %+v", partial, next)
	}
}

func TestFileShutdownFlushesPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("ready\nlast"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Item, 2)
	done := make(chan error, 1)
	go func() { done <- ReadFile(ctx, path, Position{}, ch) }()
	if got := nextFile(t, ch); got.Record.Line != "ready" {
		t.Fatal(got)
	}
	time.Sleep(300 * time.Millisecond)
	cancel()
	if got := nextFile(t, ch); got.Record.Line != "last" || got.Position.Offset != 10 {
		t.Fatalf("shutdown: %+v", got)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reader did not stop")
	}
}
