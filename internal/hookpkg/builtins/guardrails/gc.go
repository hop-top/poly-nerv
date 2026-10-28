package guardrails

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GCConfig governs the backup garbage collector. Zero values are
// invalid (caller must set them); see ADR §"Backup retention + GC"
// for defaults.
type GCConfig struct {
	// TTL: archives older than (now - TTL) are deleted. Hours-grained.
	TTL time.Duration

	// MaxPerProject: at most this many archives kept per project key
	// (newest first). 0 disables the count cap.
	MaxPerProject int

	// MaxTotalBytes: total size cap across all projects. 0 disables.
	MaxTotalBytes int64
}

// GCStats reports the work GC did. Tests inspect this; production
// callers may log it.
type GCStats struct {
	ArchivesScanned int
	DeletedTTL      int
	DeletedCount    int
	DeletedSize     int
	BytesFreed      int64
}

// archiveEntry is the internal record per archive while running GC.
type archiveEntry struct {
	path    string
	mtime   time.Time
	size    int64
	project string // project-key dir name (for the count cap)
}

// RunGC walks <stateDir>/backups/ and prunes archives per cfg. The
// algorithm runs in three phases (matches ADR pseudocode):
//
//  1. Per-archive: delete anything older than TTL.
//  2. Per-project: keep newest MaxPerProject; delete the rest.
//  3. Global: while total size > MaxTotalBytes, delete oldest.
//
// Each phase narrows the surviving set; later phases see the result
// of earlier phases (so a TTL-deleted archive is not double-counted).
//
// Returns stats and a wrapped error if any deletion failed; partial
// progress is preserved (best-effort).
func RunGC(stateDir string, cfg GCConfig) (GCStats, error) {
	stats := GCStats{}
	root := filepath.Join(stateDir, "backups")

	entries, err := scanArchives(root)
	if err != nil {
		return stats, err
	}
	stats.ArchivesScanned = len(entries)

	now := time.Now()

	// Phase 1: TTL.
	if cfg.TTL > 0 {
		var keep []archiveEntry
		for _, e := range entries {
			if now.Sub(e.mtime) > cfg.TTL {
				if err := os.Remove(e.path); err == nil {
					stats.DeletedTTL++
					stats.BytesFreed += e.size
				}
				continue
			}
			keep = append(keep, e)
		}
		entries = keep
	}

	// Phase 2: per-project count cap.
	if cfg.MaxPerProject > 0 {
		byProj := make(map[string][]archiveEntry)
		for _, e := range entries {
			byProj[e.project] = append(byProj[e.project], e)
		}
		var keep []archiveEntry
		for _, list := range byProj {
			sort.Slice(list, func(i, j int) bool {
				return list[i].mtime.After(list[j].mtime)
			})
			for i, e := range list {
				if i < cfg.MaxPerProject {
					keep = append(keep, e)
					continue
				}
				if err := os.Remove(e.path); err == nil {
					stats.DeletedCount++
					stats.BytesFreed += e.size
				}
			}
		}
		entries = keep
	}

	// Phase 3: global size cap.
	if cfg.MaxTotalBytes > 0 {
		var total int64
		for _, e := range entries {
			total += e.size
		}
		if total > cfg.MaxTotalBytes {
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].mtime.Before(entries[j].mtime)
			})
			for total > cfg.MaxTotalBytes && len(entries) > 0 {
				e := entries[0]
				entries = entries[1:]
				if err := os.Remove(e.path); err == nil {
					stats.DeletedSize++
					stats.BytesFreed += e.size
					total -= e.size
				}
			}
		}
	}

	return stats, nil
}

// scanArchives walks <root>/<project>/<archive>.tar.gz collecting
// metadata. Missing root → empty result, no error (first run).
func scanArchives(root string) ([]archiveEntry, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var out []archiveEntry
	projects, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, pe := range projects {
		if !pe.IsDir() {
			continue
		}
		projDir := filepath.Join(root, pe.Name())
		archives, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, ae := range archives {
			if ae.IsDir() {
				continue
			}
			ai, err := ae.Info()
			if err != nil {
				continue
			}
			out = append(out, archiveEntry{
				path:    filepath.Join(projDir, ae.Name()),
				mtime:   ai.ModTime(),
				size:    ai.Size(),
				project: pe.Name(),
			})
		}
	}
	return out, nil
}
