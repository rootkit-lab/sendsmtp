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

# Simple landing page for humans
cat > index.html <<'HTML'
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>SendSMTP APT repository</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 42rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.5; }
    code, pre { background: #f4f4f5; padding: 0.15em 0.4em; border-radius: 4px; }
    pre { padding: 1rem; overflow-x: auto; }
  </style>
</head>
<body>
  <h1>SendSMTP APT</h1>
  <p>Signed Debian package repository for <a href="https://github.com/rootkit-lab/sendsmtp">SendSMTP</a>.</p>
  <pre>curl -fsSL https://rootkit-lab.github.io/sendsmtp/pubkey.gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/sendsmtp.gpg

echo "deb [signed-by=/usr/share/keyrings/sendsmtp.gpg arch=amd64] \
  https://rootkit-lab.github.io/sendsmtp stable main" \
  | sudo tee /etc/apt/sources.list.d/sendsmtp.list

sudo apt update
sudo apt install sendsmtp</pre>
</body>
</html>
HTML

popd >/dev/null
echo "APT repo ready at $OUT"
