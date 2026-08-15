#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

OUTDIR="${1:-signing}"
PASSFILE="$OUTDIR/.pass"

# This script's only stdout output is the signing password, so build.sh can
# capture it with $(...). Everything else goes to stderr.

mkdir -p "$OUTDIR"

# If a cert already exists, reuse its stored password so later builds can sign
# without re-typing it. Explicit -signpass / SIGN_PASSWORD wins otherwise.
if [ -f "$OUTDIR/signing.pfx" ] && [ -f "$OUTDIR/signing.crt" ]; then
  if [ -f "$PASSFILE" ]; then
    PASS="$(<"$PASSFILE")"
  elif [ -n "${2:-}" ]; then
    PASS="$2"
  else
    PASS="${SIGN_PASSWORD:-changeme}"
  fi
  echo "$PASS"
  exit 0
fi

# Fresh cert: use a random password unless one was supplied, and persist it
# next to the cert so builds stay reproducible.
PASS="${2:-}"
if [ -z "$PASS" ]; then
  PASS="${SIGN_PASSWORD:-$(openssl rand -base64 24)}"
fi
umask 077
printf '%s' "$PASS" > "$PASSFILE"

openssl req -x509 -newkey rsa:2048 -sha256 -days 1095 -nodes \
  -keyout "$OUTDIR/signing.key" \
  -out "$OUTDIR/signing.crt" \
  -subj "/CN=smol-tailscaler" \
  -addext "extendedKeyUsage=codeSigning" \
  -addext "keyUsage=digitalSignature" 2>/dev/null

openssl pkcs12 -export \
  -out "$OUTDIR/signing.pfx" \
  -inkey "$OUTDIR/signing.key" \
  -in "$OUTDIR/signing.crt" \
  -passout "pass:$PASS" 2>/dev/null

echo "$PASS"
echo "Created: $OUTDIR/signing.pfx (password stored in $PASSFILE)" >&2
echo "Import $OUTDIR/signing.crt into the VM (Trusted Publishers + Root)" >&2
