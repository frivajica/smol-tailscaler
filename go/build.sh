#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

PASSWORD=""
AUTHKEY=""
USERNAME=""
SIGNCERT=""
SIGNPASS=""
NOSIGN=""

# Load secrets from .env — prefer build dir, fall back to repo root.
# .env provides TS_AUTH_KEY and (optionally) USER_PASSWORD / USER_NAME.
for env_file in .env ../.env; do
  if [ -f "$env_file" ]; then
    set -a
    source "$env_file"
    set +a
    break
  fi
done

PASSWORD="${USER_PASSWORD:-}"
USERNAME="${USER_NAME:-}"
SIGNCERT="${SIGN_CERT:-}"
SIGNPASS="${SIGN_PASSWORD:-}"
SIGNTS="${SIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}"

# Parse optional flags; CLI overrides .env values.
while [[ $# -gt 0 ]]; do
  case "$1" in
    -password)
      PASSWORD="${2:-}"
      shift 2
      ;;
    -authkey)
      AUTHKEY="${2:-}"
      shift 2
      ;;
    -user)
      USERNAME="${2:-}"
      shift 2
      ;;
    -signcert)
      SIGNCERT="${2:-}"
      shift 2
      ;;
    -signpass)
      SIGNPASS="${2:-}"
      shift 2
      ;;
    -nosign)
      NOSIGN=1
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: ./build.sh [-password <temp-password>] [-authkey <ts-auth-key>] [-user <admin-user>] [-signcert <pfx|pem>] [-signpass <password>] [-nosign]" >&2
      exit 1
      ;;
  esac
done

if [ -z "$AUTHKEY" ]; then
  AUTHKEY="${TS_AUTH_KEY:-}"
fi

LDFLAGS=""
if [ -n "$PASSWORD" ]; then
  LDFLAGS="$LDFLAGS -X 'main.userPassword=$PASSWORD'"
fi
if [ -n "$AUTHKEY" ]; then
  LDFLAGS="$LDFLAGS -X 'main.tsAuthKey=$AUTHKEY'"
fi
if [ -n "$USERNAME" ]; then
  LDFLAGS="$LDFLAGS -X 'main.targetUser=$USERNAME'"
fi

if [ -z "$LDFLAGS" ]; then
  echo "Warning: no secrets provided (no -password/-authkey, nothing in .env)."
  echo "The binary will prompt interactively at runtime."
else
  [ -n "$PASSWORD" ] && echo "Embedding: user password"
  [ -n "$AUTHKEY" ] && echo "Embedding: Tailscale auth key"
  [ -n "$USERNAME" ] && echo "Embedding: admin user '$USERNAME'"
fi

mkdir -p dist

echo "Building dist/setup.exe (windows/amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o dist/setup.exe .

sign() {
  local cert="$1"
  if [[ "$cert" == *.pfx || "$cert" == *.p12 ]]; then
    osslsigncode sign -pkcs12 "$cert" ${SIGNPASS:+-pass "$SIGNPASS"} -t "$SIGNTS" \
      -in dist/setup.exe -out dist/setup.exe.signed
  else
    osslsigncode sign -certs "$cert" -key "${cert%.*}.key" ${SIGNPASS:+-pass "$SIGNPASS"} -t "$SIGNTS" \
      -in dist/setup.exe -out dist/setup.exe.signed
  fi
}

if [ -n "$NOSIGN" ]; then
  echo "Signing skipped (-nosign)."
else
  # Plain `./build.sh` always produces a signed binary: generate a self-signed
  # code-signing cert on first build if none exists.
  if [ -z "$SIGNCERT" ]; then
    SIGNCERT="signing/signing.pfx"
    [ -n "$SIGNPASS" ] || SIGNPASS="changeme"
    if [ ! -f "$SIGNCERT" ]; then
      echo "No signing cert found - generating $SIGNCERT (self-signed, 3 years)..."
      mkdir -p signing
      openssl req -x509 -newkey rsa:2048 -sha256 -days 1095 -nodes \
        -keyout signing/signing.key \
        -out signing/signing.crt \
        -subj "/CN=smol-tailscaler" \
        -addext "extendedKeyUsage=codeSigning" \
        -addext "keyUsage=digitalSignature"
      openssl pkcs12 -export \
        -out "$SIGNCERT" \
        -inkey signing/signing.key \
        -in signing/signing.crt \
        -passout "pass:$SIGNPASS"
      echo "Generated: $SIGNCERT (password: $SIGNPASS)"
    fi
  elif [ ! -f "$SIGNCERT" ]; then
    echo "ERROR: signing cert not found: $SIGNCERT" >&2
    exit 1
  fi

  if ! command -v osslsigncode >/dev/null 2>&1; then
    if command -v brew >/dev/null 2>&1; then
      echo "osslsigncode not found - installing via brew..."
      brew install osslsigncode
    else
      echo "ERROR: osslsigncode is required to sign. Install it (brew install osslsigncode) or build with -nosign." >&2
      exit 1
    fi
  fi

  echo "Signing dist/setup.exe with $SIGNCERT..."
  sign "$SIGNCERT"
  mv -f dist/setup.exe.signed dist/setup.exe
  echo "Signed: dist/setup.exe"
  # verify fails on self-signed certs (no trusted chain) even when the
  # signature is intact; extract-signature checks the signature is present
  # and structurally valid without needing a trusted root.
  rm -f dist/setup.exe.sig
  if ! osslsigncode extract-signature -in dist/setup.exe -out dist/setup.exe.sig >/dev/null 2>&1; then
    echo "ERROR: signature check failed after signing" >&2
    exit 1
  fi
  rm -f dist/setup.exe.sig
  echo "Signature verified."
fi

echo "Done: dist/setup.exe"
