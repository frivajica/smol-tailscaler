// Package ssh installs and configures the OpenSSH Server.
package ssh

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"smol-tailscaler/internal/ui"
	"smol-tailscaler/internal/winutil"
)

// ConfigPath is where the OpenSSH server config lives.
const ConfigPath = `C:\ProgramData\ssh\sshd_config`

const opensshReleaseAPI = "https://api.github.com/repos/PowerShell/Win32-OpenSSH/releases/latest"

type githubRelease struct {
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// EnsureInstalled ensures the OpenSSH Server service exists, installing via
// DISM with a GitHub release fallback when Windows Update is unavailable.
func EnsureInstalled() error {
	if winutil.ServiceExists("sshd") {
		ui.Ok("OpenSSH Server already installed")
		return nil
	}

	ui.Cyan("  Installing via Windows Update (DISM)...\n")
	capOut, err := winutil.RunPS("(Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*').Name")
	if err == nil && capOut != "" {
		if _, err := winutil.RunPS(fmt.Sprintf("Add-WindowsCapability -Online -Name '%s'", capOut)); err == nil && winutil.ServiceExists("sshd") {
			ui.Ok("OpenSSH Server installed via DISM")
			return nil
		}
		ui.Cyan("  DISM reported success but sshd is missing (reboot pending or unsupported edition); trying GitHub...\n")
	}

	ui.Cyan("  Falling back to GitHub release...\n")
	if err := installFromGitHub(); err != nil {
		return err
	}
	if !winutil.ServiceExists("sshd") {
		return fmt.Errorf("OpenSSH installed but sshd service is still missing")
	}
	ui.Ok("OpenSSH Server installed from GitHub")
	return nil
}

func installFromGitHub() error {
	arch := "Win64"
	if runtime.GOARCH == "386" {
		arch = "Win32"
	}

	release, err := fetchGitHubRelease()
	if err != nil {
		return err
	}
	var asset struct {
		Name               string
		BrowserDownloadURL string
	}
	for _, a := range release.Assets {
		if strings.Contains(a.Name, arch) && strings.HasSuffix(a.Name, ".zip") {
			asset.Name = a.Name
			asset.BrowserDownloadURL = a.BrowserDownloadURL
			break
		}
	}
	if asset.BrowserDownloadURL == "" {
		return fmt.Errorf("no %s zip asset found in latest Win32-OpenSSH release", arch)
	}

	zipPath := filepath.Join(os.TempDir(), "openssh.zip")
	if err := winutil.DownloadFile(asset.BrowserDownloadURL, zipPath); err != nil {
		return err
	}
	defer os.Remove(zipPath)

	installDir := `C:\Program Files\OpenSSH`
	if err := os.MkdirAll(installDir, 0); err != nil {
		return err
	}
	if err := unzip(zipPath, installDir); err != nil {
		return err
	}

	// Register the service using the bundled installer script. The script is
	// idempotent, so re-running setup after a partial install is safe.
	if winutil.ServiceExists("sshd") {
		return nil
	}
	if out, err := winutil.RunPSFile(installDir + `\install-sshd.ps1`); err != nil {
		return fmt.Errorf("install-sshd.ps1: %w (%s)", err, out)
	}
	winutil.RunCmdOK("sc.exe", "config", "sshd", "start=auto")
	winutil.RunCmdOK("sc.exe", "start", "sshd")
	return nil
}

func fetchGitHubRelease() (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, opensshReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", winutil.UserAgent)
	resp, err := winutil.APIClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// WriteConfig applies a clean base config. Password auth is enabled so the
// user can log in and add their key manually (disable it afterwards).
func WriteConfig() error {
	content := `Port 22
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
PasswordAuthentication yes
PermitEmptyPasswords no
Subsystem sftp sftp-server.exe
`
	if err := os.MkdirAll(`C:\ProgramData\ssh`, 0); err != nil {
		return err
	}

	// The DISM/GitHub installer locks sshd_config down with SYSTEM-only ACLs
	// (possibly including Deny ACEs). Take ownership and grant write access via
	// SIDs so the overwrite below can't hit "Access denied".
	if winutil.FileExists(ConfigPath) {
		winutil.RunCmdOK("attrib", "-r", ConfigPath)
		winutil.RunCmdOK("takeown", "/f", ConfigPath)
		if out, err := winutil.RunCmd("icacls", ConfigPath, "/reset"); err != nil {
			ui.Gray("  note: icacls reset: %s\n", strings.TrimSpace(out))
		}
		if out, err := winutil.RunCmd("icacls", ConfigPath, "/inheritance:r", "/grant", "*S-1-5-18:(F)", "/grant", "*S-1-5-32-544:(F)"); err != nil {
			ui.Gray("  note: icacls grant: %s\n", strings.TrimSpace(out))
		}
	}
	if err := os.WriteFile(ConfigPath, []byte(content), 0); err != nil {
		// Fallback: PowerShell Set-Content runs in the already-elevated
		// session and often succeeds where os.WriteFile hits ACL issues.
		ps := fmt.Sprintf("Set-Content -Path '%s' -Value @'%s'@ -Encoding ascii", ConfigPath, content)
		if _, perr := winutil.RunPS(ps); perr != nil {
			return fmt.Errorf("writing sshd_config: %w (PowerShell fallback: %v)", err, perr)
		}
	}
	ui.Ok("sshd_config written")
	return nil
}

// SetDefaultShell makes OpenSSH launch PowerShell for interactive sessions
// instead of cmd.exe via the DefaultShell registry value.
func SetDefaultShell() error {
	const shell = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	script := fmt.Sprintf("New-Item -Path 'HKLM:\\SOFTWARE\\OpenSSH' -Force | Out-Null; Set-ItemProperty -Path 'HKLM:\\SOFTWARE\\OpenSSH' -Name 'DefaultShell' -Value '%s' -Force", shell)
	return winutil.RunCmdOK("powershell", "-NoProfile", "-Command", script)
}

// zipRootPrefix returns the top-level directory all entries share, or "" if
// the zip is flat. The GitHub OpenSSH zip nests everything under a single
// directory, which we strip on extraction.
func zipRootPrefix(files []*zip.File) (string, error) {
	var root string
	for _, f := range files {
		name := f.Name
		i := strings.IndexAny(name, "/\\")
		first := name
		if i >= 0 {
			first = name[:i]
		}
		if first == "" || first == "." {
			continue
		}
		if root == "" {
			root = first
		} else if root != first {
			return "", nil
		}
	}
	return root, nil
}

func unzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	// The OpenSSH-Win64 zip wraps every entry in a top-level directory
	// (e.g. "OpenSSH-Win64/"). Strip it so the service paths point straight at
	// C:\Program Files\OpenSSH\*.exe instead of a nested folder.
	prefix, err := zipRootPrefix(r.File)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		rel, err := filepath.Rel(prefix, f.Name)
		if err != nil || rel == "." {
			continue
		}
		// Zip entries use forward slashes; filepath normalizes them to the OS
		// separator and filepath.Dir resolves the real parent dir for MkdirAll.
		// filepath.Clean also neutralizes any ../ traversal in the entry name.
		target := filepath.Join(destDir, rel)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("zip entry escapes destination: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(target)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		dst.Close()
	}
	return nil
}
