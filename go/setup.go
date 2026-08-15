package main

import (
	"fmt"
	"os"
	"strings"
)

const sshdConfigPath = `C:\ProgramData\ssh\sshd_config`

func runSetup() error {
	if err := stepOpenSSH(); err != nil {
		return fmt.Errorf("OpenSSH install: %w", err)
	}
	if err := stepUser(); err != nil {
		return fmt.Errorf("user setup: %w", err)
	}
	if err := writeSshdConfig(); err != nil {
		return fmt.Errorf("writing sshd_config: %w", err)
	}
	tsPath, err := stepTailscale()
	if err != nil {
		warn("Tailscale step: %s", err)
	}
	stepFirewallAndServices()

	// Idempotency gate: setup only reports success when the services it
	// configured are actually running.
	if !serviceRunning("sshd") {
		return fmt.Errorf("sshd is not RUNNING after setup - OpenSSH install may have failed")
	}
	if !serviceRunning("Tailscale") {
		warn("Tailscale service is not RUNNING after setup")
	}

	printReport(tsPath)
	return nil
}

// stepOpenSSH ensures the OpenSSH Server service exists, installing via DISM
// with a GitHub release fallback when Windows Update is unavailable.
func stepOpenSSH() error {
	step(1, 6, "OpenSSH Server")
	if serviceExists("sshd") {
		ok("OpenSSH Server already installed")
		return nil
	}

	cYellow("  Installing via Windows Update (DISM)...\n")
	capOut, err := runPS("(Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*').Name")
	if err == nil && capOut != "" {
		if _, err := runPS(fmt.Sprintf("Add-WindowsCapability -Online -Name '%s'", capOut)); err == nil && serviceExists("sshd") {
			ok("OpenSSH Server installed via DISM")
			return nil
		}
		cYellow("  DISM reported success but sshd is missing (reboot pending or unsupported edition); trying GitHub...\n")
	}

	cYellow("  Falling back to GitHub release...\n")
	if err := installOpenSSHFromGitHub(); err != nil {
		return err
	}
	if !serviceExists("sshd") {
		return fmt.Errorf("OpenSSH installed but sshd service is still missing")
	}
	ok("OpenSSH Server installed from GitHub")
	return nil
}

// writeSshdConfig applies a clean base config. Password auth is enabled so the
// user can log in and add their key manually (disable it afterwards).
func writeSshdConfig() error {
	step(3, 6, "sshd_config")
	cfg := `Port 22
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
PasswordAuthentication yes
PermitEmptyPasswords no
Subsystem sftp sftp-server.exe
`
	if err := os.MkdirAll(`C:\ProgramData\ssh`, 0); err != nil {
		return err
	}

	// The DISM/GitHub installer locks sshd_config down with SYSTEM-only ACLs
	// (possibly including Deny ACEs). Take ownership and grant write access via
	// SIDs so the overwrite below can't hit "Access denied".
	if fileExists(sshdConfigPath) {
		// The DISM/GitHub installer locks sshd_config down with SYSTEM-only
		// ACLs that can include explicit Deny ACEs. /reset clears them, then
		// grant write access via SIDs (locale-independent).
		runCmdOK("attrib", "-r", sshdConfigPath)
		runCmdOK("takeown", "/f", sshdConfigPath)
		if out, err := runCmd("icacls", sshdConfigPath, "/reset"); err != nil {
			cGray("  note: icacls reset: %s\n", strings.TrimSpace(out))
		}
		if out, err := runCmd("icacls", sshdConfigPath, "/inheritance:r", "/grant", "*S-1-5-18:(F)", "/grant", "*S-1-5-32-544:(F)"); err != nil {
			cGray("  note: icacls grant: %s\n", strings.TrimSpace(out))
		}
	}
	if err := os.WriteFile(sshdConfigPath, []byte(cfg), 0); err != nil {
		// Fallback: PowerShell Set-Content runs in the already-elevated
		// session and often succeeds where os.WriteFile hits ACL issues.
		ps := fmt.Sprintf("Set-Content -Path '%s' -Value @'%s'@ -Encoding ascii", sshdConfigPath, cfg)
		if _, perr := runPS(ps); perr != nil {
			return fmt.Errorf("writing sshd_config: %w (PowerShell fallback: %v)", err, perr)
		}
	}
	return nil
}
