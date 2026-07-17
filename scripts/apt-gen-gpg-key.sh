#!/usr/bin/env bash
# Generate an APT-signing GPG key for SendSMTP and print setup instructions.
#
# Creates (in ./build/apt/):
#   pubkey.asc / pubkey.gpg  — commit these (or let CI export them)
#   private.asc              — DO NOT commit; add as GitHub secret APT_GPG_PRIVATE_KEY
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/build/apt"
mkdir -p "$OUT"
chmod 700 "$OUT"

GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
chmod 700 "$GNUPGHOME"
trap 'rm -rf "$GNUPGHOME"' EXIT

EMAIL="${APT_KEY_EMAIL:-apt@sendsmtp.dev}"
NAME="${APT_KEY_NAME:-SendSMTP APT Archive}"

cat >"$GNUPGHOME/keycfg" <<EOF
%no-protection
Key-Type: RSA
Key-Length: 4096
Key-Usage: sign
Name-Real: ${NAME}
Name-Email: ${EMAIL}
Expire-Date: 0
%commit
EOF

gpg --batch --generate-key "$GNUPGHOME/keycfg"
KEY_ID="$(gpg --list-secret-keys --with-colons | awk -F: '/^fpr:/ {print $10; exit}')"

gpg --batch --export --armor "$KEY_ID" >"$OUT/pubkey.asc"
gpg --batch --export "$KEY_ID" >"$OUT/pubkey.gpg"
gpg --batch --export-secret-keys --armor "$KEY_ID" >"$OUT/private.asc"
chmod 600 "$OUT/private.asc"

echo
echo "Generated key: $KEY_ID"
echo "Public key:  $OUT/pubkey.asc  (safe to commit)"
echo "Private key: $OUT/private.asc (secret — never commit)"
echo
echo "Add GitHub secret:"
echo "  gh secret set APT_GPG_PRIVATE_KEY < $OUT/private.asc"
echo
echo "Then enable Pages: Settings → Pages → Deploy from branch gh-pages / root"
