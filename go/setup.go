package main

import (
	"fmt"
	"os"
)

const sshdConfigPath = `C:\ProgramData\ssh\sshd_config`

func runSetup() error {
	if err := stepOpenSSH(); err != nil {
		warn("OpenSSH step: %s", err)
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
		if _, err := runPS(fmt.Sprintf("Add-WindowsCapability -Online -Name '%s'", capOut)); err == nil {
			ok("OpenSSH Server installed via DISM")
			return nil
		}
	}

	cYellow("  DISM failed, falling back to GitHub release...\n")
	if err := installOpenSSHFromGitHub(); err != nil {
		return err
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
	return os.WriteFile(sshdConfigPath, []byte(cfg), 0)
}
