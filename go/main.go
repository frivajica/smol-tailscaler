package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"setup-windows/internal/ui"
	"setup-windows/internal/winutil"
)

// Embedded secrets - injected at build time via -ldflags (see build.sh).
// Treat these as "security by obscurity": extractable with `strings setup.exe`.
var (
	tsAuthKey    = ""
	userPassword = ""
	targetUser   = ""
)

var (
	verifyMode       bool
	silentMode       bool
	overridePassword string
	overrideAuthKey  string
	overrideUser     string
)

func main() {
	flag.BoolVar(&verifyMode, "verify", false, "check current state without making changes")
	flag.BoolVar(&silentMode, "silent", false, "use embedded values only, never prompt")
	flag.StringVar(&overridePassword, "password", "", "override embedded user password")
	flag.StringVar(&overrideAuthKey, "authkey", "", "override embedded Tailscale auth key")
	flag.StringVar(&overrideUser, "user", "", "override embedded admin user")
	flag.Parse()

	// Keep ldflags-embedded values unless explicitly overridden on the CLI.
	if overridePassword != "" {
		userPassword = overridePassword
	}
	if overrideAuthKey != "" {
		tsAuthKey = overrideAuthKey
	}
	if overrideUser != "" {
		targetUser = overrideUser
	}
	if targetUser == "" {
		targetUser = "admin"
	}
	if !validUsername(targetUser) {
		fatal("invalid user name %q: only letters, digits, '.', '_' and '-' are allowed", targetUser)
	}

	if !winutil.IsAdmin() {
		relaunchAsAdmin()
	}

	if verifyMode {
		exit(verify())
	}

	if err := resolveSecrets(); err != nil {
		fatal("%s", err)
	}

	banner()

	tsPath, err := runSetup()
	if err != nil {
		printReport(tsPath, false)
		fatal("Setup failed: %s", err)
	}

	printReport(tsPath, true)
	exit(0)
}

// relaunchAsAdmin re-launches the process elevated so double-click works;
// the new process takes over and this one exits.
func relaunchAsAdmin() {
	var args []string
	for _, a := range os.Args[1:] {
		args = append(args, strconv.Quote(a))
	}
	arglist := strings.Join(args, " ")
	if arglist != "" {
		arglist = " -ArgumentList " + arglist
	}
	cmd := fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs%s", os.Args[0], arglist)
	if err := winutil.RunCmdOK("powershell", "-NoProfile", "-Command", cmd); err != nil {
		fatal("could not relaunch as Administrator: %s", err)
	}
	os.Exit(0)
}

// exit prints the exit-prompt so the window stays open for reading, then exits.
func exit(code int) {
	exitPause()
	os.Exit(code)
}

// exitPause keeps the console window open after a double-click launch.
func exitPause() {
	fmt.Println()
	fmt.Print("Press Enter to exit...")
	fmt.Scanln()
	fmt.Println()
}

// resolveSecrets fills in any missing values, prompting unless silent mode.
func resolveSecrets() error {
	if userPassword == "" {
		if silentMode {
			return fmt.Errorf("no user password provided and -silent is set")
		}
		secret, err := ui.PromptSecret("Password for user '" + targetUser + "'")
		if err != nil {
			return err
		}
		userPassword = secret
	}
	if tsAuthKey == "" {
		if silentMode {
			return fmt.Errorf("no Tailscale auth key provided and -silent is set")
		}
		secret, err := ui.PromptSecret("Tailscale auth key")
		if err != nil {
			return err
		}
		tsAuthKey = secret
	}
	return nil
}

func banner() {
	ui.Header("Setup: SSH + Tailscale")
	fmt.Printf("Target user: %s\n", targetUser)
}

func fatal(format string, args ...any) {
	ui.Red(fmt.Sprintf("ERROR: %s", fmt.Sprintf(format, args...)))
	exit(1)
}
