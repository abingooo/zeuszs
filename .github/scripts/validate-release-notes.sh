#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ -z "$version" ]]; then
  echo "Release version is required." >&2
  exit 1
fi

notes_path=".github/releases/${version}.md"
if [[ ! -s "$notes_path" ]]; then
  echo "Missing or empty release notes: $notes_path" >&2
  exit 1
fi

if ! grep -Eq '^[[:space:]]*[-*][[:space:]]+[^[:space:]]' "$notes_path"; then
  echo "Release notes must contain at least one non-empty change item: $notes_path" >&2
  exit 1
fi

echo "Validated release notes: $notes_path"
