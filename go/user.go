package main

import (
	"fmt"
	"strings"
)

func userExists(name string) bool {
	_, err := runPS(fmt.Sprintf("Get-LocalUser -Name '%s' -ErrorAction SilentlyContinue", name))
	return err == nil
}

// userDescription returns the account description in the system's UI language.
func userDescription() string {
	if out, err := runPS("(Get-WinSystemLocale).Name"); err == nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(out)), "es") {
		return "Administrador con acceso total"
	}
	return "Administrator with full access"
}

func stepUser() error {
	step(2, 6, "Admin user '"+targetUser+"'")

	group, err := adminGroupName()
	if err != nil {
		return err
	}
	fmt.Printf("  Administrators group detected: %s\n", group)

	if userExists(targetUser) {
		runPS(fmt.Sprintf("Set-LocalUser -Name '%s' -Password (ConvertTo-SecureString '%s' -AsPlainText -Force)", targetUser, userPassword))
		ok("Existing user password updated")
	} else {
		script := fmt.Sprintf("New-LocalUser -Name '%s' -Password (ConvertTo-SecureString '%s' -AsPlainText -Force) -Description '%s' -PasswordNeverExpires", targetUser, userPassword, userDescription())
		if _, err := runPS(script); err != nil {
			return fmt.Errorf("creating user: %w", err)
		}
		ok("User created")
	}

	// Idempotent: `net localgroup X user /add` fails with exit code 2 when the
	// user is already a member, so check membership first.
	member, _ := runPS(fmt.Sprintf("Get-LocalGroupMember -Group '%s' -Member '%s' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name", group, targetUser))
	if strings.Contains(member, targetUser) {
		ok("Already in " + group)
	} else if err := runCmdOK("net", "localgroup", group, targetUser, "/add"); err != nil {
		return fmt.Errorf("adding to %s: %w", group, err)
	} else {
		ok("Added to " + group)
	}
	return nil
}
