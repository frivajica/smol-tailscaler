// Package winutil wraps Windows process execution, file checks, downloads,
// and service query helpers used across setup steps.
package winutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// UserAgent identifies the tool to download hosts (Tailscale, GitHub).
const UserAgent = "smol-tailscaler/setup"

// HTTPClient is used for large downloads so a dead network can't hang setup
// forever waiting on a silent blackhole.
var HTTPClient = &http.Client{Timeout: 10 * time.Minute}

// APIClient is used for small metadata requests (GitHub releases); failing
// fast here beats waiting out the long download timeout.
var APIClient = &http.Client{Timeout: 30 * time.Second}

// RunCmd runs a command and returns trimmed stdout+stderr combined.
func RunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunCmdOK runs a command and returns just the error.
func RunCmdOK(name string, args ...string) error {
	_, err := RunCmd(name, args...)
	return err
}

// RunPS runs a PowerShell script snippet.
func RunPS(script string) (string, error) {
	return RunCmd("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
}

// RunPSEnv runs a PowerShell script snippet with extra KEY=VALUE entries set
// on the child process environment, so secrets reach the script as $env:VAR
// instead of being interpolated into the command line (visible in process
// listings) or the script string itself (injection surface).
func RunPSEnv(script string, env ...string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunPSFile runs a PowerShell script file.
func RunPSFile(path string) (string, error) {
	return RunCmd("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path)
}

// FileExists reports whether the path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DownloadFile downloads url to dest over HTTPS with a timeout and User-Agent.
func DownloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned %s", url, resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// IsAdmin reports whether the current process has Administrator privileges.
func IsAdmin() bool {
	script := "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)"
	out, err := RunPS(script)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(out), "True")
}

// AdminGroupName resolves the local Administrators group name for the current
// system language (Administrators / Administradores / Administratoren ...).
func AdminGroupName() (string, error) {
	out, err := RunPS("(Get-LocalGroup | Where-Object { $_.SID -like 'S-1-5-32-544' }).Name")
	if err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out), nil
	}
	for _, name := range []string{"Administrators", "Administradores", "Administratoren", "Administrateurs"} {
		if RunCmdOK("net", "localgroup", name) == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not resolve the Administrators group name")
}
