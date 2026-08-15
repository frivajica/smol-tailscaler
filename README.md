# smol-tailscaler

One-click Windows setup for **SSH + Tailscale** — a small Go binary you run once, then `ssh` in.

## What it does

```
1  OpenSSH Server    → installs (DISM, GitHub fallback on Home)
2  Admin user        → creates / resets password, adds to Administrators
3  sshd_config       → sane defaults, password auth ON (disable after keys)
4  Tailscale         → installs if missing; locks down policies (incoming on, always-on), hides tray
5  Tailscale auth    → connects unattended (embedded auth key); self-repairs a broken state store
6  Firewall + start  → port 22 open, sshd + Tailscale auto-start (delayed) with restart-on-failure + boot/logon/daily self-healing tasks
```

Everything is idempotent — safe to re-run, works on a fresh or broken machine.

## Build (on Mac/Linux)

```bash
cp .env.example .env        # set TS_AUTH_KEY (+ optional USER_PASSWORD, USER_NAME)
./build.sh -password "TempPass123"   # embed a temp password (optional)
```

Secrets resolve as: **CLI flag > `.env` > prompt at runtime**.
Output: `dist/setup.exe`

## Code-sign the binary (automatic)

Windows 11 **Smart App Control** blocks unsigned binaries like `setup.exe`.
`build.sh` now does it all for you — on the first build it generates a
self-signed code-signing cert (`signing/signing.pfx`), then builds and
signs `setup.exe` automatically. No extra steps:

```bash
./build.sh
```

Signing uses `osslsigncode` (auto-installed via brew on first use if missing)
with a DigiCert timestamp. Override with `-signcert`/`-signpass` or
`SIGN_CERT`/`SIGN_PASSWORD` env; skip with `-nosign`. On the Windows machine,
trust the cert before running (double-click `signing.crt`, or):

```powershell
Import-Certificate -FilePath C:\signing.crt -CertStoreLocation Cert:\LocalMachine\TrustedPublisher
Import-Certificate -FilePath C:\signing.crt -CertStoreLocation Cert:\LocalMachine\Root
```

Self-signed = trust per-machine, not "reputation". Smart App Control may still
block it depending on its reputation model — fall back to turning SAC off in a
throwaway VM. Never commit `signing/` (private key + password).

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
| `./` | Go source + `build.sh` (`make-signing-cert.sh` = optional cert regen) |
| `dist/setup.exe` | Built binary |
| `run-setup.bat` | Optional VM runner |
| `legacy/` | Original PowerShell version (reference) |
| `docs/` | Security notes + Hyper-V testing guide |
| `.env.example` | `TS_AUTH_KEY`, `USER_PASSWORD`, `USER_NAME` template |
