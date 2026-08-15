package main

import (
	"fmt"
	"os"
	"strings"

	"setup-windows/internal/ui"
	"setup-windows/internal/winutil"
)

const sshdConfigPath = `C:\ProgramData\ssh\sshd_config`

func runSetup() (string, error) {
	if err := stepOpenSSH(); err != nil {
		return "", fmt.Errorf("OpenSSH install: %w", err)
	}
	if err := stepUser(); err != nil {
		return "", fmt.Errorf("user setup: %w", err)
	}
	if err := writeSshdConfig(); err != nil {
		return "", fmt.Errorf("writing sshd_config: %w", err)
	}
	tsPath, err := stepTailscale()
	if err != nil {
		ui.Warn("Tailscale step: %s", err)
	}
	stepFirewallAndServices()

	// Idempotency gate: setup only reports success when the services it
	// configured are actually running.
	if !winutil.ServiceRunning("sshd") {
		return tsPath, fmt.Errorf("sshd is not RUNNING after setup - OpenSSH install may have failed")
	}
	if !winutil.ServiceRunning("Tailscale") {
		ui.Warn("Tailscale service is not RUNNING after setup")
	}

	// Land SSH sessions in PowerShell instead of cmd.exe.
	if err := setDefaultShell(); err != nil {
		ui.Warn("could not set PowerShell as default shell: %s", err)
	} else {
		ui.Ok("Default shell set to PowerShell")
	}
	return tsPath, nil
}

// setDefaultShell makes OpenSSH launch PowerShell for interactive sessions
// instead of cmd.exe via the DefaultShell registry value.
func setDefaultShell() error {
	const shell = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	script := fmt.Sprintf("New-Item -Path 'HKLM:\\SOFTWARE\\OpenSSH' -Force | Out-Null; Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\OpenSSH' -Name 'DefaultShell' -Value '%s' -Force", shell)
	return winutil.RunCmdOK("powershell", "-NoProfile", "-Command", script)
}

// stepOpenSSH ensures the OpenSSH Server service exists, installing via DISM
// with a GitHub release fallback when Windows Update is unavailable.
func stepOpenSSH() error {
	ui.Step(1, 6, "OpenSSH Server")
	if winutil.ServiceExists("sshd") {
		ui.Ok("OpenSSH Server already installed")
		return nil
	}

	ui.Cyan("  Installing via Windows Update (DISM)...\n")
	capOut, err := winutil.RunPS("(Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*').Name")
	if err == nil && capOut != "" {
		if _, err := winutil.RunPS(fmt.Sprintf("Add-WindowsCapability -Online -Name '%s'", capOut)); err == nil && winutil.ServiceExists("sshd") {
			ui.Ok("OpenSSH Server installed via DISM")
			return nil
		}
		ui.Cyan("  DISM reported success but sshd is missing (reboot pending or unsupported edition); trying GitHub...\n")
	}

	ui.Cyan("  Falling back to GitHub release...\n")
	if err := installOpenSSHFromGitHub(); err != nil {
		return err
	}
	if !winutil.ServiceExists("sshd") {
		return fmt.Errorf("OpenSSH installed but sshd service is still missing")
	}
	ui.Ok("OpenSSH Server installed from GitHub")
	return nil
}

// writeSshdConfig applies a clean base config. Password auth is enabled so the
// user can log in and add their key manually (disable it afterwards).
func writeSshdConfig() error {
	ui.Step(3, 6, "sshd_config")
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
	if winutil.FileExists(sshdConfigPath) {
		// The DISM/GitHub installer locks sshd_config down with SYSTEM-only
		// ACLs that can include explicit Deny ACEs. /reset clears them, then
		// grant write access via SIDs (locale-independent).
		winutil.RunCmdOK("attrib", "-r", sshdConfigPath)
		winutil.RunCmdOK("takeown", "/f", sshdConfigPath)
		if out, err := winutil.RunCmd("icacls", sshdConfigPath, "/reset"); err != nil {
			ui.Gray("  note: icacls reset: %s\n", strings.TrimSpace(out))
		}
		if out, err := winutil.RunCmd("icacls", sshdConfigPath, "/inheritance:r", "/grant", "*S-1-5-18:(F)", "/grant", "*S-1-5-32-544:(F)"); err != nil {
			ui.Gray("  note: icacls grant: %s\n", strings.TrimSpace(out))
		}
	}
	if err := os.WriteFile(sshdConfigPath, []byte(cfg), 0); err != nil {
		// Fallback: PowerShell Set-Content runs in the already-elevated
		// session and often succeeds where os.WriteFile hits ACL issues.
		ps := fmt.Sprintf("Set-Content -Path '%s' -Value @'%s'@ -Encoding ascii", sshdConfigPath, cfg)
		if _, perr := winutil.RunPS(ps); perr != nil {
			return fmt.Errorf("writing sshd_config: %w (PowerShell fallback: %v)", err, perr)
		}
	}
	ui.Ok("sshd_config written")
	return nil
}
