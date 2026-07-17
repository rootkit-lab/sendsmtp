#!/usr/bin/env bash
# Publish website/ onto the existing gh-pages APT tree without rebuilding packages.
# Usage: ./scripts/deploy-website.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SITE="$ROOT/website"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [[ ! -d "$SITE" ]]; then
  echo "error: missing $SITE" >&2
  exit 1
fi

git clone --branch gh-pages --single-branch "$(git -C "$ROOT" remote get-url origin)" "$TMP/ghp"
# Overlay docs (do not delete pool/ or dists/)
cp -a "$SITE"/. "$TMP/ghp/"
if [[ -f "$ROOT/build/apt/pubkey.gpg" ]]; then
  cp -f "$ROOT/build/apt/pubkey.gpg" "$TMP/ghp/pubkey.gpg"
  cp -f "$ROOT/build/apt/pubkey.asc" "$TMP/ghp/pubkey.asc" 2>/dev/null || true
fi
touch "$TMP/ghp/.nojekyll"

cd "$TMP/ghp"
git add -A
if git diff --cached --quiet; then
  echo "No website changes to publish."
  exit 0
fi
git -c user.email="apt@sendsmtp.dev" -c user.name="SendSMTP Docs" \
  commit -m "Update documentation site"
git push origin gh-pages
echo "Published https://rootkit-lab.github.io/sendsmtp/"
