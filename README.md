# smol-tailscaler

One-click Windows setup for **SSH + Tailscale** — a small Go binary you run once, then `ssh` in.

## What it does

```
1  OpenSSH Server    → installs (DISM, GitHub fallback on Home)
2  Admin user        → creates / resets password, adds to Administrators
3  sshd_config       → sane defaults, password auth ON (disable after keys)
4  Tailscale         → installs if missing
5  Tailscale auth    → connects unattended (embedded auth key)
6  Firewall + start  → port 22 open, sshd + Tailscale auto-start
```

Everything is idempotent — safe to re-run, works on a fresh or broken machine.

## Build (on Mac/Linux)

```bash
cd go
cp ../.env.example .env        # set TS_AUTH_KEY (+ optional USER_PASSWORD, USER_NAME)
./build.sh -password "TempPass123"   # embed a temp password (optional)
```

Secrets resolve as: **CLI flag > `.env` > prompt at runtime**.
Output: `go/dist/setup.exe`

## Run (on Windows)

Double-click `setup.exe` (self-elevates via UAC), or:

```powershell
setup.exe          # prompts for anything missing
setup.exe -verify  # check state only
setup.exe -silent  # embedded values only, never prompts
setup.exe -user=NAME
```

## Then

```bash
ssh <user>@<tailscale-ip>
```

Add your SSH key, then disable password auth — see `docs/SECURITY.md`.

## Files

| Path | Purpose |
|---|---|
| `go/` | Go source + `build.sh` |
| `go/dist/setup.exe` | Built binary |
| `go/run-setup.bat` | Optional VM runner |
| `legacy/` | Original PowerShell version (reference) |
| `docs/` | Security notes + Hyper-V testing guide |
| `.env.example` | `TS_AUTH_KEY`, `USER_PASSWORD`, `USER_NAME` template |
