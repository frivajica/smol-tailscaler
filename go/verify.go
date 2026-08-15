package main

import (
	"fmt"
	"os"
	"strings"

	"setup-windows/internal/config"
	"setup-windows/internal/ssh"
	"setup-windows/internal/tailscale"
	"setup-windows/internal/ui"
	"setup-windows/internal/users"
	"setup-windows/internal/winutil"
)

func verify(cfg *config.Config) int {
	ui.Header("Verify: SSH + Tailscale state")

	failed := false

	check := func(label string, pass bool, detail string) {
		if pass {
			ui.Green("  [PASS] %s\n", label)
		} else {
			ui.Red("  [FAIL] %s\n", label)
			failed = true
		}
		if detail != "" {
			ui.Gray("         %s\n", detail)
		}
	}

	// 1. OpenSSH service
	if winutil.ServiceExists("sshd") {
		out, _ := winutil.RunCmd("sc.exe", "query", "sshd")
		state := winutil.ServiceState(out)
		check("sshd service", state == "RUNNING", "state: "+state)
		if qc, err := winutil.RunCmd("sc.exe", "qc", "sshd"); err == nil {
			check("sshd auto-start", strings.Contains(winutil.StartType(qc), "AUTO_START"), "start: "+winutil.StartType(qc))
		}
	} else {
		check("sshd service", false, "not installed")
	}

	// 2. sshd_config
	if winutil.FileExists(ssh.ConfigPath) {
		data, err := os.ReadFile(ssh.ConfigPath)
		content := strings.ToLower(string(data))
		check("sshd_config present", err == nil, ssh.ConfigPath)
		check("PubkeyAuthentication enabled", strings.Contains(content, "pubkeyauthentication yes"), "")
		check("PasswordAuthentication enabled", strings.Contains(content, "passwordauthentication yes"), "")
	} else {
		check("sshd_config present", false, "missing")
	}

	// 3. Firewall rule
	if winutil.RunCmdOK("netsh", "advfirewall", "firewall", "show", "rule", "name=OpenSSH-Server-In-TCP") == nil {
		check("Firewall port 22", true, "OpenSSH-Server-In-TCP exists")
	} else {
		check("Firewall port 22", false, "rule missing")
	}

	// 3b. Default shell is PowerShell (not cmd.exe)
	out, err := winutil.RunPS("(Get-ItemProperty -Path 'HKLM:\\SOFTWARE\\OpenSSH' -Name DefaultShell -ErrorAction SilentlyContinue).DefaultShell")
	check("Default shell is PowerShell", err == nil && strings.Contains(strings.ToLower(out), "powershell"), strings.TrimSpace(out))

	// 4. User + admin membership
	if users.Exists(cfg.TargetUser) {
		check("User "+cfg.TargetUser, true, "")
		group, err := winutil.AdminGroupName()
		inGroup := err == nil && winutil.RunCmdOK("net", "localgroup", group, cfg.TargetUser) == nil
		check(cfg.TargetUser+" in admin group", inGroup, "group: "+group)
	} else {
		check("User "+cfg.TargetUser, false, "not found")
	}

	// 5. Tailscale
	tsPath := tailscale.Path()
	if tsPath == "" {
		check("Tailscale installed", false, "not found")
	} else {
		check("Tailscale installed", true, tsPath)
		if err := winutil.RunCmdOK(tsPath, "status"); err == nil {
			ip := tailscale.IP(tsPath)
			check("Tailscale connected", ip != "<unavailable>" && ip != "", "IP: "+ip)
		} else {
			check("Tailscale connected", false, "not logged in")
		}
	}

	fmt.Println()
	if failed {
		ui.Red("Result: one or more checks FAILED")
		return 1
	}
	ui.Green("Result: all checks passed")
	return 0
}
