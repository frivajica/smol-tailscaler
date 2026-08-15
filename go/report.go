package main

import (
	"fmt"
	"os"
	"strings"
)

func printField(label, value string) {
	cYellow(" %-14s ", label)
	cWhite(fmt.Sprintf("%s\n", value))
}

func printReport(tsPath string, success bool) {
	hostname, _ := os.Hostname()

	cCyan("\n==============================================")
	if success {
		cCyan(" DEPLOYMENT COMPLETE - SAVE THIS INFO         ")
	} else {
		cRed(" SETUP INCOMPLETE - PARTIAL STATE BELOW        ")
	}
	cCyan("==============================================")
	fmt.Println()
	printField("User", targetUser)
	printField("Password", userPassword)
	printField("Hostname", hostname)
	printField("Tailscale IP", tailscaleIP(tsPath))
	printField("Tailscale CLI", tsPath)
	fmt.Println()
	cCyan("----------------------------------------------")
	cCyan(" Connect with:")
	fmt.Printf("   ssh %s@<%s>\n", targetUser, "tailscale-ip")
	cCyan("----------------------------------------------")
	fmt.Println()

	// Service status.
	services := []string{"Tailscale"}
	if serviceExists("sshd") {
		services = append([]string{"sshd"}, services...)
	}
	fmt.Printf("%-12s %-10s %s\n", "SERVICE", "STATE", "START")
	for _, svc := range services {
		state := "?"
		if q, err := runCmd("sc.exe", "query", svc); err == nil {
			state = serviceState(q)
		}
		start := "?"
		if q, err := runCmd("sc.exe", "qc", svc); err == nil {
			start = startType(q)
		}
		fmt.Printf("%-12s %-10s %s\n", svc, state, start)
	}
	fmt.Println()

	fmt.Println(" Local users:")
	users, err := runPS("Get-LocalUser | Sort-Object Name | Select-Object -ExpandProperty Name")
	if err == nil {
		for _, u := range strings.Split(users, "\n") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			enabled, _ := runPS(fmt.Sprintf("(Get-LocalUser -Name '%s').Enabled", u))
			passSet, _ := runPS(fmt.Sprintf("(Get-LocalUser -Name '%s').PasswordLastSet", u))
			fmt.Printf("   %-20s active=%-5s password=%s\n", u, yesNo(enabled), yesNo(passSet))
		}
	}

	if success {
		cGreen("\nMachine is remotely accessible. Save the password before closing!")
		cYellow("Remember: change the temporary password after first login.\n")
	} else {
		cRed("\nSetup did not complete. Check the failed step above.\n")
	}
}

// startType extracts the start type from `sc qc` output. The label is
// localized (START_TYPE / TIPO_INICIO / ...), so match the numeric code that
// uniquely identifies the field: 2=auto (incl. delayed), 3=demand, 4=disabled.
func startType(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && strings.ContainsRune(fields[1], ':') {
			switch fields[2] {
			case "2":
				return "AUTO_START"
			case "3":
				return "DEMAND_START"
			case "4":
				return "DISABLED"
			}
		}
	}
	return "?"
}

func yesNo(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "false") {
		return "no"
	}
	return "yes"
}
