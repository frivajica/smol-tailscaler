package winutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
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
