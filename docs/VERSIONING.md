# Versioning

`VERSION` is the canonical Postilaatikko application version. New releases use
CalVer: `YYYY.MM.DD`. If a second release is needed on the same day, append a
numeric patch, for example `2026.10.29.1`. Historical SemVer releases remain
valid and are accepted by the version checker.

Before a future release, update `VERSION`, materialize the same value in the
application metadata, then run:

```bash
./scripts/version.sh check
./scripts/version.sh check vYYYY.MM.DD
```

The check compares `VERSION` with the Go runtime version, frontend
package and lock metadata, Wails product version, current AppStream release,
and the main Flatpak manifest tag. It does not inspect or change dependency,
runtime, database-schema, migration, or historical release-note versions.

Review the resulting diff before creating a tag. The helper intentionally has
no `sync` command: version fields are updated explicitly so release notes and
their date can be reviewed together.
