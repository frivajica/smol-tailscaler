# Security Notes (Go binary)

## Embedded secrets

`build.sh` embeds `TS_AUTH_KEY` and the temporary user password into the binary
via `-ldflags -X`. This is **security by obscurity**:

- Extractable with `strings setup.exe`
- Fine for personal use — the secrets live on the machines you set up anyway
- **Never share/distribute the built `.exe`** — it carries your auth key and password
- The user password is meant to be **temporary**: change it after first login

If you need to share the binary, build without secrets (`./build.sh` with no
`-password`/`-authkey`, nothing in `.env`) so it prompts interactively instead.
The binary is still signed with your cert — add `-nosign` if you don't want that.

## Signing certificate

`build.sh` auto-generates a self-signed code-signing cert
(`go/signing/signing.pfx`, password `changeme` by default) and signs every
build. Treat the cert + its password as a credential:

- Anyone with the PFX can sign binaries that your machines will trust (you
  imported the cert into **Trusted Root** and **Trusted Publishers** on each
  target machine).
- `go/signing/` is gitignored — never commit it.
- Change the password from the `changeme` default (`-signpass` or
  `SIGN_PASSWORD` env).
- Self-signed means per-machine trust, not reputation — **Smart App Control
  may still block the binary** depending on its reputation model. Fall back to
  turning SAC off in a throwaway VM.

## Tailscale risk

A node that joins your tailnet can reach other nodes if your ACLs allow it.
Even though this setup uses an auth key, treat the machine as part of your
network boundary.

The setup also forces inbound connections **always on** (policy
`AllowIncomingConnections=always`) and `AlwaysOn.Enabled=1`, so nodes accept
inbound tailnet traffic by default. Your tailnet **ACLs** — not the Windows
firewall — are therefore the real boundary between nodes.

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
