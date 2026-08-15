#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

PASSWORD=""

# Load secrets from .env (TS_AUTH_KEY, etc).
if [ -f .env ]; then
  set -a
  source .env
  set +a
fi

# Parse -password <value>.
while [[ $# -gt 0 ]]; do
  case "$1" in
    -password)
      PASSWORD="${2:-}"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: ./build.sh [-password <temp-password>]" >&2
      exit 1
      ;;
  esac
done

LDFLAGS=""
if [ -n "${PASSWORD:-}" ]; then
  LDFLAGS="$LDFLAGS -X 'main.userPassword=$PASSWORD'"
fi
if [ -n "${TS_AUTH_KEY:-}" ]; then
  LDFLAGS="$LDFLAGS -X 'main.tsAuthKey=$TS_AUTH_KEY'"
fi

if [ -z "$LDFLAGS" ]; then
  echo "Warning: no secrets provided (no -password, no TS_AUTH_KEY in .env)."
  echo "The binary will prompt interactively at runtime."
fi

mkdir -p dist

echo "Building dist/setup.exe (windows/amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o dist/setup.exe .

echo "Done: dist/setup.exe"
