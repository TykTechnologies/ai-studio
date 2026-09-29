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
  A third-party library may still use upstream gorm for itself: TIB 1.8
  pulls in `TykTechnologies/storage` v1.5, whose Postgres driver is built on
  `gorm.io/gorm`. It never sees Studio's models, so `gorm-verify` allows
  importers listed in `ALLOWED_GORM_IMPORTERS` (by package prefix; never a
  Studio path). Standalone binaries then carry that gorm too (the Dashboard
  links it anyway).
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
   shows the change. Accept it only on purpose, with `make schema-golden`,
   and say why in the PR.

To see what changed upstream:
`https://github.com/go-gorm/gorm/compare/<old>...<new>` (and the same for
`go-gorm/postgres` and `go-gorm/sqlite`).

### Licence

gorm and its drivers are MIT licensed (Copyright (c) 2013-present Jinzhu).
Each module's licence file is kept in its directory.

## langchaingo: AI Studio's fork of langchaingo, in tree

`third_party/langchaingo` holds the langchaingo packages AI Studio uses
(listed in `langchaingo/PACKAGES`). AI Studio imports them as
`github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/...` and
never as `github.com/tmc/langchaingo/...`.

### Why

AI Studio runs a fork of langchaingo with fixes it depends on: multi-tool
streaming, no default `temperature: 0` for Anthropic (newer Claude models
reject it), OpenAI `reasoning_effort`, and an empty Anthropic `content` read
as a stop rather than an error. The fork used to be selected with
`replace github.com/tmc/langchaingo => github.com/lonelycode/langchaingo`,
but Go ignores a `replace` in a dependency. A program that imports
`pkg/studio`, such as the Tyk Dashboard, would have built with upstream
langchaingo v0.1.13: without the fixes, and without compiling at all,
because upstream's Chroma store needs a chroma-go API that no longer exists.
Under an in-tree path the fork is simply part of this module.

### Unlike gorm, this copy is edited in place

gorm is upstream, byte for byte. This is a fork: fix it here, in the same PR
as the code that needs the fix, and add a test next to it. Mark a change to
upstream behaviour with a `Tyk:` comment. `langchaingo/VERSION` records where
the copy came from; everything since is in this repository's history.

`scripts/langchaingo-import.sh` made the copy:

1. It downloads the module (checked against the Go checksum database).
2. It copies the non-test files of each package in `PACKAGES`, the files
   they `//go:embed`, and the `LICENSE`. For the packages in `TESTED` (the
   LLM clients Tyk patches, plus `llms`) it also copies the tests and their
   `testdata`. Other langchaingo tests are left out, because they need
   testcontainers and other modules that would land in every host's module
   graph.
3. It rewrites `"github.com/tmc/langchaingo/` to the in-tree path and runs
   gofmt.

Changes made after the import:

- `vectorstores/chroma` is `//go:build cgo`, like `data_session/chroma.go`:
  chroma-go's client loads a tokenizer and the ONNX runtime through cgo, and
  a host may build with `CGO_ENABLED=0`.
- `llms/anthropic/anthropicllm_test.go` uses `t.Setenv`: upstream's
  `os.Setenv` leaked a fake API key into `TestLLM`, which then called the
  live Anthropic API with it.

### Using another langchaingo package

Add it to `PACKAGES` and copy it the same way: import into a scratch
directory with the script, then copy that package across. The script fails
if the copy imports a langchaingo package that is not in `PACKAGES`.

### Taking upstream changes

Import the new upstream (or fork) version into a scratch directory and merge
the diff by hand, keeping the Tyk changes:

```
scripts/langchaingo-import.sh github.com/tmc/langchaingo@<version> /tmp/lcg
diff -ru /tmp/lcg third_party/langchaingo
```

### Checks (`make langchaingo-verify`, run in CI on every PR)

No package of the root, microgateway or enterprise module, in either
edition, may import `github.com/tmc/langchaingo`, and no `go.mod` may name
it. The copy's tests run with the rest of the root module.

### Licence

langchaingo is MIT licensed (Copyright (c) Travis Cline). The licence
file is kept in `langchaingo/LICENSE`.
