package ssh

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// zipBytes builds an in-memory zip containing the given entries. Dir entries
// are marked with "DIR" content.
func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating %q: %v", name, err)
		}
		if content != "DIR" {
			if _, err := fw.Write([]byte(content)); err != nil {
				t.Fatalf("writing %q: %v", name, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

func zipFiles(t *testing.T, data []byte) []*zip.File {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("reading zip: %v", err)
	}
	return r.File
}

func writeZip(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing zip: %v", err)
	}
	return path
}

func TestZipRootPrefix(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{"nested single dir", map[string]string{
			"OpenSSH-Win64/":      "DIR",
			"OpenSSH-Win64/a.exe": "a",
			"OpenSSH-Win64/b.exe": "b",
		}, "OpenSSH-Win64"},
		{"flat", map[string]string{
			"a.exe": "a",
			"b.exe": "b",
		}, ""},
		{"mixed roots", map[string]string{
			"a/x": "x",
			"b/y": "y",
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := zipFiles(t, zipBytes(t, tc.entries))
			got, err := zipRootPrefix(files)
			if err != nil {
				t.Fatalf("zipRootPrefix() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("zipRootPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnzipRejectsTraversal(t *testing.T) {
	// The mixed roots force zipRootPrefix to "" so the "../" in the entry name
	// is not cancelled out by the prefix, and the escape check triggers.
	zipPath := writeZip(t, zipBytes(t, map[string]string{
		"../evil.exe": "boom",
		"safe.txt":    "ok",
	}))

	dest := t.TempDir()
	if err := unzip(zipPath, dest); err == nil {
		t.Fatal("unzip() accepted a path-traversal entry")
	}
	if _, err := os.Stat(filepath.Join(dest, "evil.exe")); !os.IsNotExist(err) {
		t.Errorf("traversal file was extracted outside dest")
	}
}

func TestUnzipStripsRootDir(t *testing.T) {
	zipPath := writeZip(t, zipBytes(t, map[string]string{
		"OpenSSH-Win64/sshd.exe":         "sshd",
		"OpenSSH-Win64/scp.exe":          "scp",
		"OpenSSH-Win64/etc/ssh_host_key": "key",
	}))

	dest := t.TempDir()
	if err := unzip(zipPath, dest); err != nil {
		t.Fatalf("unzip() error = %v", err)
	}
	for _, want := range []string{"sshd.exe", "scp.exe", filepath.Join("etc", "ssh_host_key")} {
		if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
			t.Errorf("expected %s to be extracted at dest root: %v", want, err)
		}
	}
}
