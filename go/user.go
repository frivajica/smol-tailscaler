package main

import (
	"fmt"
)

func userExists(name string) bool {
	_, err := runPS(fmt.Sprintf("Get-LocalUser -Name '%s' -ErrorAction SilentlyContinue", name))
	return err == nil
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
		script := fmt.Sprintf("New-LocalUser -Name '%s' -Password (ConvertTo-SecureString '%s' -AsPlainText -Force) -Description 'Administrador con acceso total' -PasswordNeverExpires", targetUser, userPassword)
		if _, err := runPS(script); err != nil {
			return fmt.Errorf("creating user: %w", err)
		}
		ok("User created")
	}

	if err := runCmdOK("net", "localgroup", group, targetUser, "/add"); err != nil {
		return fmt.Errorf("adding to %s: %w", group, err)
	}
	ok("Added to " + group)
	return nil
}
