package main

import (
	"fmt"
	"os"
	"strings"
)

func verify() int {
	cCyan("==============================================")
	cCyan(" Verify: SSH + Tailscale state               ")
	cCyan("==============================================")

	failed := false

	check := func(label string, pass bool, detail string) {
		if pass {
			cGreen("  [PASS] %s", label)
		} else {
			cRed("  [FAIL] %s", label)
			failed = true
		}
		if detail != "" {
			cGray("         %s\n", detail)
		}
	}

	// 1. OpenSSH service
	if serviceExists("sshd") {
		out, _ := runCmd("sc.exe", "query", "sshd")
		state := serviceState(out)
		check("sshd service", state == "RUNNING", "state: "+state)
	} else {
		check("sshd service", false, "not installed")
	}

	// 2. sshd_config
	if fileExists(sshdConfigPath) {
		data, err := os.ReadFile(sshdConfigPath)
		content := strings.ToLower(string(data))
		check("sshd_config present", err == nil, sshdConfigPath)
		check("PubkeyAuthentication enabled", strings.Contains(content, "pubkeyauthentication yes"), "")
		check("PasswordAuthentication enabled", strings.Contains(content, "passwordauthentication yes"), "")
	} else {
		check("sshd_config present", false, "missing")
	}

	// 3. Firewall rule
	if runCmdOK("netsh", "advfirewall", "firewall", "show", "rule", "name=OpenSSH-Server-In-TCP") == nil {
		check("Firewall port 22", true, "OpenSSH-Server-In-TCP exists")
	} else {
		check("Firewall port 22", false, "rule missing")
	}

	// 4. User + admin membership
	if userExists(targetUser) {
		check("User "+targetUser, true, "")
		group, err := adminGroupName()
		inGroup := err == nil && runCmdOK("net", "localgroup", group, targetUser) == nil
		check(targetUser+" in admin group", inGroup, "group: "+group)
	} else {
		check("User "+targetUser, false, "not found")
	}

	// 5. Tailscale
	tsPath := findTailscalePath()
	if tsPath == "" {
		check("Tailscale installed", false, "not found")
	} else {
		check("Tailscale installed", true, tsPath)
		if err := runCmdOK(tsPath, "status"); err == nil {
			ip := tailscaleIP(tsPath)
			check("Tailscale connected", ip != "<unavailable>" && ip != "", "IP: "+ip)
		} else {
			check("Tailscale connected", false, "not logged in")
		}
	}

	fmt.Println()
	if failed {
		cRed("Result: one or more checks FAILED")
		return 1
	}
	cGreen("Result: all checks passed")
	return 0
}

// serviceState extracts RUNNING / STOPPED from `sc query` output.
func serviceState(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "STATE") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				return strings.TrimSuffix(parts[3], ",")
			}
		}
	}
	return "UNKNOWN"
}
