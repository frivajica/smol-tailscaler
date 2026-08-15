package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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
	statusOut, err := runCmd(tsPath, "status")
	if err != nil {
		cGray("  tailscale status: %s\n", strings.TrimSpace(statusOut))
		// Not connected - authenticate with the embedded/flag auth key.
		out, upErr := runCmd(tsPath, "up", "--auth-key="+tsAuthKey, "--unattended")
		if upErr != nil {
			// A broken state store (failed TPM->plaintext migration or a file
			// locked by a stale process) blocks backend start entirely. Clear
			// the node state per Tailscale's recovery steps, then retry once.
			if strings.Contains(strings.ToLower(out+statusOut), "state store") {
				warn("Tailscale state store is unhealthy - resetting node state and retrying")
				if repairErr := repairTailscaleState(); repairErr != nil {
					return fmt.Errorf("Tailscale state store repair: %w", repairErr)
				}
				if out, retryErr := runCmd(tsPath, "up", "--auth-key="+tsAuthKey, "--unattended"); retryErr != nil {
					return fmt.Errorf("tailscale up after state reset: %w (%s)", retryErr, strings.TrimSpace(out))
				}
			} else {
				return fmt.Errorf("tailscale up: %w (%s)", upErr, strings.TrimSpace(out))
			}
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

// repairTailscaleState stops the service, removes the node state files, and
// restarts it so the backend can re-initialize cleanly. The node is then
// re-registered with the embedded auth key.
func repairTailscaleState() error {
	runCmdOK("taskkill", "/F", "/IM", "tailscale-ipn.exe")
	runCmdOK("sc.exe", "stop", "Tailscale")
	time.Sleep(2 * time.Second)

	// Try to clear any restrictive ACLs on the state dir first so deletion
	// isn't blocked by "Access is denied".
	runCmdOK("takeown", "/F", `C:\ProgramData\Tailscale`, "/R", "/D", "Y")
	runCmdOK("icacls", `C:\ProgramData\Tailscale`, "/reset", "/T", "/C")

	paths := []string{
		`C:\ProgramData\Tailscale\server-state.conf`,
		`C:\ProgramData\Tailscale\tailscaled.state`,
		`C:\ProgramData\Tailscale\tailscaled.state.lock`,
		`C:\ProgramData\Tailscale\ipn-server-state.json`,
		os.Getenv("USERPROFILE") + `\AppData\Local\Tailscale`,
	}
	for _, p := range paths {
		os.RemoveAll(p)
	}

	if err := runCmdOK("sc.exe", "start", "Tailscale"); err != nil {
		return fmt.Errorf("starting Tailscale service: %w", err)
	}
	for i := 0; i < 30; i++ {
		if serviceRunning("Tailscale") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("Tailscale service did not reach RUNNING state after repair")
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
