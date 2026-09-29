# third_party

## gorm.io: AI Studio's own copy of gorm

`third_party/gorm.io` is a copy of these modules, at the versions pinned in
[`gorm-pin/go.mod`](gorm-pin/go.mod) and listed in `gorm.io/VERSIONS`:

- `gorm.io/gorm`
- `gorm.io/driver/postgres`
- `gorm.io/driver/sqlite`

AI Studio imports gorm from here
(`github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm`, and so on)
and never from `gorm.io`.

### Why

The Tyk Dashboard embeds AI Studio (`pkg/studio`). It `replace`s
`gorm.io/gorm` with a Tyk fork, and a `replace` applies to the whole build.
Under its own import path, AI Studio's gorm is out of that replace's reach.
Each program keeps the gorm it was tested with. A binary that holds both
carries two copies of gorm, about 2–3 MB.

The database drivers underneath (`jackc/pgx`, `mattn/go-sqlite3`) and gorm's
small dependencies (`jinzhu/inflection`, `jinzhu/now`, `golang.org/x/text`)
are **not** copied. They are ordinary requirements of the root `go.mod`.
They must stay shared, because pgx and go-sqlite3 register `database/sql`
drivers by name, and a second copy would register the same name again and
panic. In a host's build, Go's version selection can move them up to the
host's versions (never down). That change shows in the host's go.mod.

### What the copy is, exactly

`scripts/gorm-vendor.sh` produces it, and the only way to change it is to
run that script:

1. Download each pinned module. `go mod download` checks it against
   `gorm-pin/go.sum` and the Go checksum database.
2. Copy the module's files, apart from `go.mod` and `go.sum`, to
   `gorm.io/<module path>`.
3. In `.go` files, rewrite `"gorm.io/` to
   `"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/`. That is
   the only change to upstream source.
4. Apply `third_party/patches/*.patch`, if any. The policy is to have none,
   so that a fix goes upstream and arrives with a release.

The directory is named `gorm.io` on purpose. gorm finds its own source
directory by that name, so it can report the caller's file and line in its
logs.

### Checks (`make gorm-verify`, run in CI on every PR)

- `third_party/gorm.io` is byte-for-byte what the pins produce. A hand edit,
  a partial update, or a pin changed without re-running the script fails.
- No package of the root, microgateway or enterprise module, in either
  edition, imports `gorm.io/...`. This matters more than it may seem. gorm
  finds model hooks (`BeforeSave`, `AfterFind`, ...), column types
  (`GormDBDataType`) and sentinel errors (`ErrRecordNotFound`) by type
  assertion at runtime. So a file that imports the other gorm still
  compiles, and then misbehaves: hooks that encrypt secrets never run,
  columns change type, and not-found checks fail.
- The copy's own unit tests pass.

These tests also guard the gorm behaviour AI Studio depends on:

- `models/schema_snapshot_test.go` and
  `microgateway/internal/database/schema_snapshot_test.go` compare the schema
  the migrations produce, on SQLite and Postgres, with golden files.
- `models/hooks_canary_test.go` ties every model hook to gorm's callback
  interfaces at compile time, and checks at runtime that the hooks fire.

### Upgrading gorm

No automation changes the copy. An upgrade is a deliberate PR:

1. Change the version(s) in `gorm-pin/go.mod` and run `go mod tidy` in
   `gorm-pin/`.
2. Run `scripts/gorm-vendor.sh` (`make gorm-vendor`).
3. Review the diff. It is the upstream change itself, apart from the import
   rewrite.
4. CI runs `gorm-verify`, the schema snapshots and the full suites. If a new
   gorm changes a column, an index or a constraint, the snapshot fails and
   shows the change.

To see what changed upstream:
`https://github.com/go-gorm/gorm/compare/<old>...<new>` (and the same for
`go-gorm/postgres` and `go-gorm/sqlite`).

### Licence

gorm and its drivers are MIT licensed (Copyright (c) 2013-present Jinzhu).
Each module's licence file is kept in its directory.
