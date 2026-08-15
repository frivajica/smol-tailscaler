#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

PASSWORD=""
AUTHKEY=""
USERNAME=""

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
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: ./build.sh [-password <temp-password>] [-authkey <ts-auth-key>] [-user <admin-user>]" >&2
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

echo "Done: dist/setup.exe"
