#!/usr/bin/env bash
# PostToolUse hook: best-effort format a file after Claude edits it.
# Reads the Claude Code hook JSON from stdin, extracts the file path, and formats by
# extension if the relevant tool is installed. Never fails the workflow (always exits 0).

set -euo pipefail

payload="$(cat)"
# Extract tool_input.file_path without requiring jq.
file="$(printf '%s' "$payload" | python3 -c 'import sys,json; d=json.load(sys.stdin); print((d.get("tool_input") or {}).get("file_path",""))' 2>/dev/null || true)"

[ -z "$file" ] && exit 0
[ -f "$file" ] || exit 0

case "$file" in
  *.go)
    command -v gofmt >/dev/null 2>&1 && gofmt -w "$file" || true
    command -v goimports >/dev/null 2>&1 && goimports -w "$file" || true
    ;;
  *.ts|*.tsx|*.js|*.jsx|*.json|*.css)
    if [ -f "$(dirname "$file")/../frontend/package.json" ] || printf '%s' "$file" | grep -q "/frontend/"; then
      command -v npx >/dev/null 2>&1 && npx --no-install prettier --write "$file" >/dev/null 2>&1 || true
    fi
    ;;
esac

exit 0
