package backup

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Busnes-app/ky-primitives/capsule"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
)

const dbPath = "data/kypulse.db"

// memberLimit is capsule.MaxFileBytes; tests lower it to split a small database.
var memberLimit = capsule.MaxFileBytes

// dbMembers seals a snapshot larger than one capsule member allows as ordered parts,
// data/kypulse.db.part-000 onwards; JoinDatabase reassembles them.
func dbMembers(raw []byte) []recoveryclient.File {
	if int64(len(raw)) <= memberLimit {
		return []recoveryclient.File{{Path: dbPath, Data: raw, Mode: 0600}}
	}
	var parts []recoveryclient.File
	for i := 0; len(raw) > 0; i++ {
		n := min(int64(len(raw)), memberLimit)
		parts = append(parts, recoveryclient.File{Path: fmt.Sprintf("%s.part-%03d", dbPath, i), Data: raw[:n], Mode: 0600})
		raw = raw[n:]
	}
	return parts
}

// warnNearLimit logs once a payload passes 80% of what a capsule can hold.
func warnNearLimit(files []recoveryclient.File) {
	var total int64
	for _, f := range files {
		total += int64(len(f.Data))
	}
	if total > capsule.MaxExpandedBytes*8/10 {
		slog.Warn("backup capsule is near its size limit; prune audit history or move to PostgreSQL with pg_dump",
			"bytes", total, "limit", capsule.MaxExpandedBytes)
	}
}

// JoinDatabase writes <dir>/data/kypulse.db from the ordered parts a capsule of a large
// database holds, removing them when remove is set. A capsule with a single member
// needs nothing and returns false.
func JoinDatabase(dir string, remove bool) (bool, error) {
	// Read the directory, not filepath.Glob: a destination such as /srv/[restore] is a
	// valid path but a glob pattern that matches nothing.
	entries, err := os.ReadDir(filepath.Join(dir, "data"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var parts []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "kypulse.db.part-") {
			parts = append(parts, filepath.Join(dir, "data", e.Name()))
		}
	}
	if len(parts) == 0 {
		return false, nil
	}
	sort.Strings(parts)
	for i, p := range parts {
		if filepath.Base(p) != fmt.Sprintf("kypulse.db.part-%03d", i) {
			return false, errors.New("backup: database parts are not consecutive")
		}
	}
	out, err := os.OpenFile(filepath.Join(dir, dbPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return false, fmt.Errorf("backup: join database parts: %w", err)
	}
	for _, p := range parts {
		if err = appendFile(out, p); err != nil {
			break
		}
	}
	if err = errors.Join(err, out.Close()); err != nil {
		return false, fmt.Errorf("backup: join database parts: %w", err)
	}
	if remove {
		for _, p := range parts {
			if err := os.Remove(p); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

func appendFile(out *os.File, path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(out, in)
	return err
}
