# smol-tailscaler

One PowerShell script to set up SSH + Tailscale on any Windows machine.

## What it does

- Creates an admin user (default `admin`, override with `-UserName`) with full system access
- Installs & configures OpenSSH Server (password auth, key added manually)
- Installs Tailscale if missing, connects with auth key, sets unattended mode
- Opens firewall port 22
- Sets all services to auto-start

## Requirements

- Windows 10/11 **Pro, Enterprise, or Server** (Home edition doesn't support OpenSSH Server via script)
- Run as **Administrator**
- Internet connection (for Tailscale download)

## Get your SSH public key

**macOS / Linux:**
```bash
cat ~/.ssh/id_ed25519.pub
```

**Windows (PowerShell):**
```powershell
type $env:USERPROFILE\.ssh\id_ed25519.pub
```

**No key yet?** Generate one:
```bash
ssh-keygen -t ed25519 -C "your@email.com"
```

## Quick start

Windows may block script execution by default. Run from an **Administrator** PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\setup-ssh.ps1 `
  -UserPassword "YourStrongPassword123" `
  -TsAuthKey "tskey-auth-kXXXXX-XXXXXXXXXXXXXXXX" `
  -UserName "admin"
```

## Parameters

| Parameter | Description |
|---|---|
| `-UserPassword` | Password for the admin user |
| `-TsAuthKey` | Tailscale auth key (from [admin console](https://login.tailscale.com/admin/settings/keys)) |
| `-UserName` | Admin user to create (optional, default: `admin`) |

## After running

```bash
ssh <user>@<tailscale-ip>
```

The admin user has admin access — can see all processes, containers, databases, services, files, and network config on the machine.

## Add your SSH key manually

After running the script, log in with the password and place your public key:

```powershell
New-Item -ItemType Directory -Path "$env:USERPROFILE\.ssh" -Force
"ssh-ed25519 AAAA... your@email.com" | Set-Content -Path "$env:USERPROFILE\.ssh\authorized_keys"
icacls "$env:USERPROFILE\.ssh\authorized_keys" /inheritance:r /grant "SYSTEM:(F)" /grant "Administrators:(F)" /grant "$env:USERNAME:(R)"
```

Then disable password auth (optional, recommended):

```powershell
$configPath = "$env:ProgramData\ssh\sshd_config"
(Get-Content $configPath) -replace "PasswordAuthentication yes", "PasswordAuthentication no" | Set-Content $configPath
Restart-Service sshd
```

## What gets configured

| Step | Action |
|---|---|
| 1 | OpenSSH Server installed (via Windows Update or GitHub fallback) |
| 2 | Admin user created or password updated |
| 3 | `sshd_config` — password auth enabled, key auth ready |
| 4 | Tailscale installed (if missing), path auto-detected |
| 5 | Tailscale authenticated, unattended mode enforced |
| 6 | Firewall rule + services set to auto-start |

## Final report

At the end, the script prints a summary you should save:

- Username, password, hostname, Tailscale IP
- Tailscale CLI install path
- Ready-to-copy SSH connect command
- Service status table
- Full list of local users (name, status, role, has password)

## Notes

- **Language detection** — Automatically detects the Administrators group name for Spanish, English, German, and French Windows
- **Idempotent** — Safe to re-run. Won't duplicate users, keys, or firewall rules
- **Tailscale** — Downloads and installs automatically if not present
- **UTF-8** — Console encoding is forced to UTF-8 to prevent character corruption

## Troubleshooting

**Error 0x800f0950 on OpenSSH install:**
- Windows Update may be disabled or blocked
- Windows Home edition doesn't support OpenSSH Server
- Manual fix: Settings > Apps > Optional Features > Add OpenSSH Server
