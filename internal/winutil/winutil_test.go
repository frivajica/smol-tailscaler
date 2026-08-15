package winutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSHA256File(t *testing.T) {
	content := []byte("smol-tailscaler checksum test\n")
	path := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	got, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File() error = %v", err)
	}
	if got != want {
		t.Errorf("SHA256File() = %q, want %q", got, want)
	}
}

func TestRunWithProgress(t *testing.T) {
	orig := progressTick
	progressTick = 200 * time.Millisecond
	defer func() { progressTick = orig }()

	if runtime.GOOS == "windows" {
		t.Skip("tests use POSIX sh")
	}

	t.Run("completes and reports progress", func(t *testing.T) {
		var ticks []string
		out, err := runWithProgress("sh", []string{"-c", "echo hi; sleep 1"}, "test", 5*time.Second,
			func(s string) { ticks = append(ticks, s) })
		if err != nil {
			t.Fatalf("runWithProgress() error = %v", err)
		}
		if out != "hi" {
			t.Errorf("output = %q, want %q", out, "hi")
		}
		if len(ticks) == 0 {
			t.Error("expected at least one progress tick")
		}
	})

	t.Run("times out and kills", func(t *testing.T) {
		_, err := runWithProgress("sh", []string{"-c", "sleep 30"}, "test", 300*time.Millisecond, nil)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "did not finish within") {
			t.Errorf("error = %q, want timeout message", err)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		if _, err := runWithProgress("definitely-not-a-real-binary-xyz", nil, "test", time.Second, nil); err == nil {
			t.Error("expected error for missing executable")
		}
	})
}
