#!/usr/bin/env python3
"""Generate a marketplace manifest.yaml entry for a newly published plugin version.

Three modes:

  --prev <manifest.yaml>
      Carry the previous version's curated entry forward, rewriting only the
      fields that change per release: version, OCI coordinates, the version
      segment of the icon URL, and the timestamps.  Comments and hand-written
      marketing copy survive untouched, so the diff between two versions stays
      as small as the one between 1.0.3 and 1.0.4.

  (no --prev)
      First publish: render a fresh entry from the plugin's own manifest.json
      and report the curated fields that still need a human.

  --add-fips --manifest <manifest.yaml> --tag T --digest D --platforms P
      Record the version's FIPS 140-3 variant (tools/publish-plugin.sh --fips)
      as an oci_fips block after oci. A later version's entry, carried forward
      with --prev, drops it.

Stdlib only, on purpose: this runs on developer laptops and in CI without a
pip install step.
"""

import argparse
import json
import os
import re
import sys
from datetime import datetime, timezone

OCI_KEYS = ("registry", "repository", "tag", "digest", "platform")


def yaml_str(value):
    """Quote a scalar for YAML output."""
    return '"{}"'.format(str(value).replace("\\", "\\\\").replace('"', '\\"'))


def yaml_list(values):
    """Render a flow-style list, matching the existing manifests."""
    return "[{}]".format(", ".join(yaml_str(v) for v in values))


def utc_now():
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def drop_fips_block(lines):
    """Remove an oci_fips block: it describes one version's FIPS variant
    (FIPS=1 adds it) and must not carry forward to the next version."""
    out = []
    skipping = False
    for line in lines:
        if re.match(r"^oci_fips:\s*$", line):
            skipping = True
            continue
        if skipping and (line.startswith((" ", "\t")) or not line.strip()):
            continue
        skipping = False
        out.append(line)
    return out


def add_fips(path, tag, digest, platforms):
    """Record a version's FIPS 140-3 variant in its entry as an oci_fips block
    (same registry and repository as oci), replacing any earlier one."""
    with open(path, encoding="utf-8") as fh:
        lines = drop_fips_block(fh.read().splitlines())
    block = [
        "",
        "oci_fips:",
        "  # FIPS 140-3 variant: built with GOFIPS140=v1.0.0 (Go Cryptographic",
        "  # Module v1.0.0, CMVP #5247); same registry and repository as oci.",
        "  tag: {}".format(yaml_str(tag)),
        "  digest: {}".format(yaml_str(digest)),
        "  platform: {}".format(yaml_list(platforms)),
    ]
    # After the oci block, which ends at the next top-level line.
    out, inserted, in_oci = [], False, False
    for line in lines:
        if not inserted and in_oci and re.match(r"^[^\s]", line):
            while out and not out[-1].strip():
                out.pop()
            out.extend(block)
            out.append("")
            inserted = True
            in_oci = False
        if re.match(r"^oci:\s*$", line):
            in_oci = True
        out.append(line)
    if not inserted:
        if not any(re.match(r"^oci:\s*$", line) for line in lines):
            raise SystemExit("error: {} has no oci block".format(path))
        out.extend(block)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(out).rstrip("\n") + "\n")


def rewrite_previous(text, args, prev_version, stamp):
    """Line-edit the previous version's manifest instead of re-serialising it."""
    lines = drop_fips_block(text.splitlines())
    out = []
    seen = set()
    in_oci = False
    in_requirements = False
    i = 0

    while i < len(lines):
        line = lines[i]
        i += 1

        if re.match(r"^oci:\s*$", line):
            in_oci = True
            out.append(line)
            continue

        if re.match(r"^requirements:\s*$", line):
            in_requirements = True
            out.append(line)
            continue

        # A non-indented, non-comment line closes whichever block we are in.
        if re.match(r"^[^\s#]", line):
            in_oci = False
            in_requirements = False

        if in_oci:
            match = re.match(r"^(\s+)(registry|repository|tag|digest|platform):", line)
            if match:
                indent, key = match.group(1), match.group(2)
                if key == "platform":
                    value = yaml_list(args.platforms)
                    # Swallow a block-style list, if the previous entry used one.
                    while i < len(lines) and re.match(r"^\s+-\s", lines[i]):
                        i += 1
                else:
                    value = yaml_str(getattr(args, key))
                out.append("{}{}: {}".format(indent, key, value))
                seen.add(key)
                continue
            out.append(line)
            continue

        # The plugin declares its own floor in manifest.json; keep the published
        # entry in step with it instead of letting it rot at whatever the first
        # release happened to say.
        if in_requirements and args.min_studio_version:
            match = re.match(r"^(\s+)min_studio_version:", line)
            if match:
                out.append("{}min_studio_version: {}".format(
                    match.group(1), yaml_str(args.min_studio_version)))
                seen.add("min_studio_version")
                continue

        if re.match(r"^version:", line):
            out.append("version: {}".format(yaml_str(args.version)))
            seen.add("version")
            continue

        if re.match(r"^icon:", line):
            # The icon is served out of the version directory we just created,
            # so rewrite the version segment of the path rather than trusting it
            # to match the previous version - several entries in the wild point
            # at an older version's icon.
            pattern = r"(plugins/{}/)[^/\"']+(/)".format(re.escape(args.plugin))
            new_line = re.sub(pattern, r"\g<1>{}\g<2>".format(args.version), line)
            if new_line == line and prev_version:
                new_line = line.replace(
                    "/{}/".format(prev_version), "/{}/".format(args.version)
                )
            out.append(new_line)
            continue

        if re.match(r"^(created_at|updated_at):", line):
            key = line.split(":", 1)[0]
            # Repairing an entry in place leaves created_at alone: the entry was
            # not created now, only corrected.
            if key == "created_at" and args.keep_created:
                out.append(line)
            else:
                out.append("{}: {}".format(key, yaml_str(stamp)))
            continue

        out.append(line)

    missing = [k for k in ("version",) + OCI_KEYS if k not in seen]
    if missing:
        raise SystemExit(
            "error: previous manifest {} is missing expected field(s): {}.\n"
            "       Its shape has drifted from the marketplace schema - fix it by "
            "hand before publishing.".format(args.prev, ", ".join(missing))
        )

    return "\n".join(out) + "\n"


TEMPLATE = """\
# Identity & Versioning
id: {id}
name: {name}
version: {version}
description: {description}

# OCI Distribution
oci:
  registry: {registry}
  repository: {repository}
  tag: {tag}
  digest: {digest}
  platform: {platform}

# Discovery & Classification
category: {category}
keywords: {keywords}
maturity: {maturity}

# Documentation & Support
links:
  documentation: {doc_url}
  repository: {repo_url}
  support: {support_url}
  issues: {issues_url}
  homepage: {homepage_url}

# Display Assets
icon: {icon}
screenshots: []

# Maintainers
maintainers:
  - name: "Tyk Technologies"
    email: "support@tyk.io"
    organization: "Tyk Technologies"

publisher: {publisher}
license: {license}

# Capabilities
capabilities:
  hooks: {hooks}
  primary_hook: {primary_hook}

# Requirements & Compatibility
requirements:
  min_studio_version: {min_studio_version}
  api_versions: []
  dependencies: []

# Security & Permissions
permissions:
  services: {perm_services}
  kv: []
  rpc: {perm_rpc}
  ui: {perm_ui}

# Configuration Schema
config_schema_url: ""

# Verification
attestation:
  enabled: false
  sigstore_bundle_url: ""

# Enterprise
enterprise_only: {enterprise_only}

# Metadata
created_at: {created_at}
updated_at: {updated_at}
deprecated: false
deprecated_message: ""
replacement: ""
"""


def render_template(source, args, stamp):
    """First publish: build an entry from the plugin's own manifest.json."""
    capabilities = source.get("capabilities", {}) or {}
    permissions = source.get("permissions", {}) or {}
    compat = source.get("compat", {}) or {}

    icon = ""
    if args.icon_base:
        icon = "{}/plugins/{}/{}/icon.svg".format(
            args.icon_base.rstrip("/"), args.plugin, args.version
        )

    return TEMPLATE.format(
        id=yaml_str(source.get("id", "")),
        name=yaml_str(source.get("name", args.plugin)),
        version=yaml_str(args.version),
        description=yaml_str(source.get("description", "")),
        registry=yaml_str(args.registry),
        repository=yaml_str(args.repository),
        tag=yaml_str(args.tag),
        digest=yaml_str(args.digest),
        platform=yaml_list(args.platforms),
        category=yaml_str(args.category),
        keywords=yaml_list([]),
        maturity=yaml_str("beta"),
        doc_url=yaml_str(args.doc_url),
        repo_url=yaml_str(args.repo_url),
        support_url=yaml_str("https://community.tyk.io"),
        issues_url=yaml_str(args.issues_url),
        homepage_url=yaml_str("https://tyk.io/ai-studio"),
        icon=yaml_str(icon),
        publisher=yaml_str(args.publisher),
        license=yaml_str(source.get("license", "AGPL v3")),
        hooks=yaml_list(capabilities.get("hooks", [])),
        primary_hook=yaml_str(capabilities.get("primary_hook", "")),
        min_studio_version=yaml_str(
            args.min_studio_version or compat.get("min_studio_version", "2.0")),
        perm_services=yaml_list(permissions.get("services", [])),
        perm_rpc=yaml_list(permissions.get("rpc", [])),
        perm_ui=yaml_list(permissions.get("ui", [])),
        enterprise_only="true" if args.enterprise_only else "false",
        created_at=yaml_str(stamp),
        updated_at=yaml_str(stamp),
    )


def main():
    if "--add-fips" in sys.argv[1:]:
        # tools/publish-plugin.sh --fips: add the FIPS variant to an entry.
        fips = argparse.ArgumentParser(description="Add a FIPS 140-3 variant (oci_fips) to a manifest.yaml")
        fips.add_argument("--add-fips", action="store_true")
        fips.add_argument("--manifest", required=True)
        fips.add_argument("--tag", required=True)
        fips.add_argument("--digest", required=True)
        fips.add_argument("--platforms", required=True, help="comma separated os/arch")
        fargs = fips.parse_args()
        add_fips(fargs.manifest, fargs.tag, fargs.digest,
                 [p.strip() for p in fargs.platforms.split(",") if p.strip()])
        print("added oci_fips ({}) to {}".format(fargs.tag, fargs.manifest), file=sys.stderr)
        return

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plugin", required=True)
    parser.add_argument("--source-manifest", required=True)
    parser.add_argument("--prev", default="", help="previous version's manifest.yaml")
    parser.add_argument("--prev-version", default="")
    parser.add_argument("--min-studio-version", default="",
                        help="requirements.min_studio_version to publish")
    parser.add_argument("--keep-created", action="store_true",
                        help="leave created_at untouched (in-place repair)")
    parser.add_argument("--version", required=True)
    parser.add_argument("--registry", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--digest", required=True)
    parser.add_argument("--platforms", required=True, help="comma separated os/arch")
    parser.add_argument("--out", required=True)
    # Only consulted on a first publish.
    parser.add_argument("--category", default="")
    parser.add_argument("--publisher", default="tyk-community")
    parser.add_argument("--enterprise-only", action="store_true")
    parser.add_argument("--icon-base", default="")
    parser.add_argument("--doc-url", default="")
    parser.add_argument("--repo-url", default="")
    parser.add_argument("--issues-url", default="")
    args = parser.parse_args()

    args.platforms = [p.strip() for p in args.platforms.split(",") if p.strip()]

    with open(args.source_manifest) as handle:
        source = json.load(handle)

    stamp = utc_now()

    if args.prev:
        with open(args.prev) as handle:
            content = rewrite_previous(handle.read(), args, args.prev_version, stamp)
        note = "carried forward from {}".format(args.prev_version or args.prev)
    else:
        content = render_template(source, args, stamp)
        note = "first publish - review the curated fields"

    os.makedirs(os.path.dirname(os.path.abspath(args.out)), exist_ok=True)
    with open(args.out, "w") as handle:
        handle.write(content)

    print("wrote {} ({})".format(args.out, note), file=sys.stderr)

    if not args.prev:
        print(
            "note: category, keywords, maturity, links and permissions in "
            "{} are placeholders - edit them before the index rebuilds.".format(args.out),
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
