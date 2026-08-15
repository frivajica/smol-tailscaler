// Package report prints the final deployment summary and service status.
package report

import (
	"fmt"
	"os"
	"strings"

	"smol-tailscaler/internal/config"
	"smol-tailscaler/internal/tailscale"
	"smol-tailscaler/internal/ui"
	"smol-tailscaler/internal/winutil"
)

func field(label, value string) {
	ui.Yellow(" %-14s ", label)
	ui.White(fmt.Sprintf("%s\n", value))
}

// Print shows the deployment summary. tsPath is the Tailscale CLI path.
func Print(cfg *config.Config, tsPath string, success bool) {
	hostname, _ := os.Hostname()

	ui.Cyan("\n==============================================")
	if success {
		ui.Cyan(" DEPLOYMENT COMPLETE - SAVE THIS INFO         ")
	} else {
		ui.Red(" SETUP INCOMPLETE - PARTIAL STATE BELOW        ")
	}
	ui.Cyan("==============================================")
	fmt.Println()
	field("User", cfg.TargetUser)
	field("Password", cfg.UserPassword)
	field("Hostname", hostname)
	field("Tailscale IP", tailscale.IP(tsPath))
	field("Tailscale CLI", tsPath)
	fmt.Println()
	ui.Cyan("----------------------------------------------")
	ui.Cyan(" Connect with:")
	fmt.Printf("   ssh %s@<%s>\n", cfg.TargetUser, "tailscale-ip")
	ui.Cyan("----------------------------------------------")
	fmt.Println()

	// Service status.
	services := []string{"Tailscale"}
	if winutil.ServiceExists("sshd") {
		services = append([]string{"sshd"}, services...)
	}
	fmt.Printf("%-12s %-10s %s\n", "SERVICE", "STATE", "START")
	for _, svc := range services {
		state := "?"
		if q, err := winutil.RunCmd("sc.exe", "query", svc); err == nil {
			state = winutil.ServiceState(q)
		}
		start := "?"
		if q, err := winutil.RunCmd("sc.exe", "qc", svc); err == nil {
			start = winutil.StartType(q)
		}
		fmt.Printf("%-12s %-10s %s\n", svc, state, start)
	}
	fmt.Println()

	fmt.Println(" Local users:")
	users, err := winutil.RunPS("Get-LocalUser | Sort-Object Name | Select-Object -ExpandProperty Name")
	if err == nil {
		for _, u := range strings.Split(users, "\n") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			enabled, _ := winutil.RunPS(fmt.Sprintf("(Get-LocalUser -Name '%s').Enabled", u))
			passSet, _ := winutil.RunPS(fmt.Sprintf("(Get-LocalUser -Name '%s').PasswordLastSet", u))
			fmt.Printf("   %-20s active=%-5s password=%s\n", u, yesNo(enabled), yesNo(passSet))
		}
	}

	if success {
		ui.Green("\nMachine is remotely accessible. Save the password before closing!")
		ui.Yellow("Remember: change the temporary password after first login.\n")
	} else {
		ui.Red("\nSetup did not complete. Check the failed step above.\n")
	}
}

func yesNo(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "false") {
		return "no"
	}
	return "yes"
}
