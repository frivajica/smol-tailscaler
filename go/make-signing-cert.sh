#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

OUTDIR="${1:-signing}"
PASS="${SIGN_PASSWORD:-changeme}"

if [ -f "$OUTDIR/signing.pfx" ] && [ -f "$OUTDIR/signing.crt" ]; then
  echo "$PASS"
  exit 0
fi

mkdir -p "$OUTDIR"

openssl req -x509 -newkey rsa:2048 -sha256 -days 1095 -nodes \
  -keyout "$OUTDIR/signing.key" \
  -out "$OUTDIR/signing.crt" \
  -subj "/CN=smol-tailscaler" \
  -addext "extendedKeyUsage=codeSigning" \
  -addext "keyUsage=digitalSignature"

openssl pkcs12 -export \
  -out "$OUTDIR/signing.pfx" \
  -inkey "$OUTDIR/signing.key" \
  -in "$OUTDIR/signing.crt" \
  -passout "pass:$PASS"

echo "$PASS"
echo "Created:"
echo "  $OUTDIR/signing.pfx  (password: $PASS) — pass to ./build.sh -signcert"
echo "  $OUTDIR/signing.crt                — import into the VM (Trusted Publishers + Root)"
echo "  $OUTDIR/signing.key"
