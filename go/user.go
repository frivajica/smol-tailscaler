package main

import (
	"fmt"
	"strings"

	"setup-windows/internal/ui"
	"setup-windows/internal/winutil"
)

// validUsername reports whether name is safe to interpolate into PowerShell
// single-quoted strings. Windows allows a wider charset, but restricting to a
// conservative set keeps the setup commands injection-free.
func validUsername(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func userExists(name string) bool {
	_, err := winutil.RunPS(fmt.Sprintf("Get-LocalUser -Name '%s' -ErrorAction SilentlyContinue", name))
	return err == nil
}

// userDescription returns the account description in the system's UI language.
func userDescription() string {
	if out, err := winutil.RunPS("(Get-WinSystemLocale).Name"); err == nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(out)), "es") {
		return "Administrador con acceso total"
	}
	return "Administrator with full access"
}

func stepUser() error {
	ui.Step(2, 6, "Admin user '"+targetUser+"'")

	if !validUsername(targetUser) {
		return fmt.Errorf("invalid user name %q: only letters, digits, '.', '_' and '-' are allowed", targetUser)
	}

	group, err := winutil.AdminGroupName()
	if err != nil {
		return err
	}
	fmt.Printf("  Administrators group detected: %s\n", group)

	// The password travels via $env:TS_USER_PASSWORD so it never lands in the
	// PowerShell command line (visible via process listings) or the script.
	if userExists(targetUser) {
		if _, err := winutil.RunPSEnv(fmt.Sprintf("Set-LocalUser -Name '%s' -Password (ConvertTo-SecureString $env:TS_USER_PASSWORD -AsPlainText -Force)", targetUser), "TS_USER_PASSWORD="+userPassword); err != nil {
			return fmt.Errorf("updating user password: %w", err)
		}
		ui.Ok("Existing user password updated")
	} else {
		script := fmt.Sprintf("New-LocalUser -Name '%s' -Password (ConvertTo-SecureString $env:TS_USER_PASSWORD -AsPlainText -Force) -Description '%s' -PasswordNeverExpires", targetUser, userDescription())
		if _, err := winutil.RunPSEnv(script, "TS_USER_PASSWORD="+userPassword); err != nil {
			return fmt.Errorf("creating user: %w", err)
		}
		ui.Ok("User created")
	}

	// Idempotent: `net localgroup X user /add` fails with exit code 2 when the
	// user is already a member, so check membership first.
	member, _ := winutil.RunPS(fmt.Sprintf("Get-LocalGroupMember -Group '%s' -Member '%s' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name", group, targetUser))
	if strings.Contains(member, targetUser) {
		ui.Ok("Already in " + group)
	} else if err := winutil.RunCmdOK("net", "localgroup", group, targetUser, "/add"); err != nil {
		return fmt.Errorf("adding to %s: %w", group, err)
	} else {
		ui.Ok("Added to " + group)
	}
	return nil
}
