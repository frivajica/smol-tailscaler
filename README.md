# smol-ssh-install

One-click Windows setup for **SSH + Tailscale**, packaged as a small Go binary.

## What it does

- Creates admin user `frivajica`
- Installs OpenSSH Server
- Connects Tailscale (unattended)
- Opens firewall port 22
- Auto-starts services

## Build (on Mac/Linux)

```bash
cd go
cp ../.env.example .env     # set TS_AUTH_KEY
./build.sh -password "TempPass123"   # optional: embed temp password
```

Output: `go/dist/setup.exe`

No secrets? Run `./build.sh` alone — the binary prompts at runtime.

## Run (on Windows, as Administrator)

```powershell
setup.exe             # prompts for anything missing
setup.exe -verify     # check state, no changes
setup.exe -silent     # embedded values only
setup.exe -user=NAME  # change user (default: frivajica)
```

## Then

```bash
ssh frivajica@<tailscale-ip>
```

Add your SSH key manually, then disable password auth (see `SECURITY.md`).

## Files

| Path | Purpose |
|---|---|
| `go/` | Go source + `build.sh` |
| `dist/setup.exe` | Built binary |
| `legacy/` | Original PowerShell version (reference) |
| `.env.example` | `TS_AUTH_KEY` template |
