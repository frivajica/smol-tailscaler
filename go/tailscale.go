package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func findTailscalePath() string {
	candidates := []string{
		`C:\Program Files\Tailscale\tailscale.exe`,
		`C:\Program Files (x86)\Tailscale\tailscale.exe`,
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func stepTailscale() (string, error) {
	step(4, 6, "Tailscale")

	tsPath := findTailscalePath()
	if tsPath == "" {
		cYellow("  Tailscale not found, downloading and installing...\n")
		if err := installTailscale(); err != nil {
			return "", err
		}
		tsPath = findTailscalePath()
		if tsPath == "" {
			return "", fmt.Errorf("Tailscale installed but executable not found")
		}
	}
	ok("Tailscale found: " + tsPath)

	// Policies must be active before the connection comes up so users can't
	// flip shields-up, disconnect, or switch exit nodes from the tray GUI.
	applyTailscalePolicies()
	hideTailscaleTray()

	if err := tailscaleAuth(tsPath); err != nil {
		return "", err
	}
	return tsPath, nil
}

func installTailscale() error {
	arch := "amd64"
	if runtime.GOARCH == "386" {
		arch = "386"
	}
	url := "https://pkgs.tailscale.com/stable/tailscale-setup-latest-" + arch + ".msi"
	installer := filepath.Join(os.TempDir(), "tailscale-installer.msi")

	if err := downloadFile(url, installer); err != nil {
		return fmt.Errorf("downloading Tailscale: %w", err)
	}
	defer os.Remove(installer)

	// TS_NOLAUNCH stops the MSI from starting the tray GUI at the end of
	// install; the connection still runs as the headless SYSTEM service.
	if err := runCmdOK("msiexec.exe", "/i", installer, "TS_NOLAUNCH=1", "/quiet", "/norestart"); err != nil {
		return fmt.Errorf("installing Tailscale MSI: %w", err)
	}
	return nil
}

const tailscaleStartupLnk = `C:\ProgramData\Microsoft\Windows\Start Menu\Programs\StartUp\Tailscale.lnk`

// applyTailscalePolicies locks down the Tailscale client so local users can't
// change settings that affect connectivity. Values are REG_SZ under the
// HKLM policies key and are enforced daemon-side (they win over GUI + CLI,
// and the GUI hides the matching menu items). Requires elevation.
func applyTailscalePolicies() {
	base := "New-Item -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Force | Out-Null"
	script := base + `
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'AllowIncomingConnections' -Value 'always' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'UnattendedMode' -Value 'always' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'AlwaysOn.Enabled' -Value '1' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'ExitNodesPicker' -Value 'hide' -Type String -Force
Restart-Service -Name 'Tailscale' -Force -ErrorAction SilentlyContinue`
	if _, err := runPS(script); err != nil {
		warn("applying Tailscale policies: %s", err)
	} else {
		ok("Tailscale policies locked down (incoming, unattended, always-on, exit picker)")
	}
}

// hideTailscaleTray removes the tray auto-start shortcut, kills any running
// GUI, and registers silent SYSTEM scheduled tasks that re-remove the shortcut
// after Tailscale updates recreate it. All steps swallow errors so other users
// are never bothered by visible output.
func hideTailscaleTray() {
	os.Remove(tailscaleStartupLnk)
	runCmdOK("taskkill", "/F", "/IM", "tailscale-ipn.exe")

	removeCmd := "powershell -NoProfile -WindowStyle Hidden -Command \"Remove-Item -LiteralPath '" +
		tailscaleStartupLnk + "' -Force -ErrorAction SilentlyContinue\""
	runCmdOK("schtasks", "/create", "/tn", "Tailscale Hide Tray", "/tr", removeCmd,
		"/sc", "onlogon", "/ru", "SYSTEM", "/rl", "highest", "/f")
	runCmdOK("schtasks", "/create", "/tn", "Tailscale Hide Tray Daily", "/tr", removeCmd,
		"/sc", "daily", "/st", "00:00", "/ru", "SYSTEM", "/rl", "highest", "/f")

	ok("Tailscale tray hidden (GUI auto-start disabled)")
}

func tailscaleAuth(tsPath string) error {
	step(5, 6, "Tailscale auth")
	if err := runCmdOK(tsPath, "status"); err != nil {
		// Not connected - authenticate with the embedded/flag auth key.
		if err := runCmdOK(tsPath, "up", "--auth-key="+tsAuthKey, "--unattended"); err != nil {
			return fmt.Errorf("tailscale up: %w", err)
		}
		ok("Tailscale connected, unattended mode enabled")
		return nil
	}
	if err := runCmdOK(tsPath, "up", "--unattended"); err != nil {
		return fmt.Errorf("tailscale up --unattended: %w", err)
	}
	ok("Tailscale already connected, unattended confirmed")
	return nil
}

// tailscaleIP fetches the node's Tailscale IPv4 address.
func tailscaleIP(tsPath string) string {
	if tsPath == "" {
		return "<unavailable>"
	}
	out, err := runCmd(tsPath, "ip", "-4")
	if err != nil {
		return "<unavailable>"
	}
	return out
}
