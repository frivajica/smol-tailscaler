// Package winutil wraps Windows process execution, file checks, downloads,
// and service query helpers used across setup steps.
package winutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
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

// progressTick controls how often runWithProgress reports elapsed time.
// Exposed as a var so tests can shorten it.
var progressTick = 15 * time.Second

// RunPSWithProgress runs a PowerShell script snippet whose output is captured
// (and therefore invisible on screen), reporting elapsed time via progress
// while it runs and killing the process tree if it exceeds timeout. Long steps
// like a DISM Windows Update download would otherwise look hung for minutes.
func RunPSWithProgress(script, what string, timeout time.Duration, progress func(elapsed string)) (string, error) {
	return runWithProgress("powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script}, what, timeout, progress)
}

func runWithProgress(name string, args []string, what string, timeout time.Duration, progress func(elapsed string)) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	start := time.Now()
	ticker := time.NewTicker(progressTick)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case err := <-done:
			return strings.TrimSpace(buf.String()), err
		case <-timer.C:
			// Kill the tree so a DISM child isn't orphaned when the wrapping
			// PowerShell is terminated. Kill first (portable), then taskkill
			// for any remaining children.
			_ = cmd.Process.Kill()
			_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
			<-done
			return strings.TrimSpace(buf.String()), fmt.Errorf("%s did not finish within %s", what, timeout)
		case <-ticker.C:
			if progress != nil {
				progress(fmt.Sprintf("still running (%s elapsed)...", time.Since(start).Round(time.Second)))
			}
		}
	}
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

// VerifySHA256 checks file against the "<url>.sha256" sidecar served next to
// the download. The "latest" alias does not serve a sidecar, so its redirect
// target is resolved first and the sidecar is fetched from the versioned URL.
func VerifySHA256(url, file string) error {
	versioned, err := resolveRedirect(url)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, versioned+".sha256", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := APIClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum for %s returned %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	want := strings.TrimSpace(string(body))
	got, err := SHA256File(file)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, want)
	}
	return nil
}

// SHA256File returns the hex SHA-256 digest of a file.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// resolveRedirect returns url itself if it responds directly, or the final
// location it redirects to (without following it).
func resolveRedirect(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return rawURL, nil
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("resolving %s: got %s with no redirect target", rawURL, resp.Status)
	}
	base, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
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
