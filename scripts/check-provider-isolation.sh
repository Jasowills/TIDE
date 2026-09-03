#!/bin/sh
# QA §3.5 / Grilling §5.2: provider isolation enforced by lint from commit #1.
# Fails if CORE code branches on a provider name. What counts:
#   1. string literals "traccar"/"flespi" (any quote/case) outside /adapters/**
#   2. comparisons of a provider value against a literal: provider == "..."
# Struct field names (id.Provider) and test fixtures are NOT violations:
# tests (*_test.go) are excluded; field accesses have no string literal.
set -eu
violations=""
lit=$(grep -rnEi --include='*.go' -e '"traccar' -e '"flespi' -e "'traccar" -e "'flespi" \
  . | grep -v '^\./adapters/' | grep -v '_test\.go' || true)
if [ -n "$lit" ]; then
  violations="${violations}${lit}\n"
fi
cmp=$(grep -rnE --include='*.go' \
  -e 'provider\s*==\s*"' -e '"\s*==\s*[^"]*provider' \
  . | grep -v '^\./adapters/' | grep -v '_test\.go' || true)
if [ -n "$cmp" ]; then
  violations="${violations}${cmp}\n"
fi
if [ -n "$violations" ]; then
  printf 'provider-isolation violations (core must not name providers):\n%b' "$violations"
  exit 1
fi
echo "provider-isolation: ok"
