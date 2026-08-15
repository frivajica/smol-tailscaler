# Security Notes (Go binary)

## Embedded secrets

`build.sh` embeds `TS_AUTH_KEY` and the temporary user password into the binary
via `-ldflags -X`. This is **security by obscurity**:

- Extractable with `strings setup.exe`
- Fine for personal use — the secrets live on the machines you set up anyway
- **Never share/distribute the built `.exe`** — it carries your auth key and password
- The user password is meant to be **temporary**: change it after first login

If you need to share the binary, build without secrets (`./build.sh` with no
values) so it prompts interactively instead.

## Tailscale risk

A node that joins your tailnet can reach other nodes if your ACLs allow it.
Even though this setup uses an auth key, treat the machine as part of your
network boundary.

Recommended Tailscale settings:
- Enable **"Require device approval"** in the admin console
- Use **ephemeral** auth keys where possible
- Tighten **ACLs** so nodes can only reach what they must

## SSH hardening checklist (after manual key setup)

1. Place your public key in the user's `.ssh\authorized_keys`
2. Set strict ACLs on that file:
   ```
   icacls "$env:USERPROFILE\.ssh\authorized_keys" /inheritance:r /grant "SYSTEM:(F)" "Administrators:(F)" "$env:USERNAME:(R)"
   ```
3. Disable password auth in `sshd_config` and restart sshd
4. Consider `MaxAuthTries 3` and `LogLevel VERBOSE`

## What the binary does NOT protect against

- **Determined admins** — any Windows admin can take ownership of files
- **Weak/unchanged passwords** — password auth is on by default until you disable it
- **Tailscale admin compromise** — control-plane access means tailnet-wide control
- **Physical access** — nothing stops offline extraction
