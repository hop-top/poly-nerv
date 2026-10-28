package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hop.top/kit/go/core/xdg"
)

// InstallRecord tracks one install event.
type InstallRecord struct {
	CLI        string    `json:"cli"`
	Version    string    `json:"version"`
	Timestamp  time.Time `json:"timestamp"`
	PreHash    string    `json:"pre_hash"`
	PostHash   string    `json:"post_hash"`
	BackupPath string    `json:"backup_path"`
}

// Installer handles transactional adapter install/uninstall.
type Installer struct {
	lockPath    string
	backupDir   string
	journalPath string
}

// NewInstaller creates an Installer with paths under xdg.StateDir("nerv").
func NewInstaller() *Installer {
	stateDir := xdg.MustEnsure(xdg.StateDir("nerv"))
	return &Installer{
		lockPath:    filepath.Join(stateDir, "install.lock"),
		backupDir:   filepath.Join(stateDir, "backups"),
		journalPath: filepath.Join(stateDir, "installs.jsonl"),
	}
}

// newInstallerAt creates an Installer rooted at dir (for testing).
func newInstallerAt(dir string) *Installer {
	return &Installer{
		lockPath:    filepath.Join(dir, "install.lock"),
		backupDir:   filepath.Join(dir, "backups"),
		journalPath: filepath.Join(dir, "installs.jsonl"),
	}
}

// Install performs transactional adapter installation.
//
// Steps:
//  1. Detect CLI version via adapter.Detect()
//  2. Read existing configs via adapter.ConfigPaths()
//  3. Compute merge via adapter.MergeConfig(existing)
//  4. If dryRun: return without writing
//  5. Acquire flock
//  6. Create timestamped backup
//  7. Atomic temp+rename write
//  8. Re-read + verify hash matches expected
//  9. On mismatch: rollback from backup
//  10. Record in journal
//  11. Release lock
func (inst *Installer) Install(adapter HookAdapter, dryRun bool) error {
	dr, err := adapter.Detect()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	if !dr.Found {
		return fmt.Errorf("cli %q not found", adapter.CLI())
	}

	paths := adapter.ConfigPaths()
	if len(paths) == 0 {
		return errors.New("adapter returned no config paths")
	}

	configPath := paths[0]
	existing, _ := os.ReadFile(configPath) // missing file → empty

	merged, err := adapter.MergeConfig(existing)
	if err != nil {
		return fmt.Errorf("merge config: %w", err)
	}

	if dryRun {
		return nil
	}

	// Acquire lock.
	unlock, err := acquireLock(inst.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer unlock()

	// Backup existing config.
	preHash := hashBytes(existing)
	backupPath, err := inst.backup(string(adapter.CLI()), existing)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	// Atomic write: temp + rename.
	if err := atomicWrite(configPath, merged); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Verify written content.
	written, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("verify read: %w", err)
	}
	postHash := hashBytes(written)
	expectedHash := hashBytes(merged)

	if postHash != expectedHash {
		// Rollback.
		if rbErr := atomicWrite(configPath, existing); rbErr != nil {
			return fmt.Errorf(
				"hash mismatch AND rollback failed: %w", rbErr,
			)
		}
		return fmt.Errorf(
			"hash mismatch after write (expected %s, got %s); rolled back",
			expectedHash, postHash,
		)
	}

	// Record in journal.
	rec := InstallRecord{
		CLI:        string(adapter.CLI()),
		Version:    dr.Info.Version,
		Timestamp:  time.Now().UTC(),
		PreHash:    preHash,
		PostHash:   postHash,
		BackupPath: backupPath,
	}
	if err := inst.appendJournal(rec); err != nil {
		return fmt.Errorf("journal: %w", err)
	}

	return nil
}

// Uninstall reverses install using backup path from journal.
func (inst *Installer) Uninstall(adapter HookAdapter) error {
	records, err := inst.Journal(string(adapter.CLI()))
	if err != nil {
		return fmt.Errorf("read journal: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no install records for %q", adapter.CLI())
	}

	// Use most recent backup.
	last := records[len(records)-1]
	backup, err := os.ReadFile(last.BackupPath)
	if err != nil {
		return fmt.Errorf("read backup %s: %w", last.BackupPath, err)
	}

	paths := adapter.ConfigPaths()
	if len(paths) == 0 {
		return errors.New("adapter returned no config paths")
	}

	return atomicWrite(paths[0], backup)
}

// DetectDrift checks if current config hash matches recorded post-hash.
func (inst *Installer) DetectDrift(adapter HookAdapter) (bool, error) {
	records, err := inst.Journal(string(adapter.CLI()))
	if err != nil {
		return false, fmt.Errorf("read journal: %w", err)
	}
	if len(records) == 0 {
		return false, nil
	}

	last := records[len(records)-1]
	paths := adapter.ConfigPaths()
	if len(paths) == 0 {
		return false, errors.New("adapter returned no config paths")
	}

	current, err := os.ReadFile(paths[0])
	if err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}

	return hashBytes(current) != last.PostHash, nil
}

// Journal returns all install records for a CLI.
func (inst *Installer) Journal(cli string) ([]InstallRecord, error) {
	data, err := os.ReadFile(inst.journalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var records []InstallRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec InstallRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // skip corrupt lines
		}
		if rec.CLI == cli {
			records = append(records, rec)
		}
	}
	return records, nil
}

// backup writes existing config to backupDir/<cli>/<timestamp>.json.
func (inst *Installer) backup(cli string, data []byte) (string, error) {
	dir := filepath.Join(inst.backupDir, cli)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102T150405Z") + ".json"
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, data, 0o640)
}

// appendJournal appends a record to the JSONL journal file.
func (inst *Installer) appendJournal(rec InstallRecord) (err error) {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(
		inst.journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640,
	)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = f.Write(append(data, '\n'))
	return err
}

// hashBytes returns the hex-encoded SHA-256 of data.
func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// atomicWrite writes data to path via temp file + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".nerv-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()        // best-effort cleanup; the write error is returned
		_ = os.Remove(tmpName) // best-effort cleanup
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) // best-effort cleanup; the close error is returned
		return err
	}
	return os.Rename(tmpName, path)
}

// acquireLock creates a lock file. Returns an unlock function.
// Uses mkdir-based locking (portable, no syscall.Flock needed).
func acquireLock(path string) (func(), error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	// Use the lock path as a directory for mkdir atomicity.
	lockDir := path + ".d"
	if err := os.Mkdir(lockDir, 0o750); err != nil {
		return nil, fmt.Errorf("lock held or cannot acquire: %w", err)
	}
	return func() { _ = os.Remove(lockDir) }, nil // unlock is best-effort; a stale dir is reported on next acquire
}
