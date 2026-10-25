#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root/web"
case "${1:-standard}" in
  standard) export VITE_LITE=false ;;
  lite) export VITE_LITE=true ;;
  *) echo 'Expected frontend build type: standard or lite' >&2; exit 1 ;;
esac

frontend_version=${OPENLIST_FRONTEND_VERSION:-}
if [[ -z "$frontend_version" ]]; then
  frontend_version="v$(node -p "require('./package.json').version")-$(git rev-parse --short HEAD)"
fi
if [[ ! "$frontend_version" =~ ^[a-zA-Z0-9._+-]+$ ]]; then
  echo "Invalid frontend version: $frontend_version" >&2
  exit 1
fi

# Locale sources are committed; no Crowdin credentials or upstream downloads.
node scripts/i18n.mjs
pnpm run build
printf '%s\n' "$frontend_version" > dist/VERSION
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  printf 'version=%s\n' "$frontend_version" >> "$GITHUB_OUTPUT"
fi
printf 'Built frontend %s (%s)\n' "$frontend_version" "${1:-standard}"
