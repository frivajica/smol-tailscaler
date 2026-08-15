package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
)

var (
	cCyan   = color.New(color.FgCyan).PrintfFunc()
	cYellow = color.New(color.FgYellow).PrintfFunc()
	cGreen  = color.New(color.FgGreen).PrintfFunc()
	cRed    = color.New(color.FgRed).PrintfFunc()
	cGray   = color.New(color.FgHiBlack).PrintfFunc()
	cWhite  = color.New(color.FgWhite).PrintfFunc()
)

// step prints a numbered step header.
func step(num int, total int, label string) {
	cYellow("\n[%d/%d] %s\n", num, total, label)
}

func ok(label string) { cGreen("  OK: %s\n", label) }
func warn(format string, args ...any) {
	cYellow("  WARN: %s\n", fmt.Sprintf(format, args...))
}

// runCmd runs a command and returns trimmed stdout+stderr combined.
func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runCmdOK runs a command and returns just the error.
func runCmdOK(name string, args ...string) error {
	_, err := runCmd(name, args...)
	return err
}

// runPS runs a PowerShell script snippet.
func runPS(script string) (string, error) {
	return runCmd("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
}

// serviceExists reports whether a Windows service with the given name is present.
func serviceExists(name string) bool {
	return runCmdOK("sc.exe", "query", name) == nil
}

// serviceRunning reports whether the service currently exists and is RUNNING.
func serviceRunning(name string) bool {
	out, err := runCmd("sc.exe", "query", name)
	if err != nil {
		return false
	}
	return serviceState(out) == "RUNNING"
}

// fileExists reports whether the path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isAdmin reports whether the current process has Administrator privileges.
func isAdmin() bool {
	script := "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)"
	out, err := runPS(script)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(out), "True")
}

// adminGroupName resolves the local Administrators group name for the current
// system language (Administrators / Administradores / Administratoren ...).
func adminGroupName() (string, error) {
	out, err := runPS("(Get-LocalGroup | Where-Object { $_.SID -like 'S-1-5-32-544' }).Name")
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	// Fallback to common names.
	for _, name := range []string{"Administrators", "Administradores", "Administratoren", "Administrateurs"} {
		if runCmdOK("net", "localgroup", name) == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not resolve the Administrators group name")
}
