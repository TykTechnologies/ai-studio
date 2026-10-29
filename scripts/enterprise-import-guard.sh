#!/usr/bin/env bash
# Checks the CE/EE boundary: no public package of this repository imports the
# private enterprise module (github.com/TykTechnologies/ai-studio-enterprise).
#
# `go mod tidy` in a module that imports Studio considers every build tag, so
# a single `//go:build enterprise` file importing the enterprise module from
# a package a host imports would make every Community Edition consumer try,
# and fail, to fetch the private repository. Enterprise features register
# themselves with core through hooks instead (see enterprise/all).
#
# That includes tests: tidy reads the tests of every package it imports too.
# Enterprise tests of core packages live in enterprise/_coretests and are
# linked in, gitignored, by scripts/enterprise-link-tests.sh.
#
# Nothing in the root module may import it, not even a main package or
# tests/: the root go.mod does not require the enterprise module at all, so
# that a plugin author's `go list -m all` (govulncheck, Dependabot) resolves.
# The enterprise registrations of the Studio binary, examples/embed-host and
# tests/ live in enterprise/_coretests as *_entlink.go and *_entlink_test.go,
# linked in like the tests, and enterprise builds resolve the module through
# enterprise.work. Only the microgateway module (its own go.mod) may import
# it.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if grep -n 'ai-studio-enterprise' go.mod >&2; then
  echo "The root go.mod must not mention the private enterprise module: plugin authors inherit its" >&2
  echo "requirements, and \`go list -m all\` then fails for them. Enterprise builds use enterprise.work." >&2
  exit 1
fi

if matches=$(git grep -n '"github\.com/TykTechnologies/ai-studio-enterprise' -- '*.go' \
  ':!microgateway/' ':!third_party/'); then
  echo "These files import the private enterprise module; register the feature through a core hook," >&2
  echo "and put enterprise tests and registrations of core packages in enterprise/_coretests:" >&2
  echo "$matches" >&2
  exit 1
fi
echo "No public package imports the enterprise module, and the root go.mod does not require it."
