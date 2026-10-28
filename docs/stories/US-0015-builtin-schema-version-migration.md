# US-0015: Built-in schema version migration

**Persona:** Platform Engineer, OSS Go Developer
**Priority:** P2

## Story

As an OSS Go developer (built-in author) bumping a
built-in's config schema, I want the daemon to detect the
version mismatch and run my migration on user state — and
as a platform engineer, I want a visible warning if a
migration ran on engineer machines.

## Acceptance Criteria

- [ ] Given a built-in declares `SchemaVersion: 2` AND
      `CachedConfig.BuiltinVersions[name] == 1`, when daemon
      starts AND the built-in's `Migrator` is non-nil, then
      `Migrator.Migrate(fromVersion=1, stateDir)` is called
      BEFORE the built-in handles any event
- [ ] Given the migration succeeds, when daemon continues,
      then `BuiltinVersions[name]` is updated to 2 AND a log
      line records the migration with old → new version
- [ ] Given `Migrator` is nil AND version bumps, when daemon
      starts, then the version is updated silently (no
      migration code, no error)
- [ ] Given the migration FAILS (returns error), when daemon
      starts, then it logs the failure AND keeps
      `BuiltinVersions[name]` at the OLD version AND
      DISABLES the built-in for the session — fail-safe
- [ ] Given `nerv daemon status` is run, when migrations have
      occurred recently, then the output lists migrated
      built-ins with old → new versions for visibility

## Notes

- Migration touches only the built-in's own state dir
  (`$XDG_STATE_HOME/nerv/<name>/`); never user config
  (deprecation warnings instead).
- Per builtins-registry ADR §5.
