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
# Allowed: tests, and the enterprise binaries' own main packages, which no
# other module imports.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if matches=$(git grep -n '"github\.com/TykTechnologies/ai-studio-enterprise' -- '*.go' \
  ':!*_test.go' ':!main_enterprise.go' ':!microgateway/cmd/microgateway/main_enterprise.go' ':!third_party/'); then
  echo "These public files import the private enterprise module; register the feature through a core hook instead:" >&2
  echo "$matches" >&2
  exit 1
fi
echo "No public package imports the enterprise module."
