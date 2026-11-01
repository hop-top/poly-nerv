package internal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// readFileState reads path, reporting whether it exists.
func readFileState(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// plannedFile is one file's pre-state and desired post-state.
type plannedFile struct {
	path    string
	before  []byte
	existed bool
	after   []byte // nil: remove
}

func (p plannedFile) changed() bool {
	if p.after == nil {
		return p.existed
	}
	return !p.existed || !bytes.Equal(p.before, p.after)
}

// planFiles asks the adapter for its desired file set and orders it:
// ConfigPaths first, remaining paths sorted.
func planFiles(mf MultiFileAdapter) ([]plannedFile, error) {
	read := func(path string) ([]byte, bool) { return readFileState(path) }
	desired, err := mf.MergeFiles(read)
	if err != nil {
		return nil, fmt.Errorf("merge files: %w", err)
	}

	var order []string
	seen := map[string]bool{}
	for _, p := range mf.ConfigPaths() {
		if _, ok := desired[p]; ok && !seen[p] {
			order = append(order, p)
			seen[p] = true
		}
	}
	var rest []string
	for p := range desired {
		if !seen[p] {
			rest = append(rest, p)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	if len(order) == 0 {
		return nil, errors.New("adapter returned no files")
	}

	plan := make([]plannedFile, 0, len(order))
	for _, p := range order {
		before, existed := readFileState(p)
		plan = append(plan, plannedFile{
			path: p, before: before, existed: existed, after: desired[p],
		})
	}
	return plan, nil
}

// installMulti is Install for MultiFileAdapter: every changed file is
// backed up, written atomically and hash-verified; any failure rolls
// all touched files back. A run that changes nothing writes no journal
// record, so Uninstall still restores the pre-nerv state.
func (inst *Installer) installMulti(
	mf MultiFileAdapter, dr DetectResult, dryRun bool,
) error {
	plan, err := planFiles(mf)
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}

	unlock, err := acquireLock(inst.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer unlock()

	stamp := time.Now().UTC().Format("20060102T150405Z")
	records := make([]FileRecord, len(plan))
	var changed bool
	for i, pf := range plan {
		rec := FileRecord{
			Path:    pf.path,
			Existed: pf.existed,
			Removed: pf.after == nil,
			PreHash: hashBytes(pf.before),
		}
		if pf.after != nil {
			rec.PostHash = hashBytes(pf.after)
		}
		if pf.changed() {
			changed = true
			if pf.existed {
				bp, err := inst.backupFile(string(mf.CLI()), stamp, i, pf.before)
				if err != nil {
					return fmt.Errorf("backup %s: %w", pf.path, err)
				}
				rec.BackupPath = bp
			}
		}
		records[i] = rec
	}
	if !changed {
		return nil
	}

	var done []plannedFile
	rollback := func(cause error) error {
		for i := len(done) - 1; i >= 0; i-- {
			pf := done[i]
			var rbErr error
			if pf.existed {
				rbErr = atomicWrite(pf.path, pf.before)
			} else if err := os.Remove(pf.path); err != nil && !os.IsNotExist(err) {
				rbErr = err
			}
			if rbErr != nil {
				return fmt.Errorf("%w AND rollback of %s failed: %v", cause, pf.path, rbErr)
			}
		}
		return fmt.Errorf("%w; rolled back", cause)
	}

	for _, pf := range plan {
		if !pf.changed() {
			continue
		}
		if pf.after == nil {
			if err := os.Remove(pf.path); err != nil {
				return rollback(fmt.Errorf("remove %s: %w", pf.path, err))
			}
			done = append(done, pf)
			continue
		}
		if err := atomicWrite(pf.path, pf.after); err != nil {
			return rollback(fmt.Errorf("write %s: %w", pf.path, err))
		}
		done = append(done, pf)
		written, err := os.ReadFile(pf.path)
		if err != nil {
			return rollback(fmt.Errorf("verify read %s: %w", pf.path, err))
		}
		if hashBytes(written) != hashBytes(pf.after) {
			return rollback(fmt.Errorf("hash mismatch after write of %s", pf.path))
		}
	}

	rec := InstallRecord{
		CLI:        string(mf.CLI()),
		Version:    dr.Info.Version,
		Timestamp:  time.Now().UTC(),
		PreHash:    records[0].PreHash,
		PostHash:   records[0].PostHash,
		BackupPath: records[0].BackupPath,
		Files:      records,
	}
	if err := inst.appendJournal(rec); err != nil {
		return rollback(fmt.Errorf("journal: %w", err))
	}
	return nil
}

// backupFile stores the pre-install content of one file of a
// multi-file install.
func (inst *Installer) backupFile(cli, stamp string, idx int, data []byte) (string, error) {
	dir := filepath.Join(inst.backupDir, cli)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.%d.bak", stamp, idx))
	return path, os.WriteFile(path, data, 0o640)
}

// restoreFiles reverses one multi-file install record.
func restoreFiles(files []FileRecord) error {
	for _, f := range files {
		if !f.Existed {
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", f.Path, err)
			}
			continue
		}
		if f.BackupPath == "" {
			continue // existed and unchanged by that install
		}
		backup, err := os.ReadFile(f.BackupPath)
		if err != nil {
			return fmt.Errorf("read backup %s: %w", f.BackupPath, err)
		}
		if err := atomicWrite(f.Path, backup); err != nil {
			return fmt.Errorf("restore %s: %w", f.Path, err)
		}
	}
	return nil
}

// filesDrifted reports whether any file differs from its recorded
// post-install state.
func filesDrifted(files []FileRecord) (bool, error) {
	for _, f := range files {
		data, exists := readFileState(f.Path)
		if f.Removed {
			if exists {
				return true, nil
			}
			continue
		}
		if !exists || hashBytes(data) != f.PostHash {
			return true, nil
		}
	}
	return false, nil
}
