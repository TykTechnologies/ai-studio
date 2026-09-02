#!/usr/bin/env bash
#
# Build, sign, push and index a Tyk AI Studio plugin in one step.
#
#   tools/publish-plugin.sh publish <plugin-name> [flags]
#   tools/publish-plugin.sh verify  <plugin-name>|all [--version X]
#
# Normally driven through the Makefile:
#
#   make plugin-publish NAME=llm-cache SIGN_KEY=~/keys/plugin-ci.key
#   make plugin-verify  NAME=llm-cache        # or NAME=all to audit everything
#
# The version comes from the plugin's own manifest.json and is used verbatim as
# the OCI tag, so the tag, the marketplace entry and the binary can no longer
# drift apart.  The digest written into the marketplace manifest is read back
# from the registry rather than copied by hand.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

ARTIFACT_TYPE="application/vnd.tyk.plugin.binary.v1"
LAYER_TYPE="application/vnd.tyk.plugin.layer.v1"

MARKETPLACE_CE_URL="${MARKETPLACE_CE_URL:-git@github.com:lonelycode/tyk-ai-studio-plugins.git}"
MARKETPLACE_INTERNAL_URL="${MARKETPLACE_INTERNAL_URL:-git@github.com:TykTechnologies/studio-internal-marketplace.git}"
MARKETPLACE_BRANCH="${MARKETPLACE_BRANCH:-main}"

# Defaults, all overridable from the environment or flags.
REG="${REG:-}"
VERSION="${VERSION:-}"
PLATFORMS="${PLATFORMS:-linux/amd64 darwin/arm64}"
SIGN_KEY="${SIGN_KEY:-}"
PUB_KEY="${PUB_KEY:-}"
MARKETPLACE_DIR="${MARKETPLACE_DIR:-}"
DRY_RUN="${DRY_RUN:-0}"
ASSUME_YES="${YES:-0}"
SKIP_SIGN="${SKIP_SIGN:-0}"
SKIP_UI="${SKIP_UI:-0}"
SKIP_MARKETPLACE="${SKIP_MARKETPLACE:-0}"
NO_PUSH="${NO_PUSH:-0}"
FORCE="${FORCE:-0}"
MARKETPLACE_ONLY="${MARKETPLACE_ONLY:-0}"
MIN_STUDIO_VERSION="${MIN_STUDIO_VERSION:-}"

info()  { printf '%s\n' "$*" >&2; }
step()  { printf '\n==> %s\n' "$*" >&2; }
warn()  { printf 'warning: %s\n' "$*" >&2; }
die()   { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,16p' "${BASH_SOURCE[0]}" | sed 's/^#\{0,1\} \{0,1\}//' >&2
  cat >&2 <<'EOF'

Flags (publish):
  -v, --version X       override the version from manifest.json
      --registry HOST   OCI registry host (default: the one in the last release)
      --platforms LIST  space or comma separated os/arch list
      --min-studio-version X
                        studio version floor for the entry (default: the
                        plugin's own compat.min_studio_version)
      --sign-key PATH   cosign private key (env: SIGN_KEY)
      --pub-key PATH    cosign public key for the read-back verify
      --skip-sign       build and push without signing (not for releases)
      --skip-ui         skip the npm UI build
      --no-marketplace  stop after signing; do not touch the marketplace repo
      --marketplace-only
                        skip build/push/sign and only (re)write the marketplace
                        entry, reading the digest back from the registry -
                        the repair path for an entry that went out wrong
      --no-push         write and commit the marketplace entry but do not push
      --force           overwrite a version directory that already exists
      --dry-run         print every step, build locally, push nothing
  -y, --yes             do not prompt for confirmation

Full guide: docs/site/docs/plugins-publishing.md
EOF
  exit 1
}

# --- argument parsing -------------------------------------------------------

CMD="publish"
case "${1:-}" in
  publish|verify) CMD="$1"; shift ;;
  -h|--help|"")   usage ;;
esac

PLUGIN="${1:-}"
[ -n "$PLUGIN" ] || usage
shift || true

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version)     VERSION="$2"; shift 2 ;;
    --registry)       REG="$2"; shift 2 ;;
    --platforms)      PLATFORMS="$2"; shift 2 ;;
    --sign-key)       SIGN_KEY="$2"; shift 2 ;;
    --pub-key)        PUB_KEY="$2"; shift 2 ;;
    --marketplace-dir) MARKETPLACE_DIR="$2"; shift 2 ;;
    --min-studio-version) MIN_STUDIO_VERSION="$2"; shift 2 ;;
    --skip-sign)      SKIP_SIGN=1; shift ;;
    --skip-ui)        SKIP_UI=1; shift ;;
    --no-marketplace) SKIP_MARKETPLACE=1; shift ;;
    --marketplace-only) MARKETPLACE_ONLY=1; SKIP_SIGN=1; shift ;;
    --no-push)        NO_PUSH=1; shift ;;
    --force)          FORCE=1; shift ;;
    --dry-run)        DRY_RUN=1; shift ;;
    -y|--yes)         ASSUME_YES=1; shift ;;
    -h|--help)        usage ;;
    *)                die "unknown flag: $1" ;;
  esac
done

PLATFORMS="$(printf '%s' "$PLATFORMS" | tr ',' ' ')"

run() {
  if [ "$DRY_RUN" = "1" ]; then
    printf '    [dry-run] %s\n' "$*" >&2
    return 0
  fi
  "$@"
}

require_tool() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is not installed. $2"
}

confirm() {
  [ "$ASSUME_YES" = "1" ] && return 0
  [ "$DRY_RUN" = "1" ] && return 0
  if [ ! -t 0 ]; then
    die "refusing to publish without confirmation in a non-interactive shell; pass YES=1 once you have read the plan above."
  fi
  printf '\nProceed? [y/N] ' >&2
  read -r reply
  case "$reply" in
    y|Y|yes|YES) return 0 ;;
    *) die "aborted" ;;
  esac
}

# --- plugin resolution ------------------------------------------------------

# Sets SRC_DIR, CHANNEL, GO_DIR, UI_DIR, SRC_MANIFEST, BUILD_TAGS,
# DEFAULT_REPO_PATH and the marketplace location for $PLUGIN.
resolve_plugin() {
  SRC_DIR=""
  for candidate in \
      "community/plugins/$PLUGIN:community" \
      "enterprise/plugins/$PLUGIN:enterprise" \
      "tyk-internal/plugins/$PLUGIN:internal" \
      "examples/plugins/studio/$PLUGIN:example" \
      "examples/plugins/gateway/$PLUGIN:example" \
      "examples/plugins/data-collectors/$PLUGIN:example"; do
    path="${candidate%%:*}"
    kind="${candidate##*:}"
    if [ -d "$REPO_ROOT/$path" ]; then
      SRC_DIR="$REPO_ROOT/$path"
      CHANNEL="$kind"
      break
    fi
  done
  [ -n "$SRC_DIR" ] || die "no plugin named '$PLUGIN' under community/plugins, enterprise/plugins, tyk-internal/plugins or examples/plugins"

  # The Go module sometimes lives in a server/ subdirectory.
  if [ -f "$SRC_DIR/go.mod" ]; then
    GO_DIR="$SRC_DIR"
  elif [ -f "$SRC_DIR/server/go.mod" ]; then
    GO_DIR="$SRC_DIR/server"
  else
    die "no go.mod found in $SRC_DIR or $SRC_DIR/server"
  fi

  UI_DIR=""
  for candidate in "$GO_DIR/ui" "$SRC_DIR/ui"; do
    if [ -f "$candidate/package.json" ]; then UI_DIR="$candidate"; break; fi
  done

  SRC_MANIFEST=""
  for candidate in "$SRC_DIR/manifest.json" "$GO_DIR/plugin.manifest.json" "$SRC_DIR/plugin.manifest.json"; do
    if [ -f "$candidate" ]; then SRC_MANIFEST="$candidate"; break; fi
  done
  [ -n "$SRC_MANIFEST" ] || die "no manifest.json (or plugin.manifest.json) found for $PLUGIN"

  BUILD_TAGS=""
  PUBLISHER="tyk-community"
  ENTERPRISE_ONLY=0
  DEFAULT_REPO_PATH="studio-plugins/$PLUGIN"
  case "$CHANNEL" in
    enterprise)
      BUILD_TAGS="enterprise"
      PUBLISHER="tyk"
      ENTERPRISE_ONLY=1
      ;;
    internal)
      PUBLISHER="tyk-internal"
      ENTERPRISE_ONLY=1
      DEFAULT_REPO_PATH="internal-studio-plugins/$PLUGIN"
      ;;
  esac

  if [ -z "$MARKETPLACE_DIR" ]; then
    if [ "$CHANNEL" = "internal" ]; then
      MARKETPLACE_DIR="$REPO_ROOT/tyk-internal-marketplace"
      MARKETPLACE_URL="$MARKETPLACE_INTERNAL_URL"
    else
      MARKETPLACE_DIR="$REPO_ROOT/tyk-ai-studio-plugins"
      MARKETPLACE_URL="$MARKETPLACE_CE_URL"
    fi
  else
    MARKETPLACE_URL=""
  fi
  MP_PLUGIN_DIR="$MARKETPLACE_DIR/plugins/$PLUGIN"
}

# Read the digest a reference currently resolves to, with a usable error when
# the tag is missing or the registry needs a login.
resolve_digest() {
  local ref="$1" descriptor
  if ! descriptor="$(oras manifest fetch --descriptor "$ref" 2>&1)"; then
    die "could not read $ref from the registry:
       $descriptor
       If the tag should exist, check you are logged in: oras login ${ref%%/*}"
  fi
  printf '%s' "$descriptor" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])' 2>/dev/null \
    || die "unexpected descriptor for $ref: $descriptor"
}

# Repairing an entry in place means source and destination can be the same
# file; cp treats that as an error.
copy_file() {
  [ "$1" = "$2" ] && return 0
  cp "$1" "$2"
}

json_field() {
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$1" "$2"
}

# Read a dotted path out of the plugin's manifest.json, e.g.
# compat.min_studio_version.
json_path() {
  python3 -c '
import json, sys
node = json.load(open(sys.argv[1]))
for part in sys.argv[2].split("."):
    if not isinstance(node, dict):
        node = ""
        break
    node = node.get(part, "")
print(node if isinstance(node, str) else "")
' "$1" "$2"
}

# Highest semver directory under $MP_PLUGIN_DIR, excluding $1 if given.
previous_version() {
  local exclude="${1:-}"
  [ -d "$MP_PLUGIN_DIR" ] || return 0
  python3 - "$MP_PLUGIN_DIR" "$exclude" <<'PY'
import os, re, sys
root, exclude = sys.argv[1], sys.argv[2]

def key(name):
    m = re.match(r"^(\d+)\.(\d+)\.(\d+)", name)
    return tuple(int(g) for g in m.groups()) if m else (0, 0, 0)

versions = [
    d for d in os.listdir(root)
    if d != exclude
    and os.path.isfile(os.path.join(root, d, "manifest.yaml"))
]
if versions:
    print(sorted(versions, key=key)[-1])
PY
}

# Read a scalar out of a marketplace manifest.yaml without a YAML dependency.
manifest_value() {
  python3 - "$1" "$2" <<'PY'
import re, sys
path, key = sys.argv[1], sys.argv[2]
section, _, leaf = key.rpartition(".")
in_section = not section
for line in open(path):
    if section:
        if re.match(r"^%s:\s*$" % re.escape(section), line):
            in_section = True
            continue
        if in_section and re.match(r"^[^\s#]", line):
            in_section = False
    if not in_section:
        continue
    m = re.match(r"^\s*%s:\s*(.*?)\s*$" % re.escape(leaf), line)
    if m and (bool(section) == line.startswith((" ", "\t"))):
        print(m.group(1).strip().strip('"').strip("'"))
        break
PY
}

# On a first publish there is no previous entry to copy the house URLs from,
# so borrow them from any existing entry in the same marketplace and swap in
# this plugin's name.  Keeps a new plugin consistent with its neighbours.
marketplace_defaults() {
  ICON_BASE=""; DOC_URL=""; REPO_URL=""; ISSUES_URL=""
  local sample
  sample="$(ls "$MARKETPLACE_DIR"/plugins/*/*/manifest.yaml 2>/dev/null | head -1)"
  [ -n "$sample" ] || return 0

  local sample_plugin icon doc
  sample_plugin="$(basename "$(dirname "$(dirname "$sample")")")"

  icon="$(manifest_value "$sample" icon)"
  case "$icon" in
    */plugins/*) ICON_BASE="${icon%%/plugins/*}" ;;
  esac

  doc="$(manifest_value "$sample" links.documentation)"
  case "$doc" in
    */"$sample_plugin") DOC_URL="${doc%/$sample_plugin}/$PLUGIN" ;;
  esac

  REPO_URL="$(manifest_value "$sample" links.repository)"
  ISSUES_URL="$(manifest_value "$sample" links.issues)"
}

# --- marketplace checkout ---------------------------------------------------

prepare_marketplace() {
  if [ ! -d "$MARKETPLACE_DIR/.git" ]; then
    [ -n "$MARKETPLACE_URL" ] || die "$MARKETPLACE_DIR is not a git checkout"
    step "Cloning the marketplace repo into $MARKETPLACE_DIR"
    run git clone "$MARKETPLACE_URL" "$MARKETPLACE_DIR"
    if [ "$DRY_RUN" = "1" ]; then
      return 0
    fi
  fi

  # Only tracked changes matter: a stray untracked file (an editor directory,
  # say) is no reason to block a release, but an uncommitted edit would end up
  # in our commit.
  if [ "$DRY_RUN" != "1" ] && ! git -C "$MARKETPLACE_DIR" diff --quiet HEAD --; then
    die "$MARKETPLACE_DIR has uncommitted changes to tracked files; commit or stash them first"
  fi

  step "Updating $MARKETPLACE_DIR ($MARKETPLACE_BRANCH)"
  # The index is built by CI from the manifests on main, so the entry has to be
  # written on top of the current main or the rebuild will race.
  run git -C "$MARKETPLACE_DIR" checkout "$MARKETPLACE_BRANCH"
  run git -C "$MARKETPLACE_DIR" pull --ff-only origin "$MARKETPLACE_BRANCH"
}

# --- publish ----------------------------------------------------------------

do_publish() {
  require_tool go "Install Go 1.26.6+."
  require_tool git ""
  require_tool python3 ""
  [ "$DRY_RUN" = "1" ] || require_tool oras "See microgateway/docs/oci-plugin-workflow.md."
  if [ "$SKIP_SIGN" != "1" ] && [ "$DRY_RUN" != "1" ]; then
    require_tool cosign "See microgateway/docs/oci-plugin-workflow.md."
  fi

  resolve_plugin

  MANIFEST_VERSION="$(json_field "$SRC_MANIFEST" version)"
  PLUGIN_ID="$(json_field "$SRC_MANIFEST" id)"
  if [ -z "$VERSION" ]; then
    VERSION="$MANIFEST_VERSION"
    [ -n "$VERSION" ] || die "$SRC_MANIFEST has no version field; pass --version"
  elif [ "$VERSION" != "$MANIFEST_VERSION" ]; then
    warn "--version $VERSION does not match $SRC_MANIFEST (version $MANIFEST_VERSION)."
    warn "The registry tag and the marketplace entry will say $VERSION."
  fi

  # The compatibility floor comes from the plugin's own manifest.json unless it
  # is given explicitly, so it tracks the code rather than the first release.
  if [ -z "$MIN_STUDIO_VERSION" ]; then
    MIN_STUDIO_VERSION="$(json_path "$SRC_MANIFEST" compat.min_studio_version)"
  fi

  PREV_VERSION="$(previous_version "$VERSION")"

  # Registry coordinates: prefer whatever the last release actually used, so a
  # plugin never silently moves repository between versions.
  PREV_MANIFEST=""
  if [ -n "$PREV_VERSION" ]; then
    PREV_MANIFEST="$MP_PLUGIN_DIR/$PREV_VERSION/manifest.yaml"
  fi
  # Repairing an entry: base it on the entry for this same version so the
  # curated wording survives and only the OCI coordinates are corrected.
  REPAIR_IN_PLACE=0
  if [ "$MARKETPLACE_ONLY" = "1" ] && [ -f "$MP_PLUGIN_DIR/$VERSION/manifest.yaml" ]; then
    PREV_VERSION="$VERSION"
    PREV_MANIFEST="$MP_PLUGIN_DIR/$VERSION/manifest.yaml"
    REPAIR_IN_PLACE=1
  fi

  REPO_PATH=""
  if [ -f "$PREV_MANIFEST" ]; then
    [ -n "$REG" ] || REG="$(manifest_value "$PREV_MANIFEST" oci.registry)"
    REPO_PATH="$(manifest_value "$PREV_MANIFEST" oci.repository)"
  fi
  if [ -z "$REPO_PATH" ] && [ -f "$SRC_DIR/Makefile" ]; then
    REPO_PATH="$(sed -n 's/^REGISTRY_PATH[[:space:]]*:*=[[:space:]]*//p' "$SRC_DIR/Makefile" | head -1)"
  fi
  [ -n "$REPO_PATH" ] || REPO_PATH="$DEFAULT_REPO_PATH"
  [ -n "$REG" ] || REG="docker.tyk.io"

  PREV_MIN_STUDIO=""
  if [ -f "$PREV_MANIFEST" ]; then
    PREV_MIN_STUDIO="$(manifest_value "$PREV_MANIFEST" requirements.min_studio_version)"
  fi
  if [ -n "$MIN_STUDIO_VERSION" ] && [ -n "$PREV_MIN_STUDIO" ] \
     && [ "$MIN_STUDIO_VERSION" != "$PREV_MIN_STUDIO" ]; then
    warn "min_studio_version moves from $PREV_MIN_STUDIO to $MIN_STUDIO_VERSION in this release."
  fi

  TARGET="$REG/$REPO_PATH"
  VERSION_DIR="$MP_PLUGIN_DIR/$VERSION"

  if [ "$MARKETPLACE_ONLY" = "1" ] && [ -z "$PUB_KEY" ] && [ -n "$SIGN_KEY" ]; then
    PUB_KEY="${SIGN_KEY%.key}.pub"
  fi
  if [ "$SKIP_SIGN" != "1" ]; then
    [ -n "$SIGN_KEY" ] || die "SIGN_KEY is not set. Pass SIGN_KEY=/path/to/plugin-ci.key, or --skip-sign for an unsigned test push."
    [ "$DRY_RUN" = "1" ] || [ -f "$SIGN_KEY" ] || die "signing key not found: $SIGN_KEY"
    if [ -z "$PUB_KEY" ]; then
      PUB_KEY="${SIGN_KEY%.key}.pub"
    fi
  fi

  if [ "$SKIP_MARKETPLACE" != "1" ]; then
    prepare_marketplace
    if [ -d "$VERSION_DIR" ] && [ "$FORCE" != "1" ]; then
      die "$VERSION_DIR already exists.
       Bump the version in $SRC_MANIFEST to cut a new release, or pass --force
       to overwrite the entry. To repair an entry for a version that is already
       in the registry, use --marketplace-only --force."
    fi
  fi

  local ui_display="none"
  [ -n "$UI_DIR" ] && ui_display="${UI_DIR#$REPO_ROOT/}"
  local sign_display="SKIPPED"
  [ "$SKIP_SIGN" != "1" ] && sign_display="$SIGN_KEY"
  local min_studio_display="$MIN_STUDIO_VERSION"
  [ -n "$min_studio_display" ] || min_studio_display="(carried forward)"
  local mp_display="SKIPPED"
  if [ "$SKIP_MARKETPLACE" != "1" ]; then
    if [ "$NO_PUSH" = "1" ]; then
      mp_display="${VERSION_DIR#$REPO_ROOT/} -> commit only, no push"
    else
      mp_display="${VERSION_DIR#$REPO_ROOT/} -> push to origin/$MARKETPLACE_BRANCH"
    fi
  fi

  cat >&2 <<EOF

Publish plan
------------
  plugin        $PLUGIN ($CHANNEL${BUILD_TAGS:+, build tag: $BUILD_TAGS})
  id            $PLUGIN_ID
  version       $VERSION${PREV_VERSION:+  (previous: $PREV_VERSION)}
  source        ${SRC_DIR#$REPO_ROOT/}
  go module     ${GO_DIR#$REPO_ROOT/}
  ui bundle     $ui_display
  platforms     $PLATFORMS
  min studio    $min_studio_display
  registry      $TARGET:$VERSION
  signing       $sign_display
  marketplace   $mp_display
EOF
  [ "$DRY_RUN" = "1" ] && info "" && info "  (dry run: nothing will be pushed)"
  confirm

  # --- build ---
  if [ "$MARKETPLACE_ONLY" = "1" ]; then
    step "Marketplace-only: reading $TARGET:$VERSION back from the registry"
  else
    if [ -n "$UI_DIR" ] && [ "$SKIP_UI" != "1" ]; then
      step "Building UI bundle in ${UI_DIR#$REPO_ROOT/}"
      if [ -f "$UI_DIR/package-lock.json" ]; then
        ( cd "$UI_DIR" && run npm ci )
      else
        ( cd "$UI_DIR" && run npm install )
      fi
      ( cd "$UI_DIR" && run npm run build )
    fi

    PLATFORM_TAGS=""
    for platform in $PLATFORMS; do
      goos="${platform%%/*}"
      goarch="${platform##*/}"
      binary="$PLUGIN-$goos-$goarch"
      step "Building $binary"
      if [ -n "$BUILD_TAGS" ]; then
        ( cd "$GO_DIR" && run env GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -tags "$BUILD_TAGS" -o "$SRC_DIR/$binary" . )
      else
        ( cd "$GO_DIR" && run env GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -o "$SRC_DIR/$binary" . )
      fi
      PLATFORM_TAGS="$PLATFORM_TAGS $VERSION-${goos}_${goarch}"
    done

    # --- push ---
    for platform in $PLATFORMS; do
      goos="${platform%%/*}"
      goarch="${platform##*/}"
      binary="$PLUGIN-$goos-$goarch"
      tag="$VERSION-${goos}_${goarch}"
      step "Pushing $TARGET:$tag ($platform)"
      # Per-platform tags carry the version so a half-finished release can never
      # leave the index pointing at a previous version's binary.
      ( cd "$SRC_DIR" && run oras push "$TARGET:$tag" \
          --artifact-type "$ARTIFACT_TYPE" \
          --artifact-platform "$platform" \
          "./$binary:$LAYER_TYPE" )
    done

    step "Creating multi-platform index $TARGET:$VERSION"
    run oras manifest index create "$TARGET:$VERSION" $PLATFORM_TAGS
  fi

  if [ "$DRY_RUN" = "1" ]; then
    DIGEST="sha256:<resolved-after-push>"
  else
    DIGEST="$(resolve_digest "$TARGET:$VERSION")"
    [ -n "$DIGEST" ] || die "could not resolve the index digest for $TARGET:$VERSION"
  fi
  info "    index digest: $DIGEST"

  # --- sign ---
  if [ "$SKIP_SIGN" = "1" ]; then
    if [ "$MARKETPLACE_ONLY" = "1" ]; then
      info "    leaving the existing signature untouched"
    else
      warn "signing skipped - this artifact will fail verification on any gateway with OCI_PLUGINS_REQUIRE_SIGNATURE=true"
    fi
  else
    step "Signing $TARGET@$DIGEST"
    # Sign by digest: signing by tag would sign whatever the tag resolves to at
    # that moment, which is the failure mode we are engineering out.
    run cosign sign --yes --key "$SIGN_KEY" "$TARGET@$DIGEST"
  fi

  # Always read the signature back when we have a public key, including in
  # marketplace-only mode - a repaired entry is only worth writing if the
  # artifact it points at actually verifies.
  if [ -n "$PUB_KEY" ] && { [ -f "$PUB_KEY" ] || [ "$DRY_RUN" = "1" ]; }; then
    step "Verifying the signature with $PUB_KEY"
    run cosign verify --key "$PUB_KEY" "$TARGET@$DIGEST" >/dev/null
    info "    signature verifies"
  elif [ "$SKIP_SIGN" != "1" ]; then
    warn "no public key at $PUB_KEY - skipping the read-back verify. Pass --pub-key to check the signature."
  fi

  # --- marketplace entry ---
  if [ "$SKIP_MARKETPLACE" = "1" ]; then
    step "Done (marketplace entry skipped)"
    print_summary
    return 0
  fi

  step "Writing marketplace entry ${VERSION_DIR#$REPO_ROOT/}"
  if [ "$DRY_RUN" = "1" ]; then
    info "    [dry-run] would generate manifest.yaml${PREV_VERSION:+ from $PREV_VERSION}, plus README.md, icon.svg and CHANGELOG.md"
  else
    mkdir -p "$VERSION_DIR"
    marketplace_defaults

    local platform_csv
    platform_csv="$(printf '%s' "$PLATFORMS" | tr ' ' ',')"
    python3 "$REPO_ROOT/tools/plugin_marketplace_manifest.py" \
      --plugin "$PLUGIN" \
      --source-manifest "$SRC_MANIFEST" \
      ${PREV_MANIFEST:+--prev "$PREV_MANIFEST"} \
      ${PREV_VERSION:+--prev-version "$PREV_VERSION"} \
      --version "$VERSION" \
      --registry "$REG" \
      --repository "$REPO_PATH" \
      --tag "$VERSION" \
      --digest "$DIGEST" \
      --platforms "$platform_csv" \
      --publisher "$PUBLISHER" \
      --min-studio-version "$MIN_STUDIO_VERSION" \
      --icon-base "$ICON_BASE" \
      --doc-url "$DOC_URL" \
      --repo-url "$REPO_URL" \
      --issues-url "$ISSUES_URL" \
      $([ "$REPAIR_IN_PLACE" = "1" ] && echo --keep-created) \
      $([ "$ENTERPRISE_ONLY" = "1" ] && echo --enterprise-only) \
      --out "$VERSION_DIR/manifest.yaml"

    # An in-place repair only corrects the OCI coordinates; the docs and assets
    # that shipped with that version stay exactly as they are.
    if [ "$REPAIR_IN_PLACE" = "1" ]; then
      info "    repair: manifest.yaml only, README/icon/CHANGELOG left as published"
    else
      # README tracks the source tree; the icon and changelog are curated in the
      # marketplace repo, so they carry forward from the previous version.
      if [ -f "$SRC_DIR/README.md" ]; then
        copy_file "$SRC_DIR/README.md" "$VERSION_DIR/README.md"
      elif [ -n "$PREV_VERSION" ] && [ -f "$MP_PLUGIN_DIR/$PREV_VERSION/README.md" ]; then
        copy_file "$MP_PLUGIN_DIR/$PREV_VERSION/README.md" "$VERSION_DIR/README.md"
      fi

      if [ -n "$PREV_VERSION" ] && [ -f "$MP_PLUGIN_DIR/$PREV_VERSION/icon.svg" ]; then
        copy_file "$MP_PLUGIN_DIR/$PREV_VERSION/icon.svg" "$VERSION_DIR/icon.svg"
      elif [ -f "$SRC_DIR/assets/icon.svg" ]; then
        copy_file "$SRC_DIR/assets/icon.svg" "$VERSION_DIR/icon.svg"
      else
        warn "no icon.svg found for $PLUGIN; the marketplace card will render without one"
      fi

      if [ -n "$PREV_VERSION" ] && [ -f "$MP_PLUGIN_DIR/$PREV_VERSION/CHANGELOG.md" ]; then
        copy_file "$MP_PLUGIN_DIR/$PREV_VERSION/CHANGELOG.md" "$VERSION_DIR/CHANGELOG.md"
        warn "CHANGELOG.md was carried forward from $PREV_VERSION - add the $VERSION entry before this ships."
      else
        printf '# Changelog\n\n## [%s] - %s\n\n### Added\n- Initial release\n' \
          "$VERSION" "$(date -u +%Y-%m-%d)" > "$VERSION_DIR/CHANGELOG.md"
      fi
    fi
  fi

  step "Committing the marketplace entry"
  run git -C "$MARKETPLACE_DIR" add "plugins/$PLUGIN/$VERSION"
  run git -C "$MARKETPLACE_DIR" commit -m "$PLUGIN $VERSION

Published to $TARGET:$VERSION
Digest: $DIGEST"

  if [ "$NO_PUSH" = "1" ]; then
    warn "not pushing: the index will not rebuild until you run 'git -C $MARKETPLACE_DIR push origin $MARKETPLACE_BRANCH'"
  else
    step "Pushing to origin/$MARKETPLACE_BRANCH (this triggers the index rebuild)"
    run git -C "$MARKETPLACE_DIR" push origin "$MARKETPLACE_BRANCH"
  fi

  print_summary
}

print_summary() {
  SIGNED_SUMMARY="yes ($SIGN_KEY)"
  if [ "$MARKETPLACE_ONLY" = "1" ]; then
    SIGNED_SUMMARY="unchanged (no signing in marketplace-only mode)"
  elif [ "$SKIP_SIGN" = "1" ]; then
    SIGNED_SUMMARY="no"
  fi
  cat >&2 <<EOF

Published
---------
  $TARGET:$VERSION
  $DIGEST
  platforms: $PLATFORMS
  signed:    $SIGNED_SUMMARY

Check it with:
  make plugin-verify NAME=$PLUGIN VERSION=$VERSION
EOF
}

# --- verify -----------------------------------------------------------------

# Walk every published entry in both marketplace checkouts.  Cheap, and the
# fastest way to see whether a release actually landed the way its entry claims.
verify_all() {
  local failures=0 checked=0
  for marketplace in "$REPO_ROOT/tyk-ai-studio-plugins" "$REPO_ROOT/tyk-internal-marketplace"; do
    [ -d "$marketplace/plugins" ] || continue
    for plugin_dir in "$marketplace"/plugins/*/; do
      [ -d "$plugin_dir" ] || continue
      local name
      name="$(basename "$plugin_dir")"
      for version_dir in "$plugin_dir"*/; do
        [ -f "$version_dir/manifest.yaml" ] || continue
        local version
        version="$(basename "$version_dir")"
        checked=$((checked + 1))
        printf '\n--- %s %s\n' "$name" "$version" >&2
        local output
        if output="$("$REPO_ROOT/tools/publish-plugin.sh" verify "$name" --version "$version" 2>&1)"; then
          :
        else
          failures=$((failures + 1))
        fi
        printf '%s\n' "$output" | grep -vE '^Verifying |^$' >&2 || true
      done
    done
  done
  info ""
  info "$checked entries checked, $failures with problems"
  [ "$failures" -eq 0 ] || exit 1
}

do_verify() {
  require_tool oras ""
  require_tool python3 ""

  if [ "$PLUGIN" = "all" ]; then
    verify_all
    return 0
  fi

  resolve_plugin

  if [ -z "$VERSION" ]; then
    VERSION="$(previous_version "")"
    [ -n "$VERSION" ] || die "no published versions found under $MP_PLUGIN_DIR"
  fi

  local manifest="$MP_PLUGIN_DIR/$VERSION/manifest.yaml"
  [ -f "$manifest" ] || die "no marketplace entry at ${manifest#$REPO_ROOT/}"

  local reg repo tag want_digest
  reg="$(manifest_value "$manifest" oci.registry)"
  repo="$(manifest_value "$manifest" oci.repository)"
  tag="$(manifest_value "$manifest" oci.tag)"
  want_digest="$(manifest_value "$manifest" oci.digest)"
  TARGET="$reg/$repo"

  info "Verifying $PLUGIN $VERSION against $TARGET:$tag"
  local failures=0

  if [ "$tag" != "$VERSION" ]; then
    warn "FAIL  manifest version is $VERSION but oci.tag is '$tag' - the entry and the registry disagree"
    failures=$((failures + 1))
  else
    info "  ok  tag matches the manifest version"
  fi

  local got_digest
  got_digest="$(oras manifest fetch --descriptor "$TARGET:$tag" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])' 2>/dev/null || true)"
  if [ -z "$got_digest" ]; then
    warn "FAIL  $TARGET:$tag does not resolve in the registry (missing, or you are not logged in)"
    failures=$((failures + 1))
  elif [ "$got_digest" != "$want_digest" ]; then
    warn "FAIL  digest drift: the entry records $want_digest but the tag now resolves to $got_digest"
    failures=$((failures + 1))
  else
    info "  ok  digest matches: $got_digest"
  fi

  if [ -n "$got_digest" ]; then
    local platforms
    platforms="$(oras manifest fetch "$TARGET@$got_digest" 2>/dev/null | python3 -c '
import json, sys
doc = json.load(sys.stdin)
for m in doc.get("manifests", []):
    p = m.get("platform") or {}
    if p.get("os"):
        print("{}/{}".format(p["os"], p["architecture"]))
' 2>/dev/null || true)"
    if [ -z "$platforms" ]; then
      warn "FAIL  $TARGET@$got_digest is not a multi-platform index (no platform manifests)"
      failures=$((failures + 1))
    else
      info "  ok  index advertises: $(printf '%s' "$platforms" | tr '\n' ' ')"
    fi
  fi

  if [ -z "$PUB_KEY" ] && [ -n "$SIGN_KEY" ]; then
    PUB_KEY="${SIGN_KEY%.key}.pub"
  fi
  if [ -n "$PUB_KEY" ] && [ -f "$PUB_KEY" ] && [ -n "$got_digest" ]; then
    if cosign verify --key "$PUB_KEY" "$TARGET@$got_digest" >/dev/null 2>&1; then
      info "  ok  signature verifies against $PUB_KEY"
    else
      warn "FAIL  no valid signature for $TARGET@$got_digest under $PUB_KEY"
      failures=$((failures + 1))
    fi
  else
    warn "skip  signature check (pass PUB_KEY=/path/to/plugin-ci.pub)"
  fi

  if [ "$failures" -gt 0 ]; then
    die "$failures check(s) failed for $PLUGIN $VERSION"
  fi
  info ""
  info "All checks passed for $PLUGIN $VERSION"
}

case "$CMD" in
  publish) do_publish ;;
  verify)  do_verify ;;
esac
