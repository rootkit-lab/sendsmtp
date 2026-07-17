#!/usr/bin/env bash
# Build a signed APT repository tree from .deb files.
#
# Usage:
#   ./scripts/apt-build-repo.sh <deb-glob-or-dir> <out-dir>
#
# Env:
#   APT_GPG_PRIVATE_KEY  — armored private key (required unless already imported)
#   APT_GPG_PASSPHRASE   — optional passphrase
#   APT_ORIGIN           — Release Origin (default: SendSMTP)
#   APT_SUITE            — suite/codename (default: stable)
set -euo pipefail

DEB_SRC="${1:?usage: $0 <deb-dir-or-glob> <out-dir>}"
OUT="${2:?usage: $0 <deb-dir-or-glob> <out-dir>}"
SUITE="${APT_SUITE:-stable}"
ORIGIN="${APT_ORIGIN:-SendSMTP}"
LABEL="${APT_LABEL:-SendSMTP}"

GNUPGHOME="${GNUPGHOME:-$(mktemp -d)}"
export GNUPGHOME
chmod 700 "$GNUPGHOME"
trap 'rm -rf "$GNUPGHOME"' EXIT

if [[ -n "${APT_GPG_PRIVATE_KEY:-}" ]]; then
  printenv APT_GPG_PRIVATE_KEY | gpg --batch --import
fi

KEY_ID="$(gpg --list-secret-keys --with-colons | awk -F: '/^fpr:/ {print $10; exit}')"
if [[ -z "$KEY_ID" ]]; then
  echo "error: no secret GPG key available (set APT_GPG_PRIVATE_KEY)" >&2
  exit 1
fi
echo "Signing with key $KEY_ID"

mkdir -p "$OUT/pool/main" "$OUT/dists/${SUITE}/main/binary-amd64"

# Collect debs (keep previous pool if OUT already has packages)
shopt -s nullglob
if [[ -d "$DEB_SRC" ]]; then
  cp -f "$DEB_SRC"/*.deb "$OUT/pool/main/" 2>/dev/null || true
else
  cp -f $DEB_SRC "$OUT/pool/main/"
fi

mapfile -t DEBS < <(find "$OUT/pool/main" -name '*.deb' | sort)
if [[ ${#DEBS[@]} -eq 0 ]]; then
  echo "error: no .deb files in $OUT/pool/main" >&2
  exit 1
fi
echo "Packages: ${#DEBS[@]}"

# Prefer versioned filenames for pool hygiene
for deb in "${DEBS[@]}"; do
  base="$(basename "$deb")"
  if [[ "$base" == "sendsmtp.deb" ]]; then
    ver="$(dpkg-deb -f "$deb" Version)"
    arch="$(dpkg-deb -f "$deb" Architecture)"
    named="$OUT/pool/main/sendsmtp_${ver}_${arch}.deb"
    if [[ "$deb" != "$named" ]]; then
      mv -f "$deb" "$named"
    fi
  fi
done

pushd "$OUT" >/dev/null
apt-ftparchive packages pool/main > "dists/${SUITE}/main/binary-amd64/Packages"
gzip -n -9 -k -f "dists/${SUITE}/main/binary-amd64/Packages"

apt-ftparchive \
  -o "APT::FTPArchive::Release::Origin=${ORIGIN}" \
  -o "APT::FTPArchive::Release::Label=${LABEL}" \
  -o "APT::FTPArchive::Release::Suite=${SUITE}" \
  -o "APT::FTPArchive::Release::Codename=${SUITE}" \
  -o "APT::FTPArchive::Release::Architectures=amd64" \
  -o "APT::FTPArchive::Release::Components=main" \
  -o "APT::FTPArchive::Release::Description=SendSMTP APT repository" \
  release "dists/${SUITE}" > "dists/${SUITE}/Release"

SIGN_OPTS=(--batch --yes --pinentry-mode loopback)
if [[ -n "${APT_GPG_PASSPHRASE:-}" ]]; then
  SIGN_OPTS+=(--passphrase-fd 0)
  gpg "${SIGN_OPTS[@]}" --default-key "$KEY_ID" -abs -o "dists/${SUITE}/Release.gpg" "dists/${SUITE}/Release" <<<"$APT_GPG_PASSPHRASE"
  gpg "${SIGN_OPTS[@]}" --default-key "$KEY_ID" --clearsign -o "dists/${SUITE}/InRelease" "dists/${SUITE}/Release" <<<"$APT_GPG_PASSPHRASE"
else
  gpg --batch --yes --default-key "$KEY_ID" -abs -o "dists/${SUITE}/Release.gpg" "dists/${SUITE}/Release"
  gpg --batch --yes --default-key "$KEY_ID" --clearsign -o "dists/${SUITE}/InRelease" "dists/${SUITE}/Release"
fi

gpg --batch --export --armor "$KEY_ID" > pubkey.asc
gpg --batch --export "$KEY_ID" > pubkey.gpg

# Human-facing docs site (keeps APT files at repo root)
SITE_SRC="${APT_SITE_SRC:-}"
if [[ -z "$SITE_SRC" ]]; then
  # scripts/apt-build-repo.sh → repo root/website
  SITE_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/website"
fi
if [[ -d "$SITE_SRC" ]]; then
  cp -a "$SITE_SRC"/. .
  # Prefer committed archive pubkey over site copies
  if [[ -f "${SITE_SRC}/../build/apt/pubkey.gpg" ]]; then
    cp -f "${SITE_SRC}/../build/apt/pubkey.gpg" pubkey.gpg
    cp -f "${SITE_SRC}/../build/apt/pubkey.asc" pubkey.asc 2>/dev/null || true
  fi
fi
touch .nojekyll

popd >/dev/null
echo "APT repo ready at $OUT"
