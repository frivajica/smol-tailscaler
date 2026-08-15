# Security Annex

## Tailscale Risks

A compromised node on your tailnet can:

| Can Do | Cannot Do |
|---|---|
| Reach other nodes on the tailnet (if ACLs allow) | Change tailnet ACLs |
| Access internal services on other machines | Add/remove nodes from your tailnet |
| Use the node's Tailscale IP for lateral movement | Modify Tailscale DNS settings |
| Run commands as any local user | Access the Tailscale admin console |
| See running processes and open ports | See other nodes' auth keys or secrets |

**The real risk is lateral movement.** Once on your tailnet, the node can reach other machines.

## Mitigations

### 1. Tailscale ACLs (Recommended)

Restrict which nodes can talk to which in your [tailnet policy](https://login.tailscale.com/admin/acls):

```json
{
  "acls": [
    {
      "action": "accept",
      "src": ["autogroup:admin"],
      "dst": ["autogroup:self:*"]
    }
  ]
}
```

This limits nodes to only reach themselves. Add explicit rules per node as needed.

### 2. Enable Device Approval

In [Tailscale admin console](https://login.tailscale.com/admin/settings/general):
- Turn on **"Require device approval"**
- New nodes must be manually approved before joining

### 3. Use Ephemeral Auth Keys

Instead of reusable auth keys, generate ephemeral ones that expire after first use:
- Admin Console > Settings > Keys > Generate Auth Key > Check **"Ephemeral"**

### 4. Strong Passwords

The `frivajica` user password should be complex. Never use simple passwords like `1824` on internet-facing machines.

## SSH Hardening

The script configures:
- Password authentication enabled initially (so you can log in and add your key manually)
- No empty passwords (`PermitEmptyPasswords no`)
- Key auth ready (`PubkeyAuthentication yes`)

**After adding your SSH key manually, disable password auth:**

```powershell
$configPath = "$env:ProgramData\ssh\sshd_config"
(Get-Content $configPath) -replace "PasswordAuthentication yes", "PasswordAuthentication no" | Set-Content $configPath
Restart-Service sshd
```

**Additional hardening you can add manually** to `C:\ProgramData\ssh\sshd_config`:

```
MaxAuthTries 3
LoginGraceTime 30
ClientAliveInterval 300
ClientAliveCountMax 2
LogLevel VERBOSE
```

Then restart: `Restart-Service sshd`

## What This Script Protects Against

- **Brute force SSH** — Password auth can be disabled after adding your key manually
- **Casual users** — SSH requires admin user credentials

## What This Script Does NOT Protect Against

- **Weak passwords** — Password auth is enabled by default until you add a key and disable it
- **Determined admins** — Any Windows admin can take ownership of files and change service ACLs
- **Strong password attacks** — If the password is weak, it can be cracked locally
- **Tailscale admin compromise** — If someone gets your Tailscale account credentials, they control the entire tailnet
- **Physical access** — Someone with physical access can boot from USB and extract data
