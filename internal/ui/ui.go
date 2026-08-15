// Package ui provides all user-facing output helpers (colors, step headers,
// banners) and secret prompting.
package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"
)

var (
	Cyan   = color.New(color.FgCyan).PrintfFunc()
	Yellow = color.New(color.FgYellow).PrintfFunc()
	Green  = color.New(color.FgGreen).PrintfFunc()
	Red    = color.New(color.FgRed).PrintfFunc()
	Gray   = color.New(color.FgHiBlack).PrintfFunc()
	White  = color.New(color.FgWhite).PrintfFunc()
)

// Header prints a section banner box.
func Header(title string) {
	Cyan("==============================================")
	Cyan(" %-45s", title)
	Cyan("==============================================")
}

// Step prints a numbered step header.
func Step(num, total int, label string) {
	Yellow("\n[%d/%d] %s\n", num, total, label)
}

func Ok(label string) { Green("  OK: %s\n", label) }
func Warn(format string, args ...any) {
	Yellow("  WARN: %s\n", fmt.Sprintf(format, args...))
}

// PromptSecret reads a password-style value from the terminal.
func PromptSecret(label string) (string, error) {
	fmt.Printf("%s: ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
