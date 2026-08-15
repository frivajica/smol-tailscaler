# Hyper-V Testing Guide

Test `setup.exe` without reinstalling Windows.

**Rule:** Hyper-V runs on the **host** (Pro/Enterprise/Server). The **guest** can be Home — it never needs Hyper-V.

## Where commands run

| Where | What runs |
|---|---|
| **Host** (Admin) | All `*VM*`, `*Checkpoint*`, `Enable-WindowsOptionalFeature`, `New-SmbShare` |
| **Guest** (Admin) | `setup.exe`, verification, `ssh` tests |
| **Mac** | `python3 -m http.server` (serves `dist/setup.exe`) |

## The test loop

### 1. Setup — once (host, Admin)
```powershell
# Install Hyper-V (reboot required)
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All

# Create the VM
New-VM -Name "testwin" -MemoryStartupBytes 4GB -Generation 2
Set-VMDvdDrive -VMName "testwin" -Path "C:\iso\win11.iso"   # install Windows
```

### 2. Baseline snapshot — after clean install (host, Admin)
```powershell
Checkpoint-VM -Name "Windows VM NAME" -SnapshotName "clean"
```
Take this right after the clean install. Once a `setup.exe` run succeeds, take a
**fresh `clean` checkpoint again** — otherwise reverts roll back the configured
state too (scheduled tasks, service start types, Tailscale node, cert trust).

### 3. Test — anything, anytime (guest, Admin)
Run `setup.exe`, break permissions, change ACLs — whatever.

### 4. Revert — resets everything in seconds (host, Admin)
```powershell
Restore-VMCheckpoint -VMName "Windows VM NAME" -Name "clean" -Confirm:$false
Start-VM "Windows VM NAME"
```

Reverts registry ACLs, service permissions, Tailscale state, users, firewall.

## Alternative: Differencing disks (heavy iteration)

Parent disk stays pristine; each test boots a throwaway child disk:
```powershell
New-VHD -Path D:\vms\testwin-child.vhdx -ParentPath D:\vms\testwin.vhdx -Differencing
# point the VM at the child disk, test, then:
Remove-VMHardDiskDrive -VMName testwin -ControllerNumber 0 -ControllerLocation 0
Remove-Item D:\vms\testwin-child.vhdx
```

## Sharing files with the VM

| Method | Best for | Command / use |
|---|---|---|
| **HTTP server** | Quick one-off | Mac: `python3 -m http.server 8000 --directory dist` → VM: `http://<mac-ip>:8000/setup.exe` |
| **Cloudflare quick tunnel** | Remote download (one-off) | Mac: `brew install cloudflared && cloudflared tunnel --protocol http2 --url http://localhost:8000` → share the `https://*.trycloudflare.com` URL (server first: `python3 -m http.server 8000 --directory dist`) |
| **Host shared folder** | Testing loops | Host: `New-SmbShare -Name "vmshare" -Path "C:\vm-share" -FullAccess "Everyone"` → VM: `\\<host-ip>\vmshare` |
| **Enhanced Session Mode** | Clipboard + drive redirection | Hyper-V Manager → connect with Enhanced Session Mode |

Use the Mac's Tailscale IP (or LAN IP if the VM has no Tailscale yet).

**Cloudflare quick tunnel tips** — on networks that block outbound QUIC
(UDP 7844), cloudflared hangs retrying and the URL returns `530`; pass
`--protocol http2` to force the fallback transport. Quick-tunnel URLs are
random and ephemeral: killing the cloudflared process kills the link. Serve
`signing/signing.crt` from the same folder so the target machine can trust the
cert before running `setup.exe` (see `docs/SECURITY.md`).

## Caveats

- **Tailscale re-auth** — reverting reverts Tailscale state; `setup.exe` re-registers the node. If "Require device approval" is on, approve a node every cycle. A **single-use** auth key is consumed on first use, so later cycles fail — use a reusable or ephemeral key (see `docs/SECURITY.md`).
- **Signing / Smart App Control** — `setup.exe` is code-signed with a self-signed cert each machine must trust (double-click `signing.crt` → install into Trusted Root + Trusted Publishers). Reverting a checkpoint also reverts that trust, so re-import the cert every cycle — or disable Smart App Control once and leave it off.
- **Home guest** — DISM cannot install the server there (`0x800f0950`), so `setup.exe` prints `DISM did not succeed ... trying GitHub...` and always falls back to GitHub. **Expected and verified.** Install Pro in the VM only to test the Windows Update path. The DISM/query steps show a live `still running (Xs elapsed)...` ticker and auto-fall back to GitHub if Windows Update stalls past their timeouts, so a stuck step no longer sits silent.
- **Elevation** — `setup.exe` self-elevates via UAC on double-click (or use `run-setup.bat`). `-verify` must also run elevated.
- **Snapshots on a different disk** — keep checkpoints/VHDX off the OS disk (performance/isolation).
- **Nested virtualization** — only needed to test Hyper-V *inside* the VM.

## Troubleshooting

**`Restore-VMCheckpoint` is not recognized as a cmdlet**
- Inside the **guest** (no Hyper-V module) → run from the host.
- Needs hyphen: `Restore-VMCheckpoint`, not `RestoreVMCheckpoint`.
- Not elevated → relaunch PowerShell as Administrator.
- Module missing on host → see below.

**`Microsoft-Hyper-V-Management-PowerShell is unknown`**
- Host is Home/non-Pro, or the command ran inside a Home guest. Check:
  ```powershell
  (Get-CimInstance Win32_OperatingSystem).Caption
  Get-WindowsOptionalFeature -Online | Where-Object FeatureName -like "*Hyper-V*"
  ```
- On Pro the list is non-empty; enable the exact names shown, then **reboot**.
- Verify after reboot:
  ```powershell
  Get-Module -ListAvailable Hyper-V
  Get-Command *VMCheckpoint
  ```