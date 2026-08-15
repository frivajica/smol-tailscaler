// Package tailscale installs, configures, and authenticates the Tailscale
// client with a locked-down, tray-less, unattended setup.
package tailscale

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"smol-tailscaler/internal/config"
	"smol-tailscaler/internal/ui"
	"smol-tailscaler/internal/winutil"
)

// Path returns the installed tailscale.exe path, or "" if not found.
func Path() string {
	candidates := []string{
		`C:\Program Files\Tailscale\tailscale.exe`,
		`C:\Program Files (x86)\Tailscale\tailscale.exe`,
	}
	for _, p := range candidates {
		if winutil.FileExists(p) {
			return p
		}
	}
	return ""
}

// Ensure installs Tailscale if missing, applies lockdown policies, and hides
// the tray GUI. It returns the installed tailscale.exe path.
func Ensure() (string, error) {
	tsPath := Path()
	if tsPath == "" {
		ui.Cyan("  Tailscale not found, downloading and installing...\n")
		if err := install(); err != nil {
			return "", err
		}
		tsPath = Path()
		if tsPath == "" {
			return "", fmt.Errorf("Tailscale installed but executable not found")
		}
	}
	ui.Ok("Tailscale found: " + tsPath)

	// Policies must be active before the connection comes up so users can't
	// flip shields-up, disconnect, or switch exit nodes from the tray GUI.
	applyPolicies()
	hideTray()

	return tsPath, nil
}

func install() error {
	arch := "amd64"
	if runtime.GOARCH == "386" {
		arch = "386"
	}
	url := "https://pkgs.tailscale.com/stable/tailscale-setup-latest-" + arch + ".msi"
	installer := filepath.Join(os.TempDir(), "tailscale-installer.msi")

	if err := winutil.DownloadFile(url, installer); err != nil {
		return fmt.Errorf("downloading Tailscale: %w", err)
	}
	defer os.Remove(installer)

	// TS_NOLAUNCH stops the MSI from starting the tray GUI at the end of
	// install; the connection still runs as the headless SYSTEM service.
	if err := winutil.RunCmdOK("msiexec.exe", "/i", installer, "TS_NOLAUNCH=1", "/quiet", "/norestart"); err != nil {
		return fmt.Errorf("installing Tailscale MSI: %w", err)
	}
	return nil
}

const startupLnk = `C:\ProgramData\Microsoft\Windows\Start Menu\Programs\StartUp\Tailscale.lnk`

// applyPolicies locks down the Tailscale client so local users can't change
// settings that affect connectivity. Values are REG_SZ under the HKLM policies
// key and are enforced daemon-side (they win over GUI + CLI, and the GUI hides
// the matching menu items). Requires elevation.
func applyPolicies() {
	base := "New-Item -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Force | Out-Null"
	script := base + `
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'AllowIncomingConnections' -Value 'always' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'UnattendedMode' -Value 'always' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'AlwaysOn.Enabled' -Value '1' -Type String -Force
Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\Policies\\Tailscale' -Name 'ExitNodesPicker' -Value 'hide' -Type String -Force
Restart-Service -Name 'Tailscale' -Force -ErrorAction SilentlyContinue`
	if _, err := winutil.RunPS(script); err != nil {
		ui.Warn("applying Tailscale policies: %s", err)
	} else {
		ui.Ok("Tailscale policies locked down (incoming, unattended, always-on, exit picker)")
	}
}

// hideTray removes the tray auto-start shortcut, kills any running GUI, and
// registers silent SYSTEM scheduled tasks that re-remove the shortcut after
// Tailscale updates recreate it. All steps swallow errors so other users are
// never bothered by visible output.
func hideTray() {
	os.Remove(startupLnk)
	winutil.RunCmdOK("taskkill", "/F", "/IM", "tailscale-ipn.exe")

	removeCmd := "powershell -NoProfile -WindowStyle Hidden -Command \"Remove-Item -LiteralPath '" +
		startupLnk + "' -Force -ErrorAction SilentlyContinue\""
	winutil.RunCmdOK("schtasks", "/create", "/tn", "Tailscale Hide Tray", "/tr", removeCmd,
		"/sc", "onlogon", "/ru", "SYSTEM", "/rl", "highest", "/f")
	winutil.RunCmdOK("schtasks", "/create", "/tn", "Tailscale Hide Tray Daily", "/tr", removeCmd,
		"/sc", "daily", "/st", "00:00", "/ru", "SYSTEM", "/rl", "highest", "/f")

	ui.Ok("Tailscale tray hidden (GUI auto-start disabled)")
}

// Auth connects the node with the configured auth key, or confirms an existing
// connection, repairing a broken state store and retrying once if needed.
func Auth(cfg *config.Config, tsPath string) error {
	if tsPath == "" {
		return fmt.Errorf("Tailscale is not installed")
	}

	// Right after boot the daemon may still be starting; `tailscale status`
	// then reports "Tailscale is starting" with an error, which would look
	// like a logged-out node and trigger a needless re-auth. Wait it out.
	for i := 0; i < 5; i++ {
		out, err := winutil.RunCmd(tsPath, "status")
		if err == nil || !strings.Contains(strings.ToLower(out), "starting") {
			break
		}
		ui.Gray("  tailscale status: %s\n", strings.TrimSpace(out))
		time.Sleep(3 * time.Second)
	}

	statusOut, err := winutil.RunCmd(tsPath, "status")
	if err != nil {
		ui.Gray("  tailscale status: %s\n", strings.TrimSpace(statusOut))
		// Not connected - authenticate with the embedded/flag auth key.
		out, upErr := winutil.RunCmd(tsPath, "up", "--auth-key="+cfg.TsAuthKey, "--unattended")
		if upErr != nil {
			// A broken state store (failed TPM->plaintext migration or a file
			// locked by a stale process) blocks backend start entirely. Clear
			// the node state per Tailscale's recovery steps, then retry once.
			if strings.Contains(strings.ToLower(out+statusOut), "state store") {
				ui.Warn("Tailscale state store is unhealthy - resetting node state and retrying")
				if repairErr := repairState(); repairErr != nil {
					return fmt.Errorf("Tailscale state store repair: %w", repairErr)
				}
				if out, retryErr := winutil.RunCmd(tsPath, "up", "--auth-key="+cfg.TsAuthKey, "--unattended"); retryErr != nil {
					return fmt.Errorf("tailscale up after state reset: %w (%s)", retryErr, strings.TrimSpace(out))
				}
			} else {
				return fmt.Errorf("tailscale up: %w (%s)", upErr, strings.TrimSpace(out))
			}
		}
		ui.Ok("Tailscale connected, unattended mode enabled")
		return nil
	}
	if err := winutil.RunCmdOK(tsPath, "up", "--unattended"); err != nil {
		return fmt.Errorf("tailscale up --unattended: %w", err)
	}
	ui.Ok("Tailscale already connected, unattended confirmed")
	return nil
}

// repairState stops the service, removes the node state files, and restarts it
// so the backend can re-initialize cleanly. The node is then re-registered
// with the embedded auth key.
func repairState() error {
	winutil.RunCmdOK("taskkill", "/F", "/IM", "tailscale-ipn.exe")
	winutil.RunCmdOK("sc.exe", "stop", "Tailscale")
	time.Sleep(2 * time.Second)

	// Try to clear any restrictive ACLs on the state dir first so deletion
	// isn't blocked by "Access is denied".
	winutil.RunCmdOK("takeown", "/F", `C:\ProgramData\Tailscale`, "/R", "/D", "Y")
	winutil.RunCmdOK("icacls", `C:\ProgramData\Tailscale`, "/reset", "/T", "/C")

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

	if err := winutil.RunCmdOK("sc.exe", "start", "Tailscale"); err != nil {
		return fmt.Errorf("starting Tailscale service: %w", err)
	}
	for i := 0; i < 30; i++ {
		if winutil.ServiceRunning("Tailscale") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("Tailscale service did not reach RUNNING state after repair")
}

// IP fetches the node's Tailscale IPv4 address.
func IP(tsPath string) string {
	if tsPath == "" {
		return "<unavailable>"
	}
	out, err := winutil.RunCmd(tsPath, "ip", "-4")
	if err != nil {
		return "<unavailable>"
	}
	return out
}
