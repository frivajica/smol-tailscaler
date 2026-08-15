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

	if err := runCmdOK("msiexec.exe", "/i", installer, "/quiet", "/norestart"); err != nil {
		return fmt.Errorf("installing Tailscale MSI: %w", err)
	}
	return nil
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
