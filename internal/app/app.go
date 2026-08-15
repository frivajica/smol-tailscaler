// Package app orchestrates the ordered setup steps.
package app

import (
	"fmt"
	"time"

	"smol-tailscaler/internal/config"
	"smol-tailscaler/internal/firewall"
	"smol-tailscaler/internal/ssh"
	"smol-tailscaler/internal/tailscale"
	"smol-tailscaler/internal/ui"
	"smol-tailscaler/internal/users"
	"smol-tailscaler/internal/winutil"
)

const totalSteps = 6

// runStep prints a numbered step header, then runs the step.
func runStep(num int, label string, fn func() error) error {
	ui.Step(num, totalSteps, label)
	return fn()
}

// Run executes the full setup: banner, ordered steps, service gates, and the
// PowerShell default shell. It returns the installed Tailscale CLI path.
func Run(cfg *config.Config) (string, error) {
	ui.Header("Setup: SSH + Tailscale")
	fmt.Printf("Target user: %s\n", cfg.TargetUser)

	if err := runStep(1, "OpenSSH Server", ssh.EnsureInstalled); err != nil {
		return "", fmt.Errorf("OpenSSH install: %w", err)
	}
	if err := runStep(2, "Admin user '"+cfg.TargetUser+"'", func() error { return users.EnsureAdmin(cfg) }); err != nil {
		return "", fmt.Errorf("user setup: %w", err)
	}
	if err := runStep(3, "sshd_config", ssh.WriteConfig); err != nil {
		return "", fmt.Errorf("writing sshd_config: %w", err)
	}

	var tsPath string
	if err := runStep(4, "Tailscale", func() error {
		p, err := tailscale.Ensure()
		if err == nil {
			tsPath = p
		}
		return err
	}); err != nil {
		ui.Warn("Tailscale step: %s", err)
	} else if err := runStep(5, "Tailscale auth", func() error { return tailscale.Auth(cfg, tsPath) }); err != nil {
		ui.Warn("Tailscale auth: %s", err)
	}

	runStep(6, "Firewall + services", firewall.Ensure)

	// Idempotency gate: setup only reports success when the services it
	// configured are actually running. A freshly started service transitions
	// through START_PENDING first, so wait for readiness rather than judging a
	// single snapshot taken right after `sc start`.
	if !winutil.WaitRunning("sshd", 60*time.Second) {
		return tsPath, fmt.Errorf("sshd is not RUNNING after setup - OpenSSH install may have failed")
	}
	if !winutil.WaitRunning("Tailscale", 15*time.Second) {
		ui.Warn("Tailscale service is not RUNNING after setup")
	}

	// Land SSH sessions in PowerShell instead of cmd.exe.
	if err := ssh.SetDefaultShell(); err != nil {
		ui.Warn("could not set PowerShell as default shell: %s", err)
	} else {
		ui.Ok("Default shell set to PowerShell")
	}
	return tsPath, nil
}
