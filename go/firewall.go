package main

func stepFirewallAndServices() {
	step(6, 6, "Firewall + services")

	// Allow inbound SSH on port 22.
	if runCmdOK("netsh", "advfirewall", "firewall", "show", "rule", "name=OpenSSH-Server-In-TCP") != nil {
		if err := runCmdOK("netsh", "advfirewall", "firewall", "add", "rule",
			"name=OpenSSH-Server-In-TCP", "dir=in", "action=allow", "protocol=TCP", "localport=22"); err != nil {
			warn("firewall rule: %s", err)
		} else {
			ok("Firewall rule created for port 22")
		}
	} else {
		ok("Firewall rule already exists")
	}

	if serviceExists("sshd") {
		runCmdOK("sc.exe", "config", "sshd", "start=auto")
		runCmdOK("sc.exe", "start", "sshd")
		ok("sshd set to auto-start")
	} else {
		warn("sshd service not found - install OpenSSH Server first")
	}

	if serviceExists("Tailscale") {
		runCmdOK("sc.exe", "config", "Tailscale", "start=auto")
		runCmdOK("sc.exe", "start", "Tailscale")
		ok("Tailscale set to auto-start")
	}
}
