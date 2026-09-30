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
# Allowed: the enterprise binaries' main packages (main_enterprise.go, and
# examples/embed-host/main_enterprise.go: a main package cannot be imported,
# so no host's tidy reads it), and the microgateway module and tests/ tree,
# which no program embedding Studio imports.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if matches=$(git grep -n '"github\.com/TykTechnologies/ai-studio-enterprise' -- '*.go' \
  ':!main_enterprise.go' ':!examples/embed-host/main_enterprise.go' ':!microgateway/' ':!tests/' ':!third_party/'); then
  echo "These files import the private enterprise module; register the feature through a core hook," >&2
  echo "and put enterprise tests of core packages in enterprise/_coretests:" >&2
  echo "$matches" >&2
  exit 1
fi
echo "No public package imports the enterprise module."
