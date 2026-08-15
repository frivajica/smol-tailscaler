package main

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
)

const opensshReleaseAPI = "https://api.github.com/repos/PowerShell/Win32-OpenSSH/releases/latest"

type githubRelease struct {
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func installOpenSSHFromGitHub() error {
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

	zipPath := os.TempDir() + "\\openssh.zip"
	if err := downloadFile(asset.BrowserDownloadURL, zipPath); err != nil {
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
	if serviceExists("sshd") {
		return nil
	}
	if out, err := runCmd("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", installDir+`\install-sshd.ps1`); err != nil {
		return fmt.Errorf("install-sshd.ps1: %w (%s)", err, out)
	}
	runCmdOK("sc.exe", "config", "sshd", "start=auto")
	runCmdOK("sc.exe", "start", "sshd")
	return nil
}

func fetchGitHubRelease() (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, opensshReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := apiClient.Do(req)
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

func downloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
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
			if err := os.MkdirAll(target, 0); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0); err != nil {
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
