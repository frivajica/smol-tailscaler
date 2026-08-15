package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Embedded secrets - injected at build time via -ldflags (see build.sh).
// Treat these as "security by obscurity": extractable with `strings setup.exe`.
var (
	tsAuthKey    = ""
	userPassword = ""
)

var (
	targetUser string
	verifyMode bool
	silentMode bool
)

func main() {
	flag.BoolVar(&verifyMode, "verify", false, "check current state without making changes")
	flag.BoolVar(&silentMode, "silent", false, "use embedded values only, never prompt")
	flag.StringVar(&userPassword, "password", "", "override embedded user password")
	flag.StringVar(&tsAuthKey, "authkey", "", "override embedded Tailscale auth key")
	flag.StringVar(&targetUser, "user", "frivajica", "admin user to create")
	flag.Parse()

	if !isAdmin() {
		fatal("This tool must be run as Administrator.")
	}

	if verifyMode {
		os.Exit(verify())
	}

	if err := resolveSecrets(); err != nil {
		fatal("%s", err)
	}

	banner()

	if err := runSetup(); err != nil {
		fatal("Setup failed: %s", err)
	}
}

// resolveSecrets fills in any missing values, prompting unless silent mode.
func resolveSecrets() error {
	if userPassword == "" {
		if silentMode {
			return fmt.Errorf("no user password provided and -silent is set")
		}
		userPassword = promptSecret("Password for user '" + targetUser + "'")
	}
	if tsAuthKey == "" {
		if silentMode {
			return fmt.Errorf("no Tailscale auth key provided and -silent is set")
		}
		tsAuthKey = promptSecret("Tailscale auth key")
	}
	return nil
}

func banner() {
	cCyan("==============================================")
	cCyan(" Setup: SSH + Tailscale                        ")
	cCyan("==============================================")
	fmt.Printf("Target user: %s\n", targetUser)
}

func promptSecret(label string) string {
	fmt.Printf("%s: ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		fatal("could not read input: %s", err)
	}
	return strings.TrimSpace(string(b))
}

func fatal(format string, args ...any) {
	cRed(fmt.Sprintf("ERROR: %s", fmt.Sprintf(format, args...)))
	os.Exit(1)
}
